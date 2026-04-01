# Daemon Agent Observability — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make daemon-managed agents fully observable: live events in session viewer, live costs in console/budget widget, session ID propagation, and remove local agent fallback.

**Architecture:** Daemon is the sole agent manager. New daemon events (`agent.session_id`, `agent.cost_updated`) propagate state to all TUI subscribers. Session viewer polls `StreamAgentOutput` RPC for live events. Local fallback paths are removed.

**Tech Stack:** Go, bubbletea, JSON-RPC over Unix socket, SQLite

---

### Task 1: Add `agent.cost_updated` daemon event with throttle

The daemon's `GovernedRunner` already calls `RecordCost` on every API response. We need to broadcast a throttled event when this happens, so TUI consumers get live cost data.

**Files:**
- Modify: `internal/agent/governed_runner.go` (add broadcast callback + throttle)
- Modify: `internal/daemon/agents.go` (wire broadcast into governed runner)
- Test: `internal/agent/governed_runner_test.go`
- Test: `internal/daemon/agents_test.go`

- [ ] **Step 1: Write failing test for throttled cost broadcast callback**

In `internal/agent/governed_runner_test.go`, add:

```go
func TestGovernedRunner_CostCallback_InvokesBroadcast(t *testing.T) {
	inner := &fakeRunner{}
	budget := agent.NewBudgetTracker(testDB(t), 10.0, 100.0)
	limiter := agent.NewRateLimiter(testDB(t))

	var broadcasts []float64
	g := agent.NewGovernedRunner(inner, budget, limiter)
	g.SetCostBroadcast(func(id string, inputTokens, outputTokens int, costUSD float64) {
		broadcasts = append(broadcasts, costUSD)
	})

	req := agent.Request{ID: "test-1", Prompt: "hello", Budget: 1.0}
	g.LaunchOrQueue(req)

	// Simulate cost callback being invoked (the callback was set on req.CostCallback)
	if inner.lastReq.CostCallback == nil {
		t.Fatal("expected CostCallback to be set")
	}
	inner.lastReq.CostCallback(100, 50, 0.01)
	if len(broadcasts) != 1 || broadcasts[0] != 0.01 {
		t.Errorf("expected 1 broadcast with cost 0.01, got %v", broadcasts)
	}
}

func TestGovernedRunner_CostBroadcast_ThrottledTo2Seconds(t *testing.T) {
	inner := &fakeRunner{}
	budget := agent.NewBudgetTracker(testDB(t), 10.0, 100.0)
	limiter := agent.NewRateLimiter(testDB(t))

	var broadcastCount int
	g := agent.NewGovernedRunner(inner, budget, limiter)
	g.SetCostBroadcast(func(id string, inputTokens, outputTokens int, costUSD float64) {
		broadcastCount++
	})

	req := agent.Request{ID: "test-2", Prompt: "hello", Budget: 1.0}
	g.LaunchOrQueue(req)

	cb := inner.lastReq.CostCallback
	// Rapid-fire 10 callbacks — only first should broadcast (rest within 2s window)
	for i := 0; i < 10; i++ {
		cb(100*i, 50*i, 0.01*float64(i))
	}
	if broadcastCount != 1 {
		t.Errorf("expected 1 broadcast (throttled), got %d", broadcastCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/ -run "TestGovernedRunner_Cost" -v`
Expected: FAIL — `SetCostBroadcast` method doesn't exist

- [ ] **Step 3: Implement `SetCostBroadcast` and throttled callback on GovernedRunner**

In `internal/agent/governed_runner.go`, add to the `GovernedRunner` struct:

```go
type CostBroadcastFunc func(id string, inputTokens, outputTokens int, costUSD float64)

// Add fields to GovernedRunner struct:
costBroadcast    CostBroadcastFunc
lastBroadcastAt  map[string]time.Time // per-agent throttle tracking
```

Add method:

```go
func (g *GovernedRunner) SetCostBroadcast(fn CostBroadcastFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.costBroadcast = fn
}
```

Modify the `LaunchOrQueue` cost callback wiring (around line 79):

```go
req.CostCallback = func(inputTokens, outputTokens int, costUSD float64) {
	g.budget.RecordCost(costRowID, inputTokens, outputTokens, costUSD)

	// Throttled broadcast: at most once per 2 seconds per agent.
	g.mu.Lock()
	fn := g.costBroadcast
	last := g.lastBroadcastAt[req.ID]
	shouldBroadcast := fn != nil && time.Since(last) >= 2*time.Second
	if shouldBroadcast {
		if g.lastBroadcastAt == nil {
			g.lastBroadcastAt = make(map[string]time.Time)
		}
		g.lastBroadcastAt[req.ID] = time.Now()
	}
	g.mu.Unlock()

	if shouldBroadcast {
		fn(req.ID, inputTokens, outputTokens, costUSD)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/ -run "TestGovernedRunner_Cost" -v`
Expected: PASS

- [ ] **Step 5: Wire daemon to broadcast `agent.cost_updated` on callback**

In `internal/daemon/agents.go`, in the `NewServer` or wherever the governed runner is set up, add after the runner is created:

```go
if governed, ok := s.runner.(*agent.GovernedRunner); ok {
	governed.SetCostBroadcast(func(id string, inputTokens, outputTokens int, costUSD float64) {
		data, _ := json.Marshal(map[string]interface{}{
			"id":            id,
			"cost_usd":      costUSD,
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		})
		s.Broadcast(Event{Type: "agent.cost_updated", Data: data})
	})
}
```

Find where the runner is assigned to the server (likely in `daemon.go` or `main.go` setup) and add this wiring there.

- [ ] **Step 6: Add `agent.cost_updated` to console overlay refresh triggers in `model.go`**

In `internal/tui/model.go`, line 571, change:

```go
case "agent.started", "agent.finished", "agent.stopped":
```

to:

```go
case "agent.started", "agent.finished", "agent.stopped", "agent.cost_updated":
```

- [ ] **Step 7: Add `agent.cost_updated` to budget widget refresh in `model.go`**

In the same `DaemonEventMsg` handler, after the console refresh block, add:

```go
// Refresh budget widget on cost updates.
if msg.Event.Type == "agent.cost_updated" || msg.Event.Type == "agent.started" || msg.Event.Type == "agent.finished" || msg.Event.Type == "agent.stopped" {
	if client := m.DaemonClient(); client != nil {
		cmds = append(cmds, fetchBudgetStatusCmd(client))
	}
}
```

Find the existing `fetchBudgetStatusCmd` pattern by grepping for `budgetStatusMsg`.

- [ ] **Step 8: Run full test suite**

Run: `make test`
Expected: All pass

- [ ] **Step 9: Commit**

```bash
git add internal/agent/governed_runner.go internal/agent/governed_runner_test.go internal/daemon/agents.go internal/tui/model.go
git commit -m "Add agent.cost_updated daemon event with throttled broadcast"
```

---

### Task 2: Add `agent.session_id` daemon event

The daemon captures the Claude session UUID via stream-json parsing but never broadcasts it. Add a watcher goroutine that polls the session's `SessionID` field and broadcasts when captured.

**Files:**
- Modify: `internal/daemon/agents.go` (add session ID watcher + broadcast)
- Modify: `internal/builtin/commandcenter/cc_messages.go` (handle NotifyMsg)
- Modify: `internal/builtin/prs/prs.go` (handle NotifyMsg)
- Test: `internal/builtin/commandcenter/commandcenter_test.go`
- Test: `internal/builtin/prs/prs_view_test.go`

- [ ] **Step 1: Write failing test for CC plugin handling `agent.session_id` NotifyMsg**

In `internal/builtin/commandcenter/commandcenter_test.go`, add:

```go
func TestDaemonAgentSessionID_UpdatesTodoSessionID(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].Status = db.StatusRunning

	msg := plugin.NotifyMsg{
		Event: "agent.session_id",
		Data:  []byte(`{"id":"t1","session_id":"uuid-abc-123"}`),
	}
	handled, _ := p.HandleMessage(msg)
	if !handled {
		t.Fatal("expected HandleMessage to handle agent.session_id NotifyMsg")
	}
	if p.cc.Todos[0].SessionID != "uuid-abc-123" {
		t.Errorf("expected session ID %q, got %q", "uuid-abc-123", p.cc.Todos[0].SessionID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/builtin/commandcenter/ -run "TestDaemonAgentSessionID" -v`
Expected: FAIL — `agent.session_id` event not handled

- [ ] **Step 3: Add `agent.session_id` handler to CC plugin NotifyMsg dispatch**

In `internal/builtin/commandcenter/cc_messages.go`, in the NotifyMsg switch (around line 187), add a new case:

```go
case "agent.session_id":
	return p.handleDaemonAgentSessionID(nm.Data)
```

Add the handler function:

```go
func (p *Plugin) handleDaemonAgentSessionID(data []byte) (bool, plugin.Action) {
	var payload struct {
		ID        string `json:"id"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.ID == "" || payload.SessionID == "" {
		return false, plugin.NoopAction()
	}

	// Update in-memory todo.
	if p.cc != nil {
		for i := range p.cc.Todos {
			if p.cc.Todos[i].ID == payload.ID {
				p.cc.Todos[i].SessionID = payload.SessionID
				break
			}
		}
	}

	// Persist to DB.
	return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBUpdateTodoSessionID(database, payload.ID, payload.SessionID)
	})}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/builtin/commandcenter/ -run "TestDaemonAgentSessionID" -v`
Expected: PASS

- [ ] **Step 5: Write failing test for PR plugin handling `agent.session_id`**

In `internal/builtin/prs/prs_view_test.go`, add:

```go
func TestDaemonAgentSessionID_UpdatesPRSessionID(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "PR", Category: CategoryReview,
			LastActivityAt: recentTime(), AgentStatus: "running", HeadSHA: "abc"},
	})

	msg := plugin.NotifyMsg{
		Event: "agent.session_id",
		Data:  []byte(`{"id":"o/r#1","session_id":"uuid-pr-456"}`),
	}
	handled, _ := p.HandleMessage(msg)
	if !handled {
		t.Fatal("expected HandleMessage to handle agent.session_id NotifyMsg for PR")
	}

	for _, pr := range p.prs {
		if pr.ID == "o/r#1" {
			if pr.AgentSessionID != "uuid-pr-456" {
				t.Errorf("expected session ID %q, got %q", "uuid-pr-456", pr.AgentSessionID)
			}
			return
		}
	}
	t.Fatal("PR not found")
}
```

- [ ] **Step 6: Add `agent.session_id` handler to PR plugin**

In `internal/builtin/prs/prs.go`, in the NotifyMsg switch, add:

```go
case "agent.session_id":
	return p.handleDaemonAgentSessionID(msg.Data)
```

Add handler:

```go
func (p *Plugin) handleDaemonAgentSessionID(data []byte) (bool, plugin.Action) {
	var payload struct {
		ID        string `json:"id"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.ID == "" {
		return false, plugin.NoopAction()
	}
	if !p.isPRAgent(payload.ID) {
		return false, plugin.NoopAction()
	}
	if err := db.DBUpdatePRAgentStatus(p.database, payload.ID,
		"", payload.SessionID, "", "", ""); err != nil {
		p.logger.Error("prs", fmt.Sprintf("daemon agent session_id update failed for %s: %v", payload.ID, err))
	}
	p.updatePRSessionID(payload.ID, payload.SessionID)
	return true, plugin.NoopAction()
}
```

Note: check that `DBUpdatePRAgentStatus` with empty `status` doesn't overwrite the existing status. If it does, add a `DBUpdatePRAgentSessionID` function or adjust the query.

- [ ] **Step 7: Run PR plugin tests**

Run: `go test ./internal/builtin/prs/ -run "TestDaemonAgentSessionID" -v`
Expected: PASS

- [ ] **Step 8: Add session ID watcher goroutine to daemon**

In `internal/daemon/agents.go`, add a new function:

```go
// watchAgentSessionID polls the session for its Claude session UUID and
// broadcasts agent.session_id when captured. Stops when the session is done.
func (s *Server) watchAgentSessionID(id string, sess *agent.Session) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sess.Done():
			return
		case <-ticker.C:
			sess.Mu.Lock()
			sid := sess.SessionID
			sess.Mu.Unlock()
			if sid != "" {
				data, _ := json.Marshal(map[string]interface{}{
					"id":         id,
					"session_id": sid,
				})
				s.Broadcast(Event{Type: "agent.session_id", Data: data})
				return
			}
		}
	}
}
```

Call it from the launch goroutine in `handleLaunchAgent` (after `agent.started` broadcast, alongside `watchAgentDone`):

```go
go s.watchAgentSessionID(started.ID, started.Session)
go s.watchAgentDone(started.ID, started.Session)
```

Do the same in `drainNextAgent`.

- [ ] **Step 9: Run full test suite**

Run: `make test`
Expected: All pass

- [ ] **Step 10: Commit**

```bash
git add internal/daemon/agents.go internal/builtin/commandcenter/cc_messages.go internal/builtin/commandcenter/commandcenter_test.go internal/builtin/prs/prs.go internal/builtin/prs/prs_view_test.go
git commit -m "Add agent.session_id daemon event with watcher goroutine"
```

---

### Task 3: Wire `w` key to daemon `StreamAgentOutput`

Create `listenForDaemonAgentEvents` that polls `StreamAgentOutput` RPC every 500ms, emitting the same `agentEventMsg` types as local sessions. Update the `w` key handler to use daemon when no local session exists.

**Files:**
- Create: `internal/builtin/commandcenter/cc_daemon_events.go`
- Modify: `internal/builtin/commandcenter/cc_keys_detail.go` (w key handler)
- Modify: `internal/builtin/commandcenter/cc_messages.go` (handleAgentEvent re-listen path)
- Test: `internal/builtin/commandcenter/commandcenter_test.go`

- [ ] **Step 1: Create `cc_daemon_events.go` with the daemon event polling cmd**

Create `internal/builtin/commandcenter/cc_daemon_events.go`:

```go
package commandcenter

import (
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

// listenForDaemonAgentEvents polls the daemon's StreamAgentOutput RPC and
// emits agentEventMsg for each new event. Stops when the session is done.
func listenForDaemonAgentEvents(todoID string, client *daemon.Client, offset int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(500 * time.Millisecond)

		result, err := client.StreamAgentOutput(todoID)
		if err != nil {
			// Connection lost — treat as done.
			return agentEventsDoneMsg{todoID: todoID}
		}

		if len(result.Events) > offset {
			// Return the next new event. The handler will re-invoke this cmd
			// with offset+1 to get subsequent events.
			ev := result.Events[offset]
			return daemonAgentEventMsg{
				todoID: todoID,
				event:  ev,
				offset: offset + 1,
				done:   result.Done && offset+1 >= len(result.Events),
			}
		}

		if result.Done {
			return agentEventsDoneMsg{todoID: todoID}
		}

		// No new events yet — re-poll with same offset.
		return daemonAgentPollMsg{todoID: todoID, offset: offset}
	}
}

// daemonAgentEventMsg carries a single event from a daemon-managed agent.
type daemonAgentEventMsg struct {
	todoID string
	event  sessionEvent
	offset int
	done   bool
}

// daemonAgentPollMsg signals that no new events were found; continue polling.
type daemonAgentPollMsg struct {
	todoID string
	offset int
}
```

- [ ] **Step 2: Add message handlers for the new daemon event types in `cc_messages.go`**

In `internal/builtin/commandcenter/cc_messages.go`, add case blocks in `HandleMessage`:

```go
case daemonAgentEventMsg:
	if p.sessionViewerActive && p.sessionViewerTodoID == msg.todoID {
		p.updateSessionViewerContent()
	}
	if msg.done {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: func() tea.Msg {
			return agentEventsDoneMsg{todoID: msg.todoID}
		}}
	}
	// Continue polling for more events.
	if dc := p.daemonClient(); dc != nil {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: listenForDaemonAgentEvents(msg.todoID, dc, msg.offset)}
	}
	return true, plugin.NoopAction()

case daemonAgentPollMsg:
	// No new events — continue polling.
	if dc := p.daemonClient(); dc != nil && p.sessionViewerActive && p.sessionViewerTodoID == msg.todoID {
		return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: listenForDaemonAgentEvents(msg.todoID, dc, msg.offset)}
	}
	return true, plugin.NoopAction()
```

- [ ] **Step 3: Update `buildSessionViewerContent` to also check daemon events**

In `internal/builtin/commandcenter/cc_view_session.go`, the `buildSessionViewerContent` function (line 195) currently only reads from local sessions or replay events. For daemon sessions, the events arrive via `daemonAgentEventMsg` which calls `updateSessionViewerContent`. We need to make the content builder work with daemon events too.

The simplest approach: accumulate daemon events into `sessionViewerReplayEvents` as they arrive. In the `daemonAgentEventMsg` handler:

```go
case daemonAgentEventMsg:
	// Accumulate event into replay buffer for the viewer.
	p.sessionViewerReplayEvents = append(p.sessionViewerReplayEvents, msg.event)
	if p.sessionViewerActive && p.sessionViewerTodoID == msg.todoID {
		p.updateSessionViewerContent()
	}
	// ... rest of handler
```

- [ ] **Step 4: Update `w` key handler to check daemon**

In `internal/builtin/commandcenter/cc_keys_detail.go`, replace the `w` case (lines 140-165) with:

```go
case "w":
	if todo := p.detailTodo(); todo != nil {
		// Check daemon for active session.
		if dc := p.daemonClient(); dc != nil {
			if status, err := dc.AgentStatus(todo.ID); err == nil && (status.Status == "processing" || status.Status == "blocked") {
				p.initSessionViewer(todo.ID)
				p.sessionViewerReplayEvents = nil // clear stale replay
				if !p.sessionViewerListening {
					p.sessionViewerListening = true
					// Load initial event snapshot, then start polling.
					return plugin.Action{Type: plugin.ActionNoop, TeaCmd: listenForDaemonAgentEvents(todo.ID, dc, 0)}
				}
				return plugin.ConsumedAction()
			}
		}
		// No active session — try saved log on disk.
		if todo.SessionLogPath != "" {
			if err := p.initSessionViewerFromLog(todo.ID, todo.SessionLogPath); err != nil {
				p.flashMessage = fmt.Sprintf("Cannot open session log: %v", err)
				p.flashMessageAt = time.Now()
				return plugin.ConsumedAction()
			}
			return plugin.ConsumedAction()
		}
		p.flashMessage = "No active session for this todo"
		p.flashMessageAt = time.Now()
	}
	return plugin.ConsumedAction()
```

- [ ] **Step 5: Run full test suite**

Run: `make test`
Expected: All pass

- [ ] **Step 6: Manual smoke test**

1. Launch CCC, create a todo, launch agent via `o`
2. Press `w` — should see live events streaming
3. Open `~` console — should see cost/tokens updating
4. Budget widget should update live

- [ ] **Step 7: Commit**

```bash
git add internal/builtin/commandcenter/cc_daemon_events.go internal/builtin/commandcenter/cc_keys_detail.go internal/builtin/commandcenter/cc_messages.go internal/builtin/commandcenter/cc_view_session.go
git commit -m "Wire w key and session viewer to daemon StreamAgentOutput RPC"
```

---

### Task 4: TODO spinner and `w watch` hint based on status

Change `hasActiveSession` to check todo status instead of local session existence.

**Files:**
- Modify: `internal/builtin/commandcenter/cc_view_detail.go:25` (hasActiveSession check)
- Test: `internal/builtin/commandcenter/commandcenter_view_test.go`

- [ ] **Step 1: Write failing view test**

In `internal/builtin/commandcenter/commandcenter_view_test.go`, add:

```go
func TestView_DetailSpinnerShownForDaemonRunningTodo(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].Status = db.StatusRunning
	// No local session — this simulates a daemon-managed agent.
	// The spinner and "w watch" hint should still appear.

	p.detailView = true
	p.detailTodoID = p.cc.Todos[0].ID

	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "Agent running")
	testutil.AssertViewContains(t, view, "w watch")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/builtin/commandcenter/ -run "TestView_DetailSpinnerShownForDaemonRunningTodo" -v`
Expected: FAIL — "Agent running" not found (because `hasActiveSession` is false without local session)

- [ ] **Step 3: Change `hasActiveSession` to check todo status**

In `internal/builtin/commandcenter/cc_view_detail.go`, line 25, change:

```go
hasActiveSession := p.agentRunner.Session(todo.ID) != nil
```

to:

```go
hasActiveSession := todo.Status == db.StatusRunning || todo.Status == "blocked"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/builtin/commandcenter/ -run "TestView_DetailSpinnerShownForDaemonRunningTodo" -v`
Expected: PASS

- [ ] **Step 5: Run full test suite to check for regressions**

Run: `make test`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/builtin/commandcenter/cc_view_detail.go internal/builtin/commandcenter/commandcenter_view_test.go
git commit -m "Show spinner and w watch hint based on todo status, not local session"
```

---

### Task 5: Remove local agent fallback from CC plugin

Now that all daemon paths are working, remove the local fallback code. This is the largest task — take care with each removal.

**Files:**
- Modify: `internal/builtin/commandcenter/agent_runner.go` (simplify to daemon-only)
- Modify: `internal/builtin/commandcenter/cc_messages.go` (remove local runner message handlers, remove checkAgentProcesses)
- Test: `internal/builtin/commandcenter/commandcenter_test.go` (update tests to use NotifyMsg)

- [ ] **Step 1: Simplify `launchOrQueueAgent` to daemon-only**

In `internal/builtin/commandcenter/agent_runner.go`, replace `launchOrQueueAgent` (lines 175-222):

```go
func (p *Plugin) launchOrQueueAgent(qs queuedSession) tea.Cmd {
	p.cc.AcceptTodo(qs.TodoID)
	acceptCmd := p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBAcceptTodo(database, qs.TodoID)
	})

	dc := p.daemonClient()
	if dc == nil {
		p.flashMessage = "Daemon not connected — cannot launch agent"
		p.flashMessageAt = time.Now()
		return acceptCmd
	}

	params := qs.toDaemonParams()
	if err := dc.LaunchAgent(params); err != nil {
		p.flashMessage = fmt.Sprintf("Agent launch failed: %v", err)
		p.flashMessageAt = time.Now()
		return acceptCmd
	}

	p.setTodoStatus(qs.TodoID, db.StatusRunning)
	p.publishEvent("agent.started", map[string]interface{}{
		"todo_id": qs.TodoID,
	})
	return tea.Batch(acceptCmd, p.persistTodoStatus(qs.TodoID, db.StatusRunning), agentStateChangedCmd())
}
```

- [ ] **Step 2: Simplify `activeAgentCount` to daemon-only**

```go
func (p *Plugin) activeAgentCount() int {
	if dc := p.daemonClient(); dc != nil {
		agents, err := dc.ListAgents()
		if err == nil {
			return len(agents)
		}
	}
	return 0
}
```

- [ ] **Step 3: Simplify `queuedAgentCount` to use daemon**

```go
func (p *Plugin) queuedAgentCount() int {
	// Queue is managed daemon-side. Not exposed as a separate count yet.
	// TODO: add QueueLen to budget status or a new RPC.
	return 0
}
```

- [ ] **Step 4: Remove `checkAgentProcesses` function**

Delete the function entirely from `agent_runner.go` (lines 273-279).

- [ ] **Step 5: Remove `checkAgentProcesses` call from tick handler**

In `cc_messages.go`, in `handleTickMsg`, remove lines 1026-1029:

```go
	// Check for finished agent processes.
	if agentCmd := p.checkAgentProcesses(); agentCmd != nil {
		cmds = append(cmds, agentCmd)
	}
```

- [ ] **Step 6: Remove local runner message handlers from `HandleMessage`**

In `cc_messages.go`, remove these case blocks (they're replaced by NotifyMsg handlers):

```go
case agent.SessionStartedMsg:
	return p.handleAgentStartedInternal(msg)
```

```go
case agent.SessionFinishedMsg:
	return p.handleAgentFinished(msg)
```

```go
case agent.SessionIDCapturedMsg:
	return p.handleAgentSessionID(msg)
```

```go
case agent.SessionBlockedMsg:
	return p.handleAgentStatus(msg)
```

Keep the handler functions themselves for now if NotifyMsg handlers call them, or if they're unused, remove them too. Add NotifyMsg handlers for `agent.started` and `agent.blocked` if they don't exist yet (mirroring the pattern from `agent.finished`).

- [ ] **Step 7: Update `handleAgentEvent` to not re-listen via local session**

In `cc_messages.go`, `handleAgentEvent` (around line 972) currently re-listens via `agentRunner.Session()`. Since we now use daemon polling, the daemon event handler in `daemonAgentEventMsg` handles re-polling. The `agentEventMsg` handler from the local `listenForAgentEvent` path can stay but should not be the primary path. Review and simplify.

- [ ] **Step 8: Update existing tests**

Tests that send `agent.SessionFinishedMsg` directly should be updated to send `plugin.NotifyMsg{Event: "agent.finished", Data: ...}` instead. Search for `SessionFinishedMsg` in test files and update accordingly.

- [ ] **Step 9: Run full test suite**

Run: `make test`
Expected: All pass

- [ ] **Step 10: Build and manual smoke test**

```bash
make build
ccc daemon stop && ccc daemon start
```

Launch CCC, verify:
- Agent launch works via daemon
- `w` key streams events
- Console shows live costs
- Budget widget updates
- Agent completion transitions todo to review/failed

- [ ] **Step 11: Commit**

```bash
git add internal/builtin/commandcenter/agent_runner.go internal/builtin/commandcenter/cc_messages.go internal/builtin/commandcenter/commandcenter_test.go
git commit -m "Remove local agent fallback — daemon required for all agent operations"
```

---

### Task 6: Update specs

Update all affected specs to reflect the final state.

**Files:**
- Modify: `specs/core/agent.md`
- Modify: `specs/core/daemon.md`
- Modify: `specs/builtin/command-center.md`
- Modify: `specs/builtin/console.md`
- Modify: `specs/builtin/prs.md`

- [ ] **Step 1: Update agent spec** — Add note that TUI consumers receive agent state exclusively via daemon events, not local runner messages.

- [ ] **Step 2: Update daemon spec** — Ensure all new events (`agent.session_id`, `agent.cost_updated`) are fully documented with payloads.

- [ ] **Step 3: Update command-center spec** — Remove references to local runner fallback. Document daemon-only agent lifecycle.

- [ ] **Step 4: Update console spec** — Document event-driven cost refresh via `agent.cost_updated`.

- [ ] **Step 5: Update PR spec** — Document `agent.session_id` handler.

- [ ] **Step 6: Commit**

```bash
git add specs/
git commit -m "Update specs: daemon-required agents, new events, remove local fallback"
```
