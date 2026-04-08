# SPEC: Daemon Subsystem

## Purpose

The daemon is a long-running background process that provides centralized services to the CCC TUI and CLI: periodic data refresh, terminal session tracking, agent lifecycle management, budget governance, and push-event delivery. It communicates with clients over a Unix socket using newline-delimited JSON-RPC.

## Interface

- **Inputs**:
  - JSON-RPC requests over a Unix socket (`~/.config/ccc/daemon.sock`)
  - OS signals (SIGTERM, SIGINT) for graceful shutdown
  - Configuration from `~/.config/ccc/config.yaml` (refresh interval, agent limits, automations)
  - SQLite database (`~/.config/ccc/data/ccc.db`) for persistent session state

- **Outputs**:
  - JSON-RPC responses (one per request)
  - Push events to subscriber connections (newline-delimited JSON)
  - PID file at `~/.config/ccc/daemon.pid`
  - Log output to `~/.config/ccc/data/daemon.log`

- **Dependencies**:
  - `internal/agent` — `Runner`, `GovernedRunner`, `Session` types for agent process management
  - `internal/config` — configuration loading, `ConfigDir()`, `DataDir()`, `DBPath()`
  - `internal/db` — SQLite schema, session record CRUD (`DBInsertSession`, `DBUpdateSession`, `DBLoadVisibleSessions`, etc.)
  - `internal/refresh` — data source refresh pipeline
  - `internal/automation` — post-refresh automation execution
  - `internal/llm` — LLM provider for refresh sources that need summarization

## Behavior

### Wire Protocol

All communication uses newline-delimited JSON over a Unix domain socket. Each message is a single JSON object terminated by `\n`.

- **Request**: `{"method": "<Method>", "id": <int>, "params": {...}}`
- **Response**: `{"id": <int>, "result": {...}}` or `{"id": <int>, "error": {"code": <int>, "message": "<string>"}}`
- **Push event**: `{"type": "<event.type>", "data": {...}}`

Error codes follow JSON-RPC conventions:
- `-32700` — parse error (malformed JSON)
- `-32601` — method not found
- `-32602` — invalid params
- `-32000` — application error (generic)

Scanner buffer: 64KB initial, 4MB max (accommodates large agent prompts).

### Process Lifecycle

**Starting the daemon:**

1. `ccc daemon start` checks the PID file; errors if a live daemon already exists
2. Re-execs the `ccc` binary with `--daemon-internal` flag
3. The child process runs in a new session (`Setsid: true`) so it survives parent exit
4. Parent writes the child PID to `daemon.pid` and exits
5. Child loads config, opens the database, creates the `Server`, and calls `Serve()`

**Auto-start from TUI:**

1. When the TUI starts, `connectDaemon()` tries to dial the socket
2. If the connection fails, it calls `daemon.StartProcess()` to spawn the daemon
3. Waits 500ms, then retries the connection
4. If still unreachable, the TUI runs without a daemon connection (non-fatal)

**Stopping the daemon:**

- `ccc daemon stop` reads the PID file and sends SIGTERM
- `ShutdownDaemon` RPC responds with `{"ok": true}`, then asynchronously (after 100ms) calls `Shutdown()`
- `Shutdown()` stops the agent runner, stops the refresh loop, cancels the server context, closes all client connections, and removes the socket file

**Daemon status:**

- `ccc daemon status` reads the PID file, checks process liveness via signal 0, and optionally pings the socket
- `GetDaemonStatus` RPC returns `{"state": "running"|"paused", "active_agents": <int>}`

**Pause/Resume:**

- `PauseDaemon` sets the `paused` atomic bool on both the server and the refresh loop; broadcasts `daemon.paused` event
- `ResumeDaemon` clears the paused flag; broadcasts `daemon.resumed` event
- While paused: the refresh ticker skips runs, and `LaunchAgent` rejects with an error

### Socket Security

Before creating the Unix socket, the daemon sets umask to `0177` so the socket file is created with owner-only permissions (`0600`). The original umask is restored immediately after `net.Listen`.

### Connection Handling

1. Each accepted connection spawns a goroutine (`handleConn`)
2. The goroutine reads newline-delimited JSON messages in a loop
3. Each message is unmarshalled as an `RPCRequest` and dispatched by method name
4. The response is written back on the same connection
5. On disconnect, the connection is removed from the client list

**Subscribe is special:** After the server sends the OK response, the connection becomes push-only. The goroutine blocks on `<-s.ctx.Done()` (server shutdown). The connection is added to the subscriber set and receives broadcast events until shutdown.

### Session Registry

The session registry tracks terminal sessions (Claude Code instances) with an in-memory map backed by SQLite persistence.

**States:** `active` → `ended` → `archived`

**RPCs:**

- `RegisterSession(session_id, pid, project, worktree_path)` — Creates a new session. Resolves git remote URL and branch from the project directory (best-effort, via `git -C`). Persists to `cc_sessions` table.
- `UpdateSession(session_id, topic)` — Updates mutable fields (currently only topic). Errors if session not found.
- `EndSession(session_id)` — Marks an active session as `ended` with a timestamp. No-op if the session is already ended or archived. Errors if session not found.
- `ListSessions()` — Returns all non-archived sessions from the in-memory map.
- `ArchiveSession(session_id)` — Marks a session as `archived`. Only allowed for sessions in `ended` state; errors if session is still `active`.

**Dead session pruning:** After each successful refresh, `pruneDead()` iterates active sessions and checks PID liveness via `kill(pid, 0)`. Dead processes are marked `ended` with a timestamp.

**Persistence across restarts:** On startup, `newSessionRegistry` loads all non-archived sessions from the database into the in-memory map.

**Fallback registration:** The `ccc register` CLI command tries the daemon first; if unreachable, it writes directly to the database.

### Refresh Loop

The refresh loop runs a configurable function at a configurable interval (default 5 minutes, minimum 1 minute).

- **Tick-driven:** A `time.Ticker` fires at the configured interval
- **Pause-aware:** If `paused` is true, the tick is skipped
- **Reentrant-safe:** A mutex-guarded `running` flag prevents concurrent refresh runs; if a refresh is already in progress, the new tick is silently dropped
- **On-demand:** The `Refresh` RPC triggers an immediate run in a goroutine (non-blocking to the caller)
- **Post-refresh callback:** On success, prunes dead sessions and broadcasts `data.refreshed` to subscribers
- **Refresh content:** Reloads config each cycle (picks up config changes), then runs all data sources (calendar, Gmail, GitHub, Slack, Granola). After a successful refresh, runs any configured automations.

### Agent RPCs

Agents are long-running Claude Code subprocess sessions managed by an `agent.Runner` (optionally wrapped with `agent.GovernedRunner` for budget enforcement).

**LaunchAgent(id, prompt, dir, worktree, permission, budget, resume_id, automation):**

1. Rejects if daemon is paused
2. Rejects if runner is not configured
3. Calls `runner.LaunchOrQueue(request)` which returns `(queued, cmd)`
4. If queued and cmd is non-nil, executes cmd synchronously — if the result is `LaunchDeniedMsg`, returns the denial reason as an RPC error (budget/rate-limit exceeded)
5. If not queued and cmd is non-nil, executes cmd in a goroutine. On `SessionStartedMsg`, broadcasts `agent.started` event and spawns a watcher goroutine
6. Returns `{"ok": true, "queued": <bool>}`

**StopAgent(id):**

1. Calls `runner.Kill(id)` — returns not-found error if agent doesn't exist
2. Broadcasts `agent.stopped` event

**AgentStatus(id):**

- Returns `{id, status, session_id, question, started_at}` from `runner.Status(id)`

**ListAgents():**

- Returns all active agents from `runner.Active()`

**SendAgentInput(id, message):**

- Sends a message to a running agent's stdin via `runner.SendMessage(id, message)`

**ListAgentHistory(window_hours):**

- Returns agent runs from the last `window_hours` hours (default 24 if ≤0)
- Calls `db.DBLoadAgentHistory(s.cfg.DB, window)` to load from `cc_agent_costs`
- Enriches entries where `status == "running"` with live status from `runner.Status(agentID)` if runner is available
- Returns `{entries: []AgentHistoryEntry}`

**StreamAgentOutput(agent_id):**

- Returns the current event buffer for a running agent
- If runner is nil or session is not found, returns `{done: true}` (session already cleaned up)
- If session is found: locks `sess.Mu`, copies `sess.Events`, checks `sess.Done()` non-blocking
- Returns `{events: []SessionEvent, done: bool}`

**Agent completion watcher:**

- `watchAgentDone` blocks on `sess.Done()`, then calls `runner.CleanupFinished(id)`, broadcasts `agent.finished` with exit code, and calls `drainNextAgent()` to start the next queued agent
- `drainNextAgent` pops from `runner.DrainQueue()`, launches via `runner.LaunchOrQueue()`, broadcasts `agent.started`, and spawns a new `watchAgentDone` goroutine

**Session ID propagation:**

- After launching an agent, the daemon monitors the session for its Claude session UUID (captured from stream-json output by the agent runner)
- When captured, the daemon broadcasts `agent.session_id` event with payload `{"id": "<agent_id>", "session_id": "<uuid>"}`
- The TUI CC plugin handles this event to update the todo's `SessionID` in memory and DB, enabling session resume and console display

**Cost update broadcasting:**

- The `GovernedRunner`'s cost callback broadcasts `agent.cost_updated` events when `RecordCost` fires, throttled to at most once per 2 seconds per agent
- The broadcast is wired via `SetCostBroadcast()` during daemon `NewServer()` initialization
- Payload: `{"id": "<agent_id>", "cost_usd": <float>, "input_tokens": <int>, "output_tokens": <int>}`
- All subscribing TUI instances receive these events for live cost visibility in the console overlay and budget widget
- On `agent.cost_updated`, the TUI immediately re-polls budget status (bypassing the 5s interval) so the budget widget stays current

**Daemon is required for agents:**

- The TUI does not maintain a local agent runner fallback. All agent operations (launch, watch, status, kill) go through daemon RPCs.
- If the daemon is not connected, agent operations fail with a clear error rather than silently degrading.
- The daemon auto-starts from the TUI on launch and auto-restarts on binary update.

### Budget RPCs

Budget RPCs require a `GovernedRunner` to be configured; they return an error if governance is not enabled.

**GetBudgetStatus():**

- Returns `{hourly_spent, hourly_limit, daily_spent, daily_limit, emergency_stopped, warning_level, active_agents}`
- `active_agents` includes both running (`runner.Active()`) and queued (`runner.QueueLen()`) agents

**StopAllAgents():**

1. Kills all active agents via `runner.Kill` for each
2. Activates emergency stop on the budget tracker (blocks future launches)
3. Broadcasts `budget.emergency_stop` event
4. Returns `{stopped: <count>}`

**ResumeAgents():**

1. Clears the emergency stop on the budget tracker
2. Broadcasts `budget.resumed` event
3. Returns `{resumed: true}`

### LLM Activity RPCs

The daemon maintains an in-memory ring buffer of LLM activity events, providing observability into LLM calls made by the system (command delegation, refresh summarization, etc.).

**Ring buffer:**

- Fixed capacity of 100 events
- On insert: if an event with the same ID already exists, update it in place; otherwise append as new
- When inserting a new event at capacity, the oldest entry is evicted
- `List()` returns a copy of all events sorted newest-first (by `StartedAt` desc), goroutine-safe

**LLMActivityEvent fields:** `ID, Operation, Source, TodoID (optional), StartedAt, FinishedAt (optional), DurationMs (optional), Error (optional), Status ("running"|"completed"|"failed")`

**ReportLLMActivity(LLMActivityEvent):**

1. Calls `buf.Report(evt)` to insert or update the event
2. Broadcasts an event:
   - If `evt.Status == "running"`: broadcasts `llm.started` with `{id, operation}`
   - Otherwise: broadcasts `llm.finished` with `{id, operation, duration_ms}`
3. Returns `{"ok": true}`

**ListLLMActivity():**

- Returns `buf.List()` — all events, newest-first

### Event Subscription

The subscriber system provides push delivery of server events to connected TUI clients.

**Subscription flow:**

1. Client sends `Subscribe` RPC
2. Server responds with `{"ok": true}` and adds the connection to the subscriber set
3. The connection goroutine blocks until server shutdown — no further RPCs are processed on this connection
4. A separate RPC connection is needed for request/response calls

**Broadcasting:**

1. Copies the subscriber list under lock
2. Writes to each subscriber outside the lock (so a slow consumer doesn't stall others)
3. Each write has a 5-second deadline
4. Failed writes cause the connection to be closed and removed from the subscriber set

**Event types:**

- `data.refreshed` — emitted after each successful refresh
- `session.registered` / `session.updated` / `session.ended` — session lifecycle (registration and update broadcast after successful registry mutation)
- `daemon.paused` / `daemon.resumed` — daemon state changes
- `agent.started` / `agent.session_id` / `agent.cost_updated` / `agent.finished` / `agent.stopped` — agent lifecycle
- `budget.emergency_stop` / `budget.resumed` — budget governance events
- `llm.started` / `llm.finished` — LLM activity lifecycle

**TUI integration:**

- `DaemonConn` holds two connections: one for RPCs, one for subscription
- Events are forwarded as `DaemonEventMsg` bubbletea messages via `p.Send()`
- Events are also routed to the plugin event bus as `plugin.Event{Source: "daemon", Topic: evt.Type}`
- On disconnect, a `DaemonDisconnectedMsg` is sent; the TUI retries every 10 seconds via `daemonReconnectCmd`

### Binary Staleness Detection and Auto-Restart

The daemon detects when its own binary has been replaced (e.g. by `make install`) and automatically restarts itself, ensuring running daemons always reflect the latest build.

**Startup:**

1. On startup, the daemon calls `os.Executable()` to resolve its binary path
2. It records the binary's modification time (mtime) via `os.Stat()`
3. These values are stored in the `Server` struct for later comparison

**Periodic check:**

1. A ticker fires every 30 seconds inside the `Serve()` loop (alongside the existing accept loop, run as a separate goroutine)
2. On each tick, the daemon stats its own binary path
3. If the file's mtime is strictly newer than the recorded startup mtime, the binary is considered stale
4. If the file cannot be stat'd (deleted, permission error), the check is silently skipped — no restart

**Graceful restart (re-exec):**

1. When staleness is detected, the daemon logs `"binary updated, restarting..."` to `daemon.log`
2. It calls `Shutdown()` — which stops agents, stops refresh, cancels the context, closes connections, and removes the socket file
3. After shutdown completes, it re-execs itself via `syscall.Exec(binaryPath, os.Args, os.Environ())`
4. The re-exec'd process goes through normal daemon startup: creates socket, writes PID file, begins serving
5. If `syscall.Exec` fails, the daemon logs the error and exits with code 1 (the PID file is stale; the TUI or next `ccc daemon start` will spawn a fresh daemon)

**Safety constraints:**

- The check only compares mtime, not file content — this is intentional for simplicity and speed
- The restart is synchronous within the daemon process: shutdown fully completes before re-exec
- Active agent sessions are stopped via `runner.Shutdown()` (sends SIGINT, waits up to 3s) before re-exec — agents that are mid-work will be interrupted; this is acceptable because the alternative (stale daemon running indefinitely) is worse
- The 30-second check interval means the daemon picks up new binaries within 30 seconds of `make install`

### Makefile Daemon Restart

The `make install` target automatically restarts any running daemon after installing the new binary:

1. After symlinking binaries, the Makefile runs `ccc daemon stop` (sends SIGTERM to the running daemon via PID file)
2. Waits 1 second for shutdown to complete
3. Runs `ccc daemon start` to spawn a fresh daemon with the new binary
4. Both commands are best-effort: if no daemon is running, `stop` is a no-op; if `start` fails, install still succeeds (the TUI will auto-start the daemon on next launch)

### Wire Types

**RPC param types:**

- `RegisterSessionParams{SessionID, PID, Project, WorktreePath}`
- `UpdateSessionParams{SessionID, Topic}`
- `EndSessionParams{SessionID}`
- `ArchiveSessionParams{SessionID}`
- `LaunchAgentParams{ID, Prompt, Dir, Worktree, Permission, Budget, ResumeID, Automation}`
- `StopAgentParams{ID}`
- `AgentStatusParams{ID}`
- `SendAgentInputParams{ID, Message}`
- `ListAgentHistoryParams{WindowHours}`
- `StreamAgentOutputParams{AgentID}`

**RPC result types:**

- `SessionInfo{SessionID, Topic, PID, Project, Repo, Branch, WorktreePath, State, RegisteredAt, EndedAt}`
- `AgentStatusResult{ID, Status, SessionID, Question, StartedAt}`
- `ListAgentHistoryResult{Entries []db.AgentHistoryEntry}`
- `StreamAgentOutputResult{Events []agent.SessionEvent, Done bool}`
- `BudgetStatusResult{HourlySpent, HourlyLimit, DailySpent, DailyLimit, EmergencyStopped, WarningLevel, ActiveAgents}`
- `StopAllAgentsResult{Stopped}`
- `ResumeAgentsResult{Resumed}`
- `DaemonStatusResult{State, ActiveAgents}`

## Test Cases

### Happy Path

- Start daemon, verify socket file exists and PID file is written
- Ping the daemon, receive `{"ok": true}`
- Register a session, list sessions, verify it appears
- Update a session topic, list sessions, verify the topic changed
- Trigger refresh, verify `data.refreshed` event is received by subscriber
- Launch an agent, verify `agent.started` event is broadcast
- Stop an agent, verify `agent.stopped` event is broadcast
- Pause daemon, verify refresh ticks are skipped and agent launches are rejected
- Resume daemon, verify refresh resumes and agent launches succeed
- Shutdown daemon via RPC, verify socket file is removed

### Error Cases

- Connect to daemon when none is running — returns connection error
- Call `LaunchAgent` while daemon is paused — returns error "daemon is paused"
- Call `UpdateSession` with non-existent session ID — returns "session not found"
- Call `ArchiveSession` on an active session — returns "cannot archive active session"
- Call budget RPCs when `GovernedRunner` is nil — returns "budget governance not configured"
- Call agent RPCs when runner is nil — returns "agent runner not configured"
- Send malformed JSON — returns parse error (code -32700)
- Call unknown RPC method — returns method not found (code -32601)

### Edge Cases

- Auto-start from TUI when daemon is not running — daemon starts, TUI connects after 500ms
- Daemon start when PID file references a dead process — stale PID file is ignored, new daemon starts
- Dead session pruning — session with dead PID transitions to `ended` state
- Concurrent refresh — second refresh while first is running is silently dropped (reentrant guard)
- Subscriber with slow write — 5-second write deadline; failed subscriber is removed, others unaffected
- Session persistence across daemon restart — sessions loaded from SQLite on startup
- `ShutdownDaemon` RPC — response is sent before shutdown (100ms delay before `Shutdown()`)
- Socket file security — umask `0177` ensures socket is never world-accessible, even briefly
- Binary staleness — daemon detects updated binary within 30s and auto-restarts via re-exec
- Binary deleted — if binary is removed or unreadable, staleness check is silently skipped (no crash)
- `make install` restarts daemon — stop + start after symlink, both best-effort
- Re-exec failure — daemon logs error and exits with code 1; TUI auto-starts a fresh daemon on next launch
- No client-side socket permission check — umask-on-create is sufficient for single-user macOS; client-side ownership verification is unnecessary (audit 2026-03-30)
- No graceful drain on SIGTERM — `Shutdown()` stops agents cleanly via `runner.Shutdown()`, cancels context, and closes connections; a drain period adds complexity with no practical benefit for fast RPCs (audit 2026-03-30)
- No configurable connection timeout — Unix domain sockets clean up dead fds via kernel lifecycle; subscriber write deadlines (5s) handle slow consumers; idle timeout config adds surface area for no gain (audit 2026-03-30)
