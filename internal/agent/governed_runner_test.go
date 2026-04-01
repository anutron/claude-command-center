package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeRunner is a minimal Runner implementation for testing GovernedRunner.
type fakeRunner struct {
	mu             sync.Mutex
	launched       []Request
	sessions       map[string]*Session
	shutdownCalled bool
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		sessions: make(map[string]*Session),
	}
}

func (f *fakeRunner) LaunchOrQueue(req Request) (bool, tea.Cmd) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.launched = append(f.launched, req)
	sess := &Session{
		ID:        req.ID,
		Status:    "processing",
		StartedAt: time.Now(),
		done:      make(chan struct{}),
		output:    &strings.Builder{},
	}
	f.sessions[req.ID] = sess
	return false, func() tea.Msg {
		return SessionStartedMsg{ID: req.ID, Session: sess}
	}
}

func (f *fakeRunner) Kill(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.sessions[id]
	if ok {
		delete(f.sessions, id)
	}
	return ok
}

func (f *fakeRunner) SendMessage(id string, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	return nil
}

func (f *fakeRunner) Status(id string) *SessionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.sessions[id]
	if !ok {
		return nil
	}
	return &SessionStatus{ID: id, Status: sess.Status, StartedAt: sess.StartedAt}
}

func (f *fakeRunner) Active() []SessionInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []SessionInfo
	for _, s := range f.sessions {
		result = append(result, SessionInfo{ID: s.ID, Status: s.Status, StartedAt: s.StartedAt})
	}
	return result
}

func (f *fakeRunner) QueueLen() int { return 0 }

func (f *fakeRunner) Session(id string) *Session {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[id]
}

func (f *fakeRunner) CheckProcesses() tea.Cmd { return nil }

func (f *fakeRunner) DrainQueue() (Request, bool) { return Request{}, false }

func (f *fakeRunner) CleanupFinished(id string) *Session {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.sessions[id]
	if ok {
		delete(f.sessions, id)
	}
	return sess
}

func (f *fakeRunner) Watch(id string) tea.Cmd { return nil }

func (f *fakeRunner) Shutdown() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdownCalled = true
}

func newTestGovernedRunner(t *testing.T, cfg *config.AgentConfig) (*GovernedRunner, *fakeRunner) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	inner := newFakeRunner()
	gr := NewGovernedRunner(inner, database, cfg)
	return gr, inner
}

func TestGovernedRunner_LaunchSuccess(t *testing.T) {
	cfg := defaultAgentCfg()
	gr, inner := newTestGovernedRunner(t, cfg)

	req := Request{
		ID:         "test-1",
		Budget:     5.0,
		Automation: "test",
	}

	queued, cmd := gr.LaunchOrQueue(req)
	if queued {
		t.Error("expected launch not to be queued")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	// Verify inner runner received the request.
	inner.mu.Lock()
	if len(inner.launched) != 1 {
		t.Errorf("expected 1 launch, got %d", len(inner.launched))
	}
	inner.mu.Unlock()

	// Verify cost row was tracked.
	gr.mu.Lock()
	if _, ok := gr.costRows["test-1"]; !ok {
		t.Error("expected cost row entry for test-1")
	}
	gr.mu.Unlock()
}

func TestGovernedRunner_LaunchDenied_OverBudget(t *testing.T) {
	cfg := defaultAgentCfg()
	cfg.HourlyBudget = 10.0
	gr, inner := newTestGovernedRunner(t, cfg)

	// Fill up the budget.
	rowID := gr.budget.RecordLaunch("pre-fill", "test", "", 10.0)
	gr.budget.RecordCost(rowID, 0, 0, 9.0)
	gr.budget.RecordFinished(rowID, 60, 0)

	req := Request{
		ID:         "test-over",
		Budget:     5.0,
		Automation: "test",
	}

	queued, cmd := gr.LaunchOrQueue(req)
	if !queued {
		t.Error("expected launch to be queued (denied)")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd with denial message")
	}

	// Execute the cmd and check for LaunchDeniedMsg.
	msg := cmd()
	denied, ok := msg.(LaunchDeniedMsg)
	if !ok {
		t.Fatalf("expected LaunchDeniedMsg, got %T", msg)
	}
	if denied.ID != "test-over" {
		t.Errorf("expected ID test-over, got %s", denied.ID)
	}
	if !strings.Contains(denied.Reason, "budget denied") {
		t.Errorf("expected budget denial reason, got %q", denied.Reason)
	}

	// Inner runner should NOT have been called.
	inner.mu.Lock()
	if len(inner.launched) != 0 {
		t.Errorf("expected 0 launches on inner runner, got %d", len(inner.launched))
	}
	inner.mu.Unlock()
}

func TestGovernedRunner_LaunchDenied_RateLimited(t *testing.T) {
	cfg := defaultAgentCfg()
	cfg.CooldownMinutes = 60 // 60 min cooldown
	gr, inner := newTestGovernedRunner(t, cfg)

	// Record a recent launch for this agent ID to trigger cooldown.
	gr.budget.RecordLaunch("test-rl", "test", "", 1.0)

	req := Request{
		ID:         "test-rl",
		Budget:     1.0,
		Automation: "test",
	}

	queued, cmd := gr.LaunchOrQueue(req)
	if !queued {
		t.Error("expected launch to be queued (rate limited)")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd with denial message")
	}

	msg := cmd()
	denied, ok := msg.(LaunchDeniedMsg)
	if !ok {
		t.Fatalf("expected LaunchDeniedMsg, got %T", msg)
	}
	if !strings.Contains(denied.Reason, "rate limited") {
		t.Errorf("expected rate limit reason, got %q", denied.Reason)
	}

	// Inner runner should NOT have been called.
	inner.mu.Lock()
	if len(inner.launched) != 0 {
		t.Errorf("expected 0 launches on inner runner, got %d", len(inner.launched))
	}
	inner.mu.Unlock()
}

func TestGovernedRunner_LaunchDenied_EmergencyStop(t *testing.T) {
	cfg := defaultAgentCfg()
	gr, _ := newTestGovernedRunner(t, cfg)

	gr.budget.EmergencyStop()

	req := Request{ID: "test-es", Budget: 1.0}
	queued, cmd := gr.LaunchOrQueue(req)
	if !queued {
		t.Error("expected launch to be queued (emergency stop)")
	}

	msg := cmd()
	denied, ok := msg.(LaunchDeniedMsg)
	if !ok {
		t.Fatalf("expected LaunchDeniedMsg, got %T", msg)
	}
	if !strings.Contains(denied.Reason, "emergency stop") {
		t.Errorf("expected emergency stop reason, got %q", denied.Reason)
	}
}

func TestGovernedRunner_ShutdownDoesNotPersistEmergencyStop(t *testing.T) {
	cfg := defaultAgentCfg()
	gr, inner := newTestGovernedRunner(t, cfg)

	gr.Shutdown()

	// Budget tracker should NOT be in emergency stop after a normal shutdown.
	// Emergency stop is reserved for explicit user action (ctrl+x / stop-all RPC).
	status := gr.budget.Status()
	if status.EmergencyStopped {
		t.Error("expected no emergency stop after normal Shutdown()")
	}

	// Inner runner should have been shut down.
	inner.mu.Lock()
	if !inner.shutdownCalled {
		t.Error("expected inner Shutdown() to be called")
	}
	inner.mu.Unlock()
}

func TestGovernedRunner_CleanupFinished_RecordsCost(t *testing.T) {
	cfg := defaultAgentCfg()
	gr, _ := newTestGovernedRunner(t, cfg)

	req := Request{ID: "test-cleanup", Budget: 5.0, Automation: "test"}
	gr.LaunchOrQueue(req)

	// Verify cost row exists.
	gr.mu.Lock()
	entry, ok := gr.costRows["test-cleanup"]
	gr.mu.Unlock()
	if !ok {
		t.Fatal("expected cost row entry")
	}
	if entry.rowID == 0 {
		t.Error("expected non-zero cost row ID")
	}

	// Cleanup the finished session.
	sess := gr.CleanupFinished("test-cleanup")
	if sess == nil {
		t.Fatal("expected non-nil session from CleanupFinished")
	}

	// Cost row should be removed from tracking.
	gr.mu.Lock()
	_, stillTracked := gr.costRows["test-cleanup"]
	gr.mu.Unlock()
	if stillTracked {
		t.Error("expected cost row to be removed after CleanupFinished")
	}
}

func TestGovernedRunner_DelegatesMethods(t *testing.T) {
	cfg := defaultAgentCfg()
	gr, inner := newTestGovernedRunner(t, cfg)

	// Launch a session first.
	req := Request{ID: "test-delegate", Budget: 1.0}
	gr.LaunchOrQueue(req)

	// Test Status delegation.
	status := gr.Status("test-delegate")
	if status == nil {
		t.Error("expected non-nil status")
	}

	// Test Active delegation.
	active := gr.Active()
	if len(active) != 1 {
		t.Errorf("expected 1 active session, got %d", len(active))
	}

	// Test Session delegation.
	sess := gr.Session("test-delegate")
	if sess == nil {
		t.Error("expected non-nil session")
	}

	// Test Kill delegation.
	killed := gr.Kill("test-delegate")
	if !killed {
		t.Error("expected Kill to return true")
	}

	// Test nonexistent.
	if gr.Status("nonexistent") != nil {
		t.Error("expected nil status for nonexistent")
	}

	// Test QueueLen delegation.
	if gr.QueueLen() != 0 {
		t.Error("expected queue len 0")
	}

	// Test DrainQueue delegation.
	_, ok := gr.DrainQueue()
	if ok {
		t.Error("expected DrainQueue to return false")
	}

	// Test CheckProcesses delegation.
	cmd := gr.CheckProcesses()
	if cmd != nil {
		t.Error("expected nil cmd from CheckProcesses")
	}

	// Test SendMessage delegation (nonexistent).
	err := gr.SendMessage("nonexistent", "hello")
	if err == nil {
		t.Error("expected error from SendMessage to nonexistent session")
	}

	// Test Watch delegation.
	watchCmd := gr.Watch("nonexistent")
	if watchCmd != nil {
		t.Error("expected nil cmd from Watch for nonexistent session")
	}

	// Verify inner runner is the same reference.
	_ = inner
}

func TestGovernedRunner_CostCallback_InvokesBroadcast(t *testing.T) {
	cfg := defaultAgentCfg()
	gr, inner := newTestGovernedRunner(t, cfg)

	var broadcasts []float64
	gr.SetCostBroadcast(func(id string, inputTokens, outputTokens int, costUSD float64) {
		broadcasts = append(broadcasts, costUSD)
	})

	req := Request{ID: "test-cb-1", Prompt: "hello", Budget: 1.0}
	gr.LaunchOrQueue(req)

	// Grab the launched request from the inner runner to get the wired CostCallback.
	inner.mu.Lock()
	if len(inner.launched) == 0 {
		t.Fatal("expected at least 1 launch")
	}
	lastReq := inner.launched[len(inner.launched)-1]
	inner.mu.Unlock()

	if lastReq.CostCallback == nil {
		t.Fatal("expected CostCallback to be set")
	}
	lastReq.CostCallback(100, 50, 0.01)
	if len(broadcasts) != 1 || broadcasts[0] != 0.01 {
		t.Errorf("expected 1 broadcast with cost 0.01, got %v", broadcasts)
	}
}

func TestGovernedRunner_CostBroadcast_ThrottledTo2Seconds(t *testing.T) {
	cfg := defaultAgentCfg()
	gr, inner := newTestGovernedRunner(t, cfg)

	var broadcastCount int
	gr.SetCostBroadcast(func(id string, inputTokens, outputTokens int, costUSD float64) {
		broadcastCount++
	})

	req := Request{ID: "test-cb-2", Prompt: "hello", Budget: 1.0}
	gr.LaunchOrQueue(req)

	inner.mu.Lock()
	lastReq := inner.launched[len(inner.launched)-1]
	inner.mu.Unlock()

	cb := lastReq.CostCallback
	for i := 0; i < 10; i++ {
		cb(100*i, 50*i, 0.01*float64(i))
	}
	if broadcastCount != 1 {
		t.Errorf("expected 1 broadcast (throttled), got %d", broadcastCount)
	}
}

func TestGovernedRunner_BudgetTrackerAccessor(t *testing.T) {
	cfg := defaultAgentCfg()
	gr, _ := newTestGovernedRunner(t, cfg)

	bt := gr.BudgetTracker()
	if bt == nil {
		t.Fatal("expected non-nil BudgetTracker")
	}

	status := bt.Status()
	if status.HourlyLimit != 25.0 {
		t.Errorf("expected hourly limit 25.0, got %f", status.HourlyLimit)
	}
}
