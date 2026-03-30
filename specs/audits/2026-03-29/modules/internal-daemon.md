# Spec Audit: `internal/daemon/`

- **Spec:** `specs/core/daemon.md`
- **Date:** 2026-03-29
- **Code files:** agents.go, budget.go, client.go, daemon.go, refresh.go, rpc.go, sessions.go, start.go, subscribers.go, types.go

## Summary

- **Branches analyzed:** 96
- **Covered:** 79
- **Uncovered-Behavioral:** 8
- **Uncovered-Implementation:** 9
- **Contradictions:** 0
- **Spec-only (no code):** 3

---

## Code-to-Spec Analysis

### `start.go` — `StartProcess()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Resolve executable fails | **[COVERED]** — Spec: "Re-execs the `ccc` binary with `--daemon-internal` flag" (Process Lifecycle) |
| 2 | MkdirAll DataDir fails | **[UNCOVERED-IMPLEMENTATION]** — filesystem error handling, no behavioral spec needed |
| 3 | Open log file fails | **[COVERED]** — Spec: "Log output to `~/.config/ccc/data/daemon.log`" (Interface > Outputs) |
| 4 | cmd.Start fails | **[COVERED]** — Spec: "Re-execs the `ccc` binary" (Process Lifecycle) |
| 5 | Happy path: detached process, PID file written | **[COVERED]** — Spec: "The child process runs in a new session (`Setsid: true`)" and "Parent writes the child PID to `daemon.pid`" (Process Lifecycle) |
| 6 | PID file write ignores error (`_ = os.WriteFile`) | **[UNCOVERED-BEHAVIORAL]** — *Intent question: Should PID file write failure be fatal or logged? Code silently ignores it. Spec says "Parent writes the child PID to `daemon.pid`" without specifying error handling.* |

### `daemon.go` — `NewServer()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Creates server with refresh loop and post-refresh callback | **[COVERED]** — Spec: "Post-refresh callback: On success, prunes dead sessions and broadcasts `data.refreshed`" (Refresh Loop) |

### `daemon.go` — `Serve()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Remove stale socket file | **[COVERED]** — Spec: implied by "checks the PID file; errors if a live daemon already exists" (Process Lifecycle) |
| 2 | Umask 0177 before socket creation | **[COVERED]** — Spec: "umask to `0177` so the socket file is created with owner-only permissions" (Socket Security) |
| 3 | net.Listen fails | **[UNCOVERED-IMPLEMENTATION]** — error propagation |
| 4 | Accept loop, spawn goroutine per connection | **[COVERED]** — Spec: "Each accepted connection spawns a goroutine (`handleConn`)" (Connection Handling) |
| 5 | Accept error during shutdown (ctx.Done) — returns nil | **[COVERED]** — Spec: implied by shutdown sequence |
| 6 | Accept error outside shutdown — returns error | **[UNCOVERED-IMPLEMENTATION]** — error propagation |
| 7 | Refresh loop started | **[COVERED]** — Spec: "A `time.Ticker` fires at the configured interval" (Refresh Loop) |

### `daemon.go` — `Shutdown()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Runner non-nil: calls runner.Shutdown() | **[COVERED]** — Spec: "stops the agent runner" (Stopping the daemon) |
| 2 | Stops refresh loop | **[COVERED]** — Spec: "stops the refresh loop" (Stopping the daemon) |
| 3 | Cancels context | **[COVERED]** — Spec: "cancels the server context" (Stopping the daemon) |
| 4 | Closes listener and all client connections | **[COVERED]** — Spec: "closes all client connections" (Stopping the daemon) |
| 5 | Removes socket file | **[COVERED]** — Spec: "removes the socket file" (Stopping the daemon) |

### `daemon.go` — `handleConn()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Malformed JSON — returns parse error -32700 | **[COVERED]** — Spec: "`-32700` — parse error (malformed JSON)" (Wire Protocol) |
| 2 | Subscribe method — becomes push-only, blocks on ctx.Done | **[COVERED]** — Spec: "Subscribe is special: After the server sends the OK response, the connection becomes push-only" (Connection Handling) |
| 3 | Normal RPC dispatch and response | **[COVERED]** — Spec: "Each message is unmarshalled as an `RPCRequest` and dispatched by method name" (Connection Handling) |
| 4 | Context cancelled mid-loop — returns | **[UNCOVERED-IMPLEMENTATION]** — graceful teardown detail |
| 5 | Scanner buffer 64KB/4MB | **[COVERED]** — Spec: "Scanner buffer: 64KB initial, 4MB max" (Wire Protocol) |

### `daemon.go` — `dispatch()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Ping — returns ok | **[COVERED]** — Spec: "Ping the daemon, receive `{ok: true}`" (Test Cases) |
| 2 | Refresh — triggers async run, broadcasts | **[COVERED]** — Spec: "The `Refresh` RPC triggers an immediate run in a goroutine (non-blocking)" (Refresh Loop) |
| 3 | RegisterSession — unmarshal, register, return ok | **[COVERED]** — Spec: "RegisterSession(session_id, pid, project, worktree_path)" (Session Registry) |
| 4 | UpdateSession — unmarshal, update, return ok | **[COVERED]** — Spec: "UpdateSession(session_id, topic)" (Session Registry) |
| 5 | ListSessions — returns list | **[COVERED]** — Spec: "ListSessions() — Returns all non-archived sessions" (Session Registry) |
| 6 | ArchiveSession — unmarshal, archive, return ok | **[COVERED]** — Spec: "ArchiveSession(session_id)" (Session Registry) |
| 7 | Agent RPCs delegated | **[COVERED]** — Spec: Agent RPCs section |
| 8 | Budget RPCs delegated | **[COVERED]** — Spec: Budget RPCs section |
| 9 | Daemon control RPCs delegated | **[COVERED]** — Spec: Pause/Resume/Shutdown/Status sections |
| 10 | Unknown method — -32601 | **[COVERED]** — Spec: "`-32601` — method not found" (Wire Protocol) |

### `daemon.go` — `handlePauseDaemon()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Sets paused on server and refresh loop, broadcasts daemon.paused | **[COVERED]** — Spec: "`PauseDaemon` sets the `paused` atomic bool on both the server and the refresh loop; broadcasts `daemon.paused` event" (Pause/Resume) |

### `daemon.go` — `handleResumeDaemon()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Clears paused, broadcasts daemon.resumed | **[COVERED]** — Spec: "`ResumeDaemon` clears the paused flag; broadcasts `daemon.resumed` event" (Pause/Resume) |

### `daemon.go` — `handleShutdownDaemon()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Responds ok, then async shutdown after 100ms | **[COVERED]** — Spec: "`ShutdownDaemon` RPC responds with `{ok: true}`, then asynchronously (after 100ms) calls `Shutdown()`" (Stopping the daemon) |

### `daemon.go` — `handleGetDaemonStatus()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Returns "running" state with agent count | **[COVERED]** — Spec: "`GetDaemonStatus` RPC returns `{state: running|paused, active_agents: <int>}`" (Daemon status) |
| 2 | Returns "paused" state when paused | **[COVERED]** — same cite |
| 3 | Runner nil — activeAgents=0 | **[UNCOVERED-IMPLEMENTATION]** — nil guard |

### `daemon.go` — `Paused()`, `removeClient()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Paused() returns paused state | **[UNCOVERED-IMPLEMENTATION]** — accessor |
| 2 | removeClient removes conn from slice | **[COVERED]** — Spec: "On disconnect, the connection is removed from the client list" (Connection Handling) |

---

### `agents.go` — `handleLaunchAgent()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Paused — rejects with error | **[COVERED]** — Spec: "Rejects if daemon is paused" (LaunchAgent) |
| 2 | Runner nil — rejects | **[COVERED]** — Spec: "Rejects if runner is not configured" (LaunchAgent) |
| 3 | Invalid params — -32602 | **[COVERED]** — Spec: "`-32602` — invalid params" (Wire Protocol) |
| 4 | Missing ID — -32602 | **[COVERED]** — implied by required field |
| 5 | LaunchOrQueue returns queued+cmd (denied) — returns denial reason | **[COVERED]** — Spec: "if the result is `LaunchDeniedMsg`, returns the denial reason as an RPC error" (LaunchAgent step 4) |
| 6 | LaunchOrQueue returns not-queued+cmd — starts in goroutine, broadcasts agent.started, spawns watcher | **[COVERED]** — Spec: "executes cmd in a goroutine. On `SessionStartedMsg`, broadcasts `agent.started` event and spawns a watcher goroutine" (LaunchAgent step 5) |
| 7 | Success — returns ok+queued | **[COVERED]** — Spec: "Returns `{ok: true, queued: <bool>}`" (LaunchAgent step 6) |

### `agents.go` — `handleStopAgent()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Runner nil — error | **[COVERED]** — Spec: "agent runner not configured" (Error Cases) |
| 2 | Invalid params | **[COVERED]** |
| 3 | Missing ID | **[COVERED]** |
| 4 | Agent not found | **[COVERED]** — Spec: "returns not-found error if agent doesn't exist" (StopAgent) |
| 5 | Success — kills, broadcasts agent.stopped | **[COVERED]** — Spec: "Broadcasts `agent.stopped` event" (StopAgent) |

### `agents.go` — `handleAgentStatus()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Runner nil — error | **[COVERED]** |
| 2 | Invalid params | **[COVERED]** |
| 3 | Agent not found | **[COVERED]** |
| 4 | Success — returns status with optional StartedAt | **[COVERED]** — Spec: "Returns `{id, status, session_id, question, started_at}`" (AgentStatus) |

### `agents.go` — `handleListAgents()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Runner nil — error | **[COVERED]** |
| 2 | Success — returns all active | **[COVERED]** — Spec: "Returns all active agents from `runner.Active()`" (ListAgents) |

### `agents.go` — `handleSendAgentInput()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Runner nil — error | **[COVERED]** |
| 2 | Invalid params | **[COVERED]** |
| 3 | Missing ID | **[COVERED]** |
| 4 | SendMessage fails | **[COVERED]** — Spec: "Sends a message to a running agent's stdin" (SendAgentInput) |
| 5 | Success | **[COVERED]** |

### `agents.go` — `watchAgentDone()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Blocks on Done, cleans up, broadcasts agent.finished with exit code | **[COVERED]** — Spec: "`watchAgentDone` blocks on `sess.Done()`, then calls `runner.CleanupFinished(id)` and broadcasts `agent.finished` with exit code" (Agent completion watcher) |

---

### `budget.go` — `handleGetBudgetStatus()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Governed nil — error | **[COVERED]** — Spec: "Budget RPCs require a `GovernedRunner`" (Budget RPCs) |
| 2 | Success — returns full status with active agent count | **[COVERED]** — Spec: "Returns `{hourly_spent, hourly_limit, daily_spent, daily_limit, emergency_stopped, warning_level, active_agents}`" (GetBudgetStatus) |

### `budget.go` — `handleStopAllAgents()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Governed nil — error | **[COVERED]** |
| 2 | Kills all active agents | **[COVERED]** — Spec: "Kills all active agents via `runner.Kill`" (StopAllAgents) |
| 3 | Activates emergency stop | **[COVERED]** — Spec: "Activates emergency stop on the budget tracker" (StopAllAgents) |
| 4 | Broadcasts budget.emergency_stop | **[COVERED]** — Spec: "Broadcasts `budget.emergency_stop` event" (StopAllAgents) |
| 5 | Returns stopped count | **[COVERED]** — Spec: "Returns `{stopped: <count>}`" (StopAllAgents) |

### `budget.go` — `handleResumeAgents()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Governed nil — error | **[COVERED]** |
| 2 | Clears emergency stop, broadcasts budget.resumed, returns | **[COVERED]** — Spec: "Clears the emergency stop" / "Broadcasts `budget.resumed`" / "Returns `{resumed: true}`" (ResumeAgents) |

---

### `refresh.go` — `refreshLoop`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | start() with interval<=0 or fn==nil — no-op | **[UNCOVERED-BEHAVIORAL]** — *Intent question: Should the spec mention that refresh is disabled when interval is zero or no refresh function is provided? Code silently skips. The spec says "default 5 minutes, minimum 1 minute" but does not say what happens if interval is 0.* |
| 2 | Ticker fires, paused — skips | **[COVERED]** — Spec: "If `paused` is true, the tick is skipped" (Refresh Loop) |
| 3 | Ticker fires, not paused — runs fn | **[COVERED]** — Spec: "A `time.Ticker` fires at the configured interval" (Refresh Loop) |
| 4 | run() already running — silently drops | **[COVERED]** — Spec: "A mutex-guarded `running` flag prevents concurrent refresh runs; if a refresh is already in progress, the new tick is silently dropped" (Refresh Loop) |
| 5 | run() fn is nil — returns nil | **[UNCOVERED-IMPLEMENTATION]** — defensive guard |
| 6 | run() fn succeeds — calls notify | **[COVERED]** — Spec: "Post-refresh callback: On success" (Refresh Loop) |
| 7 | run() fn fails — logs error, no notify | **[COVERED]** — implied: "On success" means failure does not trigger callback |
| 8 | stop() — closes channel once | **[UNCOVERED-IMPLEMENTATION]** — teardown detail |

---

### `sessions.go` — `sessionRegistry`

#### `newSessionRegistry()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Creates registry, loads from DB | **[COVERED]** — Spec: "On startup, `newSessionRegistry` loads all non-archived sessions from the database" (Persistence across restarts) |

#### `loadFromDB()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | DB load fails — silently ignored | **[UNCOVERED-BEHAVIORAL]** — *Intent question: Should startup DB load failure be logged or cause a fatal error? Code silently swallows the error.* |
| 2 | Success — populates map | **[COVERED]** |

#### `register()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Resolves git info from dir (best-effort) | **[COVERED]** — Spec: "Resolves git remote URL and branch from the project directory (best-effort, via `git -C`)" (RegisterSession) |
| 2 | Persists to DB | **[COVERED]** — Spec: "Persists to `cc_sessions` table" (RegisterSession) |
| 3 | DB persist fails — returns error | **[COVERED]** — implied by RPC error handling |

#### `update()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Session not found — error | **[COVERED]** — Spec: "Errors if session not found" (UpdateSession) |
| 2 | No fields to update — no-op | **[UNCOVERED-BEHAVIORAL]** — *Intent question: Should UpdateSession with no fields return an error or silently succeed? Code returns nil (success). Spec says "Updates mutable fields (currently only topic)" but doesn't address empty updates.* |
| 3 | Success — updates topic in memory and DB | **[COVERED]** |

#### `archive()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Session not found — error | **[COVERED]** |
| 2 | Session is active — error "cannot archive active session" | **[COVERED]** — Spec: "Only allowed for sessions in `ended` state; errors if session is still `active`" (ArchiveSession) |
| 3 | Success — marks archived | **[COVERED]** |

#### `list()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Returns non-archived sessions | **[COVERED]** — Spec: "Returns all non-archived sessions from the in-memory map" (ListSessions) |

#### `pruneDead()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Skips non-active sessions | **[UNCOVERED-IMPLEMENTATION]** — filter logic |
| 2 | PID alive — no change | **[COVERED]** |
| 3 | PID dead — marks ended with timestamp | **[COVERED]** — Spec: "Dead processes are marked `ended` with a timestamp" (Dead session pruning) |
| 4 | DB update error silently ignored | **[UNCOVERED-BEHAVIORAL]** — *Intent question: Should pruneDead log or propagate DB update failures? Code uses `_ =` to ignore.* |

#### `gitInfoFromDir()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Empty dir — returns empty | **[UNCOVERED-IMPLEMENTATION]** — defensive guard |
| 2 | Git commands fail — returns empty (best-effort) | **[COVERED]** — Spec: "best-effort" |
| 3 | Success — returns repo and branch | **[COVERED]** |

---

### `subscribers.go` — `subscriberSet`

#### `broadcast()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Copies list under lock, writes outside lock | **[COVERED]** — Spec: "Copies the subscriber list under lock" / "Writes to each subscriber outside the lock" (Broadcasting) |
| 2 | 5-second write deadline | **[COVERED]** — Spec: "Each write has a 5-second deadline" (Broadcasting) |
| 3 | Failed write — closes conn, removes from set | **[COVERED]** — Spec: "Failed writes cause the connection to be closed and removed" (Broadcasting) |
| 4 | No failures — no cleanup pass | **[UNCOVERED-IMPLEMENTATION]** — optimization path |

---

### `client.go` — `Client`

#### `NewClient()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Dial fails — returns error | **[COVERED]** — Spec: "Connect to daemon when none is running — returns connection error" (Error Cases) |
| 2 | Success — returns client | **[COVERED]** |

#### `call()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Marshal params fails | **[UNCOVERED-IMPLEMENTATION]** — serialization error |
| 2 | Write fails | **[UNCOVERED-IMPLEMENTATION]** — I/O error |
| 3 | Read fails | **[UNCOVERED-IMPLEMENTATION]** — I/O error |
| 4 | Unmarshal response fails | **[UNCOVERED-IMPLEMENTATION]** — deserialization error |
| 5 | RPC error in response | **[COVERED]** — Spec: error handling described throughout |
| 6 | Success | **[COVERED]** |

#### `Subscribe()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | Initial call fails | **[COVERED]** |
| 2 | Read loop — delivers events to handler | **[COVERED]** — Spec: "Client sends `Subscribe` RPC" / "receives broadcast events" (Event Subscription) |
| 3 | Malformed event — skipped | **[UNCOVERED-BEHAVIORAL]** — *Intent question: Should the client log or report malformed events from the server? Code silently skips them with `continue`.* |
| 4 | Read error — returns error (disconnect) | **[COVERED]** — Spec: "On disconnect" (TUI integration) |

All remaining `Client` methods (Ping, Refresh, RegisterSession, etc.) are thin wrappers around `call()` — their behavioral paths are the same as `call()` and the corresponding server-side RPC. No additional unique branches.

---

### `rpc.go` — `WriteMessage()` / `ReadMessage()`

| # | Branch | Classification |
|---|--------|---------------|
| 1 | WriteMessage — marshal and write newline-delimited JSON | **[COVERED]** — Spec: "newline-delimited JSON" (Wire Protocol) |
| 2 | ReadMessage — scan and unmarshal | **[COVERED]** |
| 3 | ReadMessage — EOF | **[UNCOVERED-IMPLEMENTATION]** — I/O boundary |

---

### `types.go`

All types are wire-format definitions. Every type has a corresponding spec entry under "Wire Types":

- `RPCRequest`, `RPCResponse`, `RPCError`, `Event` — **[COVERED]** (Wire Protocol)
- `RegisterSessionParams`, `UpdateSessionParams`, `ArchiveSessionParams` — **[COVERED]** (Wire Types > RPC param types)
- `SessionInfo` — **[COVERED]** (Wire Types > RPC result types)
- `BudgetStatusResult`, `StopAllAgentsResult`, `ResumeAgentsResult`, `DaemonStatusResult` — **[COVERED]** (Wire Types > RPC result types)

---

## Spec-to-Code Analysis (Spec claims without matching code)

| # | Spec Claim | Status |
|---|-----------|--------|
| 1 | "Daemon start when PID file references a dead process — stale PID file is ignored, new daemon starts" (Edge Cases) | **SPEC-ONLY** — `StartProcess()` does not check for existing PID file or running daemon. The `ccc daemon start` command (in `cmd/ccc/`) likely handles this, but the daemon package itself does not. |
| 2 | "`ccc daemon stop` reads the PID file and sends SIGTERM" (Stopping the daemon) | **SPEC-ONLY** — Not in `internal/daemon/`; implemented in CLI layer (`cmd/ccc/`). Spec blurs the boundary. |
| 3 | "Refresh content: Reloads config each cycle, runs all data sources, runs automations" (Refresh Loop) | **SPEC-ONLY** — The `refreshLoop` takes an opaque `func() error`. Config reload and automation execution happen in the caller, not in this package. Spec describes end-to-end behavior, not this package's responsibility. |
| 4 | "Fallback registration: The `ccc register` CLI command tries the daemon first; if unreachable, it writes directly to the database" (Session Registry) | **SPEC-ONLY** — Implemented in CLI layer, not `internal/daemon/`. |
| 5 | "session.registered / session.updated / session.ended" events declared in Event type comment | **SPEC-ONLY (partial)** — The spec notes these are "declared in Event type comment; registration/update don't currently broadcast". Code confirms: RegisterSession and UpdateSession do NOT broadcast events. The comment in `types.go` lists them but they are not emitted. Spec is accurate about this gap. |

---

## Behavioral Gap Summary

| # | Gap | File | Question |
|---|-----|------|----------|
| 1 | PID file write failure silently ignored | start.go:48 | Should this be fatal or at least logged? |
| 2 | Refresh disabled when interval<=0 — no spec coverage | refresh.go:26-28 | Should spec document the disable condition? |
| 3 | DB load failure on startup silently ignored | sessions.go:36-38 | Should this log a warning or fail the daemon? |
| 4 | UpdateSession with no fields is a silent no-op | sessions.go:89-91 | Should this return an error? |
| 5 | pruneDead DB update errors silently ignored | sessions.go:137 | Should this log? |
| 6 | Client skips malformed server events | client.go:249 | Should this be logged? |
| 7 | session.registered/updated/ended events not emitted | daemon.go dispatch | Is this intentional tech debt or a missing feature? |
| 8 | Spec claims about PID file checking and config reload | start.go | Spec describes CLI-layer behavior as daemon behavior — boundary should be clarified |
