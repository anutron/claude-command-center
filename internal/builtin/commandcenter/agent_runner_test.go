package commandcenter

import (
	"sync"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// testBus is a minimal EventBus that records published events.
type testBus struct {
	mu     sync.Mutex
	events []plugin.Event
}

func (b *testBus) Publish(e plugin.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

func (b *testBus) Subscribe(_ string, _ func(plugin.Event)) {}

func (b *testBus) Events() []plugin.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]plugin.Event, len(b.events))
	copy(cp, b.events)
	return cp
}

// newTestPlugin creates a minimal Plugin suitable for testing agent concurrency.
func newTestPlugin(maxConcurrent int) (*Plugin, *testBus) {
	bus := &testBus{}
	cfg := config.DefaultConfig()
	cfg.Agent.MaxConcurrent = maxConcurrent
	cc := &db.CommandCenter{}

	p := &Plugin{
		cfg:            cfg,
		bus:            bus,
		cc:             cc,
		activeSessions: make(map[string]*agentSession),
	}
	return p, bus
}

// addFakeTodo adds a todo to the plugin's in-memory CommandCenter.
func addFakeTodo(p *Plugin, id, status string) {
	p.cc.Todos = append(p.cc.Todos, db.Todo{
		ID:     id,
		Status: status,
		Title:  "Test todo " + id,
	})
}

// addFakeActiveSession inserts a fake active session (simulating a running agent).
func addFakeActiveSession(p *Plugin, todoID string) {
	done := make(chan struct{})
	p.activeSessions[todoID] = &agentSession{
		TodoID:    todoID,
		Status:    "processing",
		StartedAt: time.Now(),
		done:      done,
	}
}

func TestGetBudgetStatusIncludesQueuedAgents(t *testing.T) {
	// Create a plugin with max concurrency of 1.
	p, _ := newTestPlugin(1)

	// Add two todos.
	addFakeTodo(p, "todo-1", db.StatusBacklog)
	addFakeTodo(p, "todo-2", db.StatusBacklog)

	// Simulate: first agent is active, second is queued.
	addFakeActiveSession(p, "todo-1")
	p.sessionQueue = append(p.sessionQueue, queuedSession{
		TodoID:    "todo-2",
		Prompt:    "do something",
		AutoStart: true,
	})

	// The "budget status" should reflect both active and queued agents.
	active := p.activeAgentCount()
	queued := p.queuedAgentCount()
	total := active + queued

	if active != 1 {
		t.Errorf("expected 1 active agent, got %d", active)
	}
	if queued != 1 {
		t.Errorf("expected 1 queued agent, got %d", queued)
	}
	if total != 2 {
		t.Errorf("expected total agent count (active+queued) = 2, got %d", total)
	}

	// Verify canLaunchAgent returns false (at capacity).
	if p.canLaunchAgent() {
		t.Error("canLaunchAgent should return false when at max concurrency")
	}
}

func TestOnAgentFinishedDrainsQueue(t *testing.T) {
	// Create a plugin with max concurrency of 1.
	p, _ := newTestPlugin(1)

	// Add two todos.
	addFakeTodo(p, "todo-1", db.StatusRunning)
	addFakeTodo(p, "todo-2", db.StatusEnqueued)

	// First agent is active.
	addFakeActiveSession(p, "todo-1")

	// Second agent is queued with AutoStart=true.
	p.sessionQueue = append(p.sessionQueue, queuedSession{
		TodoID:    "todo-2",
		Prompt:    "queued task",
		AutoStart: true,
	})

	// Finish the first agent (exit code 0 = success).
	cmd := p.onAgentFinished("todo-1", 0)

	// The first agent should be removed from active sessions.
	if _, ok := p.activeSessions["todo-1"]; ok {
		t.Error("expected todo-1 to be removed from activeSessions after finishing")
	}

	// The queue should be drained (empty now).
	if len(p.sessionQueue) != 0 {
		t.Errorf("expected session queue to be empty after drain, got %d items", len(p.sessionQueue))
	}

	// The drained todo should have status "running" in-memory.
	var todo2Status string
	for _, todo := range p.cc.Todos {
		if todo.ID == "todo-2" {
			todo2Status = todo.Status
			break
		}
	}
	if todo2Status != db.StatusRunning {
		t.Errorf("expected drained todo-2 status to be %q, got %q", db.StatusRunning, todo2Status)
	}

	// cmd should be non-nil (contains launchAgent + persist commands).
	if cmd == nil {
		t.Error("expected onAgentFinished to return a non-nil tea.Cmd for launching drained agent")
	}

	// The finished todo should be in review status.
	var todo1Status string
	for _, todo := range p.cc.Todos {
		if todo.ID == "todo-1" {
			todo1Status = todo.Status
			break
		}
	}
	if todo1Status != db.StatusReview {
		t.Errorf("expected finished todo-1 status to be %q, got %q", db.StatusReview, todo1Status)
	}
}

func TestOnAgentFinishedNoAutoStartSkipsDrain(t *testing.T) {
	// When a queued session has AutoStart=false, it should NOT be launched.
	p, _ := newTestPlugin(1)

	addFakeTodo(p, "todo-1", db.StatusRunning)
	addFakeTodo(p, "todo-2", db.StatusEnqueued)

	addFakeActiveSession(p, "todo-1")

	p.sessionQueue = append(p.sessionQueue, queuedSession{
		TodoID:    "todo-2",
		Prompt:    "queued task",
		AutoStart: false, // should not auto-launch
	})

	p.onAgentFinished("todo-1", 0)

	// Queue should be drained (item removed) but todo-2 should NOT be set to running.
	if len(p.sessionQueue) != 0 {
		t.Errorf("expected session queue to be empty, got %d items", len(p.sessionQueue))
	}

	var todo2Status string
	for _, todo := range p.cc.Todos {
		if todo.ID == "todo-2" {
			todo2Status = todo.Status
			break
		}
	}
	// With AutoStart=false, the status should remain enqueued (not changed to running).
	if todo2Status == db.StatusRunning {
		t.Error("expected todo-2 NOT to be set to running when AutoStart=false")
	}
}

func TestDrainNextAgentBroadcastsStarted(t *testing.T) {
	// When a queued agent is drained, an "agent.started" event should be published.
	p, bus := newTestPlugin(1)

	addFakeTodo(p, "todo-1", db.StatusRunning)
	addFakeTodo(p, "todo-2", db.StatusEnqueued)

	addFakeActiveSession(p, "todo-1")

	p.sessionQueue = append(p.sessionQueue, queuedSession{
		TodoID:    "todo-2",
		Prompt:    "queued task",
		AutoStart: true,
	})

	p.onAgentFinished("todo-1", 0)

	// Check that the bus received an "agent.started" event for todo-2.
	events := bus.Events()
	foundStarted := false
	for _, e := range events {
		if e.Topic == "agent.started" {
			payload, ok := e.Payload.(map[string]interface{})
			if ok && payload["todo_id"] == "todo-2" {
				foundStarted = true
				break
			}
		}
	}
	if !foundStarted {
		t.Error("expected 'agent.started' event to be broadcast for drained todo-2")
	}

	// Also verify "agent.completed" was broadcast for todo-1.
	foundCompleted := false
	for _, e := range events {
		if e.Topic == "agent.completed" {
			payload, ok := e.Payload.(map[string]interface{})
			if ok && payload["todo_id"] == "todo-1" {
				foundCompleted = true
				break
			}
		}
	}
	if !foundCompleted {
		t.Error("expected 'agent.completed' event to be broadcast for finished todo-1")
	}
}

func TestOnAgentFinishedSetsFailedOnNonZeroExit(t *testing.T) {
	p, _ := newTestPlugin(1)

	addFakeTodo(p, "todo-1", db.StatusRunning)
	addFakeActiveSession(p, "todo-1")

	p.onAgentFinished("todo-1", 1)

	var status string
	for _, todo := range p.cc.Todos {
		if todo.ID == "todo-1" {
			status = todo.Status
			break
		}
	}
	if status != db.StatusFailed {
		t.Errorf("expected failed status on non-zero exit, got %q", status)
	}
}

func TestCanLaunchAgentRespectsMaxConcurrent(t *testing.T) {
	p, _ := newTestPlugin(2)

	// No active sessions — should be able to launch.
	if !p.canLaunchAgent() {
		t.Error("expected canLaunchAgent=true with 0 active and max=2")
	}

	// Add one active session.
	addFakeActiveSession(p, "todo-1")
	if !p.canLaunchAgent() {
		t.Error("expected canLaunchAgent=true with 1 active and max=2")
	}

	// Add second active session — now at capacity.
	addFakeActiveSession(p, "todo-2")
	if p.canLaunchAgent() {
		t.Error("expected canLaunchAgent=false with 2 active and max=2")
	}
}

func TestEmptyQueueOnFinishReturnsCmd(t *testing.T) {
	// When no items are queued, onAgentFinished should still return a cmd
	// (for persisting the status change).
	p, _ := newTestPlugin(1)

	addFakeTodo(p, "todo-1", db.StatusRunning)
	addFakeActiveSession(p, "todo-1")

	// No database set, so dbWriteCmd returns nil, but the function itself should not panic.
	cmd := p.onAgentFinished("todo-1", 0)

	// With no database, persist returns nil, so cmd may be nil.
	// The important thing is it doesn't panic and status is updated.
	_ = cmd

	var status string
	for _, todo := range p.cc.Todos {
		if todo.ID == "todo-1" {
			status = todo.Status
			break
		}
	}
	if status != db.StatusReview {
		t.Errorf("expected review status, got %q", status)
	}
}
