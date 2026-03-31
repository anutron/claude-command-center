# SPEC: Agent Subsystem

## Purpose

Manages headless Claude Code agent sessions from within CCC. Provides process lifecycle management (launch, kill, queue, monitor), cost tracking with budget enforcement, and rate limiting to prevent runaway automation spend. This subsystem was built in response to a runaway-agent incident (412 concurrent sessions, $1500 burned) and exists to make autonomous agent usage safe and observable.

## Interface

### Inputs

- **Request**: describes an agent to spawn
  - `ID` — unique identifier (e.g., todo ID), used for dedup and cooldown tracking
  - `Prompt` — initial text sent to the agent via PTY stdin
  - `ProjectDir` — working directory for the agent process
  - `Worktree` — if true, passes `--worktree` to `claude`
  - `Permission` — permission mode string (`"default"`, `"plan"`, `"auto"`)
  - `Budget` — max USD spend for this session; passed to Claude CLI if >= $0.50; also used for per-session budget enforcement via SIGINT
  - `ResumeID` — if set, resumes an existing Claude session instead of creating a new one
  - `AutoStart` — if true, auto-launch when dequeued
  - `Automation` — which automation triggered this (e.g., `"pr-review"`), used for rate limit scoping
  - `CostCallback` — optional callback invoked with `(inputTokens, outputTokens, costUSD)` on each usage event

### Outputs

- **Tea messages** emitted into the bubbletea event loop:
  - `SessionStartedMsg{ID, Session}` — process launched successfully
  - `SessionFinishedMsg{ID, ExitCode}` — process exited
  - `SessionIDCapturedMsg{ID, SessionID}` — Claude session UUID captured
  - `SessionBlockedMsg{ID, Question}` — agent waiting for user input (SendUserMessage/AskUser tool detected)
  - `SessionEventMsg{ID, Event}` — parsed event from agent stdout (assistant text, tool use, tool result, error, user, system)
  - `SessionEventsDoneMsg{ID}` — event channel closed
  - `LaunchDeniedMsg{ID, Reason}` — launch blocked by budget or rate limit (GovernedRunner only)

### Dependencies

- **Claude CLI** (`claude`) — must be on PATH; launched via PTY with `--verbose` and optional `--session-id`, `--resume`, `--permission-mode`, `--worktree` flags
- **SQLite database** (`cc_agent_costs`, `cc_budget_state` tables) — for cost tracking, budget state, and rate limit queries
- **`config.AgentConfig`** — budget limits, rate limit parameters, concurrency cap
- **`github.com/creack/pty`** — PTY allocation for agent processes
- **Claude native log files** (`~/.claude/projects/<encoded-path>/<session-id>.jsonl`) — source of truth for event parsing, token usage, and cost estimation. The encoded path is the project directory with all `/` replaced by `-` (including the leading slash, so `/Users/aaron/project` becomes `-Users-aaron-project`).

## Behavior

### Runner (core process lifecycle)

The `Runner` interface is the low-level session manager. `NewRunner(maxConcurrent)` creates a concrete `defaultRunner` (defaults to 10 if maxConcurrent <= 0).

**Launching:**

1. `LaunchOrQueue(req)` is called with a `Request`.
2. Dedup check: if the request ID is already active or queued, returns `(false, nil)` — silently ignored.
3. If under the concurrency limit, launches immediately via `launchSession`. Otherwise, appends to a FIFO queue and returns `(true, nil)`.
4. `launchSession` runs in a `tea.Cmd` goroutine:
   - Generates a UUID for the Claude session ID upfront.
   - Builds CLI args: `claude --verbose [--session-id UUID | --resume ID] [--permission-mode MODE] [--worktree]`.
   - Starts the process via PTY (`pty.Start`).
   - Drains PTY stdout to `/dev/null` (events come from the native log, not stdout).
   - Writes the prompt as plain text to the PTY.
   - Registers the session in the active map.
   - Starts `monitorSessionFromLog` in a goroutine.
   - Returns `SessionStartedMsg`.

**Monitoring (native log tailing):**

1. `tailNativeLog` polls the Claude native JSONL log file at `~/.claude/projects/<encoded-path>/<session-id>.jsonl`.
2. Waits up to 30 seconds for the file to appear (polling every 200ms).
3. Once open, reads JSONL lines and sends parsed `map[string]interface{}` events to a channel.
4. When no new lines are available, polls every 200ms.
5. `monitorSessionFromLog` consumes these events:
   - Serializes each event to CCC's own session log file and the session output buffer.
   - Parses events into `SessionEvent` structs and pushes them to `EventsCh` (buffered channel, capacity 64).
   - Detects blocking events: `tool_use` with name `SendUserMessage` or `AskUser` sets session status to `"blocked"`.
   - Extracts token usage from assistant events with `stop_reason` and invokes the `CostCallback`.
   - **Per-session budget enforcement:** if cumulative cost exceeds `Budget`, sends `SIGINT` to the process.
6. When the process exits, drains remaining log events for up to 2 seconds, then records the exit code and closes the `done` channel.

**Killing:**

- `Kill(id)` removes the session from the active map, closes the PTY (sends SIGHUP to the process group), then calls `Process.Kill()`.

**Shutdown:**

- `Shutdown()` closes all PTYs (SIGHUP), sends SIGINT to all processes, then waits up to 3 seconds per session for exit.

**Queue draining:**

- `DrainQueue()` pops the next queued request if there is capacity. Called by the host on tick.

**Cleanup:**

- `CleanupFinished(id)` removes a finished session from the active map, closes its PTY, and returns the session for summary extraction.

**Other:**

- `CheckProcesses()` polls active sessions for completion (via the `done` channel) and status changes. Returns batched tea messages for finished, blocked, and session-ID-captured events.
- `SendMessage(id, message)` writes to the PTY stdin and resets status from `"blocked"` to `"processing"`.
- `Watch(id)` returns a `tea.Cmd` that listens on the session's `EventsCh`.

### GovernedRunner (budget + rate limit enforcement)

`GovernedRunner` wraps a `Runner` and adds pre-launch checks. It implements the `Runner` interface so consumers are unaware of the governance layer.

**`LaunchOrQueue` flow:**

1. **Budget check** — calls `BudgetTracker.CanLaunch(budget)`. If denied, returns `LaunchDeniedMsg`.
2. **Rate limit check** — calls `RateLimiter.CanLaunch(id, automation)`. If denied, returns `LaunchDeniedMsg`.
3. **Record launch** — inserts a cost row via `BudgetTracker.RecordLaunch`, wires a `CostCallback` that calls `RecordCost` on each usage event.
4. **Delegate** — calls `inner.LaunchOrQueue(req)`.
5. If the inner runner queued it (concurrency limit), cleans up the cost row to avoid polluting budget accounting.

**`CleanupFinished` flow:**

1. Delegates to the inner runner to get the finished session.
2. Looks up the cost row ID, records duration and exit code via `RecordFinished`.

All other methods delegate directly to the inner runner.

### BudgetTracker

Tracks cumulative agent spend against rolling hourly and daily budget limits, backed by SQLite.

**State:**

- `hourlySpent` / `dailySpent` — cached in memory, refreshed from DB on every cost update.
- `stopped` — emergency stop flag, persisted in `cc_budget_state` table.

**`CanLaunch(budget)`** checks in order:

1. Emergency stop active? Deny.
2. `hourlySpent + budget > HourlyBudget`? Deny.
3. `dailySpent + budget > DailyBudget`? Deny.
4. Otherwise, allow.

**`RecordCost(rowID, inputTokens, outputTokens, costUSD)`:**

- Updates the `cc_agent_costs` row with running cost/token counts.
- Refreshes cached hourly/daily totals from DB.

**`RecordFinished(rowID, durationSec, exitCode, finalCostUSD)`:**

- Sets `finished_at`, `duration_sec`, `exit_code`, and `status` ("completed" or "failed") on the cost row.
- Refreshes cached totals.

**`EmergencyStop()` / `Resume()`:**

- Toggle the `stopped` flag in memory and persist to `cc_budget_state` with key `"emergency_stop"`.
- Emergency stop survives daemon restarts (loaded from DB on construction).

**Warning levels** (in `Status()`):

- `"critical"` — hourly spend >= 95% of limit
- `"warning"` — hourly spend >= `BudgetWarningPct` (configurable, e.g., 0.80)
- `"none"` — below thresholds

### RateLimiter

Prevents spawn loops via three checks. Fully stateless in memory; all state comes from DB queries, surviving daemon restarts.

**`CanLaunch(agentID, automation)` checks in order:**

1. **Per-automation hourly cap** — counts launches for this `automation` in the last hour. Default cap: 20. Skipped if `automation` is empty.
2. **Per-agent-ID cooldown** — checks time since last launch of this `agentID`. Default cooldown: 15 minutes. Blocks if elapsed time < cooldown.
3. **Failure backoff** — counts failures for this `automation` in the last hour. If > 0, applies exponential backoff: `min(baseSec * 2^(failures-1), maxSec)`. Default base: 60s, max: 3600s (1 hour). Skipped if `automation` is empty.

### Cost Estimation

Token usage is extracted from native log events that have `message.stop_reason` and `message.usage` fields. Cost is estimated from the model name:

- **Opus** (`model` contains "opus"): $15/M input, $75/M output
- **Sonnet** (default): $3/M input, $15/M output

### Session Event Parsing

Events from the native log are parsed into `SessionEvent` structs with types:

- `assistant_text` — text content from assistant messages
- `tool_use` — tool name, input (truncated to 80 chars), tool ID
- `tool_result` — result text, tool ID correlation, error flag
- `error` — error message
- `user` — user message text
- `system` — system messages (subtypes, session ID)

### Session Logging

Each session gets its own JSONL log file at `~/.config/ccc/data/session-logs/<timestamp>_<id>.jsonl`. Contains all parsed native log events plus start/exit markers.

### Database Schema

**`cc_agent_costs`:**

| Column | Type | Description |
|---|---|---|
| `id` | INTEGER PK | Auto-increment row ID |
| `agent_id` | TEXT | Request ID (e.g., todo ID) |
| `automation` | TEXT | Which automation triggered the launch |
| `started_at` | TEXT | ISO timestamp |
| `finished_at` | TEXT | ISO timestamp (NULL while running) |
| `duration_sec` | INTEGER | Wall-clock duration |
| `budget_usd` | REAL | Budgeted amount for this session |
| `cost_usd` | REAL | Actual/estimated cost (updated in real-time) |
| `input_tokens` | INTEGER | Cumulative input tokens |
| `output_tokens` | INTEGER | Cumulative output tokens |
| `cost_source` | TEXT | Always "estimate" currently |
| `exit_code` | INTEGER | Process exit code |
| `status` | TEXT | "running", "completed", "failed" |

Indexed on `started_at` for rolling-window budget queries.

**`cc_budget_state`:**

| Column | Type | Description |
|---|---|---|
| `key` | TEXT PK | State key (e.g., "emergency_stop") |
| `value_num` | REAL | Numeric value |
| `value_text` | TEXT | Text value |
| `updated_at` | TEXT | ISO timestamp |

## Configuration

All settings live under the `agent:` key in `~/.config/ccc/config.yaml` via `config.AgentConfig`:

| Setting | Default | Description |
|---|---|---|
| `max_concurrent` | 10 | Max simultaneous agent sessions |
| `default_budget` | — | Default USD budget per session |
| `default_permission` | — | Default permission mode |
| `hourly_budget` | — | Max spend per rolling hour |
| `daily_budget` | — | Max spend per rolling 24h |
| `budget_warning_pct` | — | Warn at this fraction of hourly budget (e.g., 0.80) |
| `max_launches_per_automation_per_hour` | 20 | Per-automation hourly launch cap |
| `cooldown_minutes` | 15 | Min time between launches of the same agent ID |
| `failure_backoff_base_seconds` | 60 | Initial failure backoff |
| `failure_backoff_max_seconds` | 3600 | Max failure backoff (1 hour) |

## Test Cases

### Happy path

- Launch a session under concurrency limit: returns `SessionStartedMsg`, process starts via PTY
- Launch when at concurrency limit: request is queued, `DrainQueue` returns it when capacity opens
- Budget check passes when spend + request < hourly and daily limits
- Rate limit passes when agent is not in cooldown and automation is under cap
- Cost callback updates token counts and USD in real-time as agent runs
- Session finishes with exit code 0: `SessionFinishedMsg` emitted, cost row marked "completed"
- Summary extraction pulls last assistant text or result text from session output

### Error cases

- PTY start fails: emits `SessionFinishedMsg` with exit code -1, logs error
- Emergency stop active: `CanLaunch` returns false, `LaunchDeniedMsg` emitted
- Hourly budget exceeded: `CanLaunch` returns false with spend breakdown
- Daily budget exceeded: same pattern
- Automation hits hourly launch cap: denied with count/cap info
- Agent ID in cooldown: denied with remaining time
- Failure backoff active: denied with failure count and remaining backoff time
- Per-session budget exceeded: SIGINT sent to process during monitoring
- Process exits with non-zero code: cost row marked "failed", exit code recorded

### Edge cases

- Duplicate request ID (already active or queued): silently ignored, returns `(false, nil)`
- Native log file does not appear within 30 seconds: monitoring goroutine exits, session runs blind
- `NativeLogPath` encodes project dir with leading `-` (e.g., `/Users/aaron/project` → `-Users-aaron-project`) matching Claude CLI's actual directory naming; incorrect encoding causes session viewer to show "Waiting for events..." indefinitely (BUG-122)
- Event channel full (64 capacity): events dropped silently (non-blocking send)
- Process exits before `CheckProcesses` runs: `SessionIDCapturedMsg` emitted before `SessionFinishedMsg` to ensure session ID is persisted
- Inner runner queues a governed launch: cost row is immediately cleaned up to avoid phantom budget consumption
- Emergency stop state survives daemon restart (persisted in `cc_budget_state`)
- Rate limiter is fully stateless in memory; survives restarts via DB queries
- Kill closes PTY first (SIGHUP to process group) then calls Process.Kill
- Shutdown sends SIGINT (graceful) not SIGKILL; waits up to 3 seconds per session
