# Design: Daemon Agent Observability

## Problem

After migrating agent management to the daemon, five observability features broke because they relied on local session state that no longer exists for daemon-managed agents:

1. Session viewer shows "Waiting for events..." (no event source)
2. `w` key fails ("No active session for this todo")
3. Console overlay shows $0 cost / 0 tokens (data exists in DB but isn't re-read)
4. Session ID is empty in console detail (daemon captures it but never broadcasts it)
5. TODO list/detail missing spinner and `w watch` hint for running daemon agents

Additionally, the dual-path architecture (daemon + local fallback) is the root cause of these bugs — every feature needs two code paths, and the local fallback is essentially untested in practice since the daemon auto-starts.

## Root Cause

The CC plugin maintains a local `agentRunner` as a fallback for when the daemon is unavailable. Every agent feature (launch, watch, finish callback, cost tracking) must handle both paths. The daemon path was bolted on to existing local-runner code, but observability features were never wired up for it.

## Design Principles

1. **Daemon is required for agents.** No local fallback. If daemon is down, agent operations fail with a clear error. The daemon auto-starts from the TUI and auto-restarts on binary update — if it can't start, something is fundamentally wrong.

2. **Event-driven, not polling.** The daemon broadcasts events that all TUI subscribers receive. Multiple TUI instances see the same updates simultaneously. No per-consumer polling logic.

## Design

### 1. Remove Local Agent Fallback

**Current:** `launchOrQueueAgent` tries daemon RPC first, falls back to local `agentRunner.LaunchOrQueue()`. `checkAgentProcesses` polls local runner on tick. `w` key checks local session first. `activeAgentCount`/`queuedAgentCount` fall back to local runner.

**Fix:** Remove all local-runner fallback paths from the CC plugin:

- `launchOrQueueAgent` — daemon RPC only. If daemon not connected, flash error "Daemon not connected — cannot launch agent" instead of silently falling back.
- `checkAgentProcesses` — remove entirely. Daemon handles lifecycle via events.
- `activeAgentCount` — daemon RPC only (already the primary path). Error → return 0.
- `queuedAgentCount` — daemon RPC only (currently always uses local runner with a comment "daemon doesn't expose queue length yet"). Add a `QueueLen` RPC or include queue count in `GetBudgetStatus`.
- `w` key — check daemon via `AgentStatus` RPC. No local session branch.

The CC plugin **keeps** a reference to `agentRunner` for one purpose: the daemon itself uses it internally. The CC plugin just stops calling it directly.

**PR plugin:** Same treatment — `evaluateAgentTriggers` (currently disabled) and manual launch paths use daemon only.

**Files:** `cc_keys_detail.go`, `agent_runner.go`, `cc_messages.go` (remove `checkAgentProcesses`, `SessionFinishedMsg`/`SessionStartedMsg` handlers become dead code — daemon events replace them)

### 2. New Daemon Event: `agent.cost_updated`

**Current:** `RecordCost` is called on every API response during agent execution, updating the DB. But no event is broadcast, so TUI consumers (console overlay, budget widget) stay stale until the agent finishes.

**Fix:** The daemon broadcasts `agent.cost_updated` when `RecordCost` fires, throttled to at most once per 2 seconds per agent. Payload:

```json
{
  "id": "<agent_id>",
  "cost_usd": 0.42,
  "input_tokens": 12500,
  "output_tokens": 3200
}
```

**Throttle mechanism:** The `GovernedRunner`'s cost callback wrapper tracks `lastBroadcast` per agent. If <2s since last broadcast, skip. This keeps event rate manageable (~30 events/minute per agent max).

**TUI consumers:**

- **Console overlay:** On `agent.cost_updated`, if overlay is visible, re-fetch `ListAgentHistory` to get full updated entries. This is a targeted refresh (only when there's actually new data), not a blind timer.
- **Budget widget:** On `agent.cost_updated`, re-fetch `GetBudgetStatus`. All TUI instances see the same update.
- **Future consumers:** Get updates for free by handling the NotifyMsg.

**Files:** `internal/daemon/agents.go` (broadcast in cost callback), `internal/agent/governed_runner.go` (throttle wrapper), `internal/tui/model.go` (handle event), `internal/tui/console_overlay.go` (refresh on event)

### 3. New Daemon Event: `agent.session_id`

**Current:** Daemon captures session ID via stream-json parser but doesn't broadcast it. Console shows empty Session ID.

**Fix:**

1. Daemon broadcasts `agent.session_id` event when the Claude session UUID is captured. Payload: `{"id": "<agent_id>", "session_id": "<uuid>"}`.
2. CC plugin handles `agent.session_id` NotifyMsg — updates in-memory todo's `SessionID` and persists to DB.
3. PR plugin handles it too — updates `agent_session_id` on the PR row.

This also fixes the console overlay since `ListAgentHistory` joins on session data.

**Implementation:** After the `agent.started` broadcast in `handleLaunchAgent`, spawn a goroutine that polls the session's `SessionID` field (under lock) every 200ms until non-empty or done. When captured, broadcast the event. This is simpler than modifying the agent runner's internal monitoring — the runner already sets `sess.SessionID`, we just need to observe it.

**Files:** `internal/daemon/agents.go` (watcher goroutine + broadcast), `internal/builtin/commandcenter/cc_messages.go` (NotifyMsg handler), `internal/builtin/prs/prs.go` (NotifyMsg handler)

### 4. `w` Key: Daemon Event Bridge

**Current:** `w` key checks `agentRunner.Session(id)` → nil for daemon agents → "No active session".

**Fix:** `w` key checks daemon via `AgentStatus` RPC. If agent is active, open the session viewer with a new `listenForDaemonAgentEvents` tea.Cmd:

- On first call, `StreamAgentOutput` RPC returns the full event buffer snapshot (all events accumulated so far). These are loaded as the initial batch.
- Sets offset to the count of events received.
- Polls every 500ms; on each poll, only processes events beyond the offset.
- Emits `sessionEventMsg` for each new event (same type as local path).
- Emits `sessionEventsDoneMsg` when `Done == true`, stops polling.

If daemon is not connected, falls back to saved log on disk (for reviewing completed sessions). No local session branch.

Session viewer code unchanged — receives same message types regardless of source.

**Files:** `cc_keys_detail.go` (simplified w key handler), new `cc_daemon_events.go` (polling cmd)

### 5. TODO Spinner and `w watch` Hint

**Current:** Spinner and hint bar may be gated on local session existence.

**Fix:**

- Spinner condition checks `todo.Status == "running"`, not whether a local session exists.
- Hint bar includes `w watch` whenever todo status is "running".
- List row spinner advances whenever any todo has "running" status.

**Files:** `cc_view_detail.go` (spinner + hint), `cc_view.go` (spinner tick condition)

## Event Summary

After this work, the daemon broadcasts these agent lifecycle events:

| Event | When | Payload |
|-------|------|---------|
| `agent.started` | Process launched | `{id, status, started_at}` |
| `agent.session_id` | Claude session UUID captured | `{id, session_id}` |
| `agent.cost_updated` | RecordCost called (throttled 2s) | `{id, cost_usd, input_tokens, output_tokens}` |
| `agent.finished` | Process exited | `{id, exit_code}` |
| `agent.stopped` | Agent killed | `{id}` |

All TUI instances subscribing to the daemon receive all events. The CC plugin, PR plugin, console overlay, and budget widget all react via the existing `NotifyMsg` dispatch pattern.

## Migration Notes

- The local `agentRunner` field stays on the CC plugin struct but is no longer called for launch/watch/status. It can be removed in a follow-up cleanup.
- `SessionStartedMsg`, `SessionFinishedMsg`, `SessionIDCapturedMsg`, `SessionBlockedMsg` handlers in the CC plugin become dead code (replaced by NotifyMsg handlers). Remove them.
- `checkAgentProcesses()` tick handler is removed entirely.
- Existing tests that use `agent.SessionFinishedMsg` directly need updating to use `plugin.NotifyMsg` with daemon event payloads instead.
