# Design: Daemon Agent Observability

## Problem

After migrating agent management to the daemon, five observability features broke because they relied on local session state that no longer exists for daemon-managed agents:

1. Session viewer shows "Waiting for events..." (no event source)
2. `w` key fails ("No active session for this todo")
3. Console overlay shows $0 cost / 0 tokens (data exists in DB but isn't re-read)
4. Session ID is empty in console detail (daemon captures it but never broadcasts it)
5. TODO list/detail missing spinner and `w watch` hint for running daemon agents

## Root Cause

The session viewer, console overlay, and key handlers were built for local agents only — they call `agentRunner.Session(id)` which returns nil for daemon-managed sessions. The daemon has all the data (events, session ID, costs) but no mechanism to push it to the TUI beyond the existing `agent.started`/`agent.finished` broadcasts.

## Design

### 2a. `w` Key: Daemon Event Bridge

**Current:** `w` key checks `agentRunner.Session(id)` → nil for daemon agents → "No active session".

**Fix:** When no local session exists, check daemon via `AgentStatus` RPC. If agent is active, open session viewer with a new `listenForDaemonAgentEvents` tea.Cmd:

- Polls `StreamAgentOutput` RPC every 500ms
- Tracks offset (events already seen)
- Emits `sessionEventMsg` for each new event (same type as local path)
- Emits `sessionEventsDoneMsg` when `Done == true`, stops polling

Session viewer code unchanged — receives same message types regardless of source.

**Files:** `cc_keys_detail.go` (w key fallback), new `cc_daemon_events.go` (polling cmd)

### 2b. Console Overlay: Periodic Cost Refresh

**Current:** Console entries only refresh on `agent.started`/`agent.finished`/`agent.stopped` events.

**Fix:** When console is visible and any entry has status "running"/"processing", re-fetch `ListAgentHistory` on tick every 2 seconds. Cost data is already in DB via `RecordCost` — just needs more frequent reads.

**Files:** `internal/tui/model.go` (tick handler), `internal/tui/console_overlay.go` (add running-check helper)

### 2c. Session ID Propagation

**Current:** Daemon captures session ID via stream-json parser but doesn't broadcast it. Console shows empty Session ID.

**Fix:**

1. Daemon broadcasts `agent.session_id` event when UUID is captured. Payload: `{"id": "<agent_id>", "session_id": "<uuid>"}`.
2. CC plugin handles `agent.session_id` NotifyMsg — updates in-memory todo's `SessionID` and persists to DB.

This also fixes the console overlay since `ListAgentHistory` joins on the session data.

**Files:** `internal/daemon/agents.go` (broadcast), `internal/builtin/commandcenter/cc_messages.go` (handler)

### 2d. TODO Detail Spinner for Daemon Agents

**Current:** Spinner animation may be gated on local session existence rather than todo status.

**Fix:** Spinner condition checks `todo.Status == "running"`, not whether a local session exists.

**Files:** `cc_view_detail.go` (spinner condition)

### 2e. Running Agent Indicators

**Current:** `w watch` hint and spinner in TODO list/detail may only appear when local session exists.

**Fix:**

- Hint bar includes `w watch` whenever todo status is "running" (not gated on local session)
- List row spinner advances whenever any todo has "running" status

**Files:** `cc_view_detail.go` (hint bar), `cc_view.go` (spinner tick condition)
