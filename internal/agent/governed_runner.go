package agent

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/anutron/claude-command-center/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// LaunchDeniedMsg is sent when a launch is denied by budget or rate limit checks.
type LaunchDeniedMsg struct {
	ID     string
	Reason string
}

// GovernedRunner wraps a Runner, enforcing budget and rate limits before
// delegating to the inner runner.
type GovernedRunner struct {
	inner   Runner
	budget  *BudgetTracker
	limiter *RateLimiter

	// costRows tracks cost row IDs for active sessions so we can
	// record finished status when CleanupFinished is called.
	mu       sync.Mutex
	costRows map[string]costEntry
}

type costEntry struct {
	rowID     int64
	startedAt time.Time
}

// NewGovernedRunner creates a GovernedRunner that wraps the given inner runner,
// enforcing budget and rate limits from the provided database and config.
func NewGovernedRunner(inner Runner, db *sql.DB, cfg *config.AgentConfig) *GovernedRunner {
	return &GovernedRunner{
		inner:    inner,
		budget:   NewBudgetTracker(db, cfg),
		limiter:  NewRateLimiter(db, cfg),
		costRows: make(map[string]costEntry),
	}
}

// BudgetTracker returns the budget tracker for external use (e.g., daemon RPC handlers).
func (g *GovernedRunner) BudgetTracker() *BudgetTracker {
	return g.budget
}

// LaunchOrQueue checks budget and rate limits before delegating to the inner runner.
// If denied, returns queued=true and a tea.Cmd that emits LaunchDeniedMsg.
func (g *GovernedRunner) LaunchOrQueue(req Request) (queued bool, cmd tea.Cmd) {
	// 1. Budget check.
	if ok, reason := g.budget.CanLaunch(req.Budget); !ok {
		return true, func() tea.Msg {
			return LaunchDeniedMsg{ID: req.ID, Reason: fmt.Sprintf("budget denied: %s", reason)}
		}
	}

	// 2. Rate limit check.
	if ok, reason := g.limiter.CanLaunch(req.ID, req.Automation); !ok {
		return true, func() tea.Msg {
			return LaunchDeniedMsg{ID: req.ID, Reason: fmt.Sprintf("rate limited: %s", reason)}
		}
	}

	// 3. Record the launch cost before delegating.
	costRowID := g.budget.RecordLaunch(req.ID, req.Automation, req.Budget)
	g.mu.Lock()
	g.costRows[req.ID] = costEntry{rowID: costRowID, startedAt: time.Now()}
	g.mu.Unlock()

	// 4. Delegate to the inner runner.
	return g.inner.LaunchOrQueue(req)
}

// Kill delegates to the inner runner.
func (g *GovernedRunner) Kill(id string) bool {
	return g.inner.Kill(id)
}

// SendMessage delegates to the inner runner.
func (g *GovernedRunner) SendMessage(id string, message string) error {
	return g.inner.SendMessage(id, message)
}

// Status delegates to the inner runner.
func (g *GovernedRunner) Status(id string) *SessionStatus {
	return g.inner.Status(id)
}

// Active delegates to the inner runner.
func (g *GovernedRunner) Active() []SessionInfo {
	return g.inner.Active()
}

// QueueLen delegates to the inner runner.
func (g *GovernedRunner) QueueLen() int {
	return g.inner.QueueLen()
}

// Session delegates to the inner runner.
func (g *GovernedRunner) Session(id string) *Session {
	return g.inner.Session(id)
}

// CheckProcesses delegates to the inner runner.
func (g *GovernedRunner) CheckProcesses() tea.Cmd {
	return g.inner.CheckProcesses()
}

// DrainQueue delegates to the inner runner.
func (g *GovernedRunner) DrainQueue() (Request, bool) {
	return g.inner.DrainQueue()
}

// CleanupFinished removes a finished session and records its cost data.
func (g *GovernedRunner) CleanupFinished(id string) *Session {
	sess := g.inner.CleanupFinished(id)

	g.mu.Lock()
	entry, ok := g.costRows[id]
	if ok {
		delete(g.costRows, id)
	}
	g.mu.Unlock()

	if ok && sess != nil {
		duration := int(time.Since(entry.startedAt).Seconds())
		exitCode := sess.ExitCode()
		// We don't have actual cost data from the session, so record with
		// the budgeted amount. Real cost tracking would come from parsing
		// the session output.
		g.budget.RecordFinished(entry.rowID, duration, exitCode, 0)
	}

	return sess
}

// Watch delegates to the inner runner.
func (g *GovernedRunner) Watch(id string) tea.Cmd {
	return g.inner.Watch(id)
}

// Shutdown triggers emergency stop on the budget tracker and shuts down the inner runner.
func (g *GovernedRunner) Shutdown() {
	g.budget.EmergencyStop()
	g.inner.Shutdown()
}
