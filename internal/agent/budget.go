package agent

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
)

// BudgetTracker tracks cumulative agent spend and enforces hourly/daily budget limits.
type BudgetTracker struct {
	mu          sync.RWMutex
	db          *sql.DB
	cfg         *config.AgentConfig
	hourlySpent float64
	dailySpent  float64
	stopped     bool // emergency stop state
}

// BudgetStatus is a point-in-time snapshot of budget usage and limits.
type BudgetStatus struct {
	HourlySpent      float64
	HourlyLimit      float64
	DailySpent       float64
	DailyLimit       float64
	EmergencyStopped bool
	WarningLevel     string // "none", "warning", "critical"
}

// NewBudgetTracker creates a BudgetTracker, loading persisted emergency stop state from DB.
func NewBudgetTracker(database *sql.DB, cfg *config.AgentConfig) *BudgetTracker {
	bt := &BudgetTracker{
		db:  database,
		cfg: cfg,
	}

	// Load emergency stop state from DB.
	num, _, err := db.DBGetBudgetState(database, "emergency_stop")
	if err == nil && num != 0 {
		bt.stopped = true
	}

	// Load current totals.
	bt.refreshTotalsLocked()

	return bt
}

// CanLaunch checks whether a new agent with the given budget can be launched.
// Returns (true, "") if allowed, or (false, reason) if denied.
// Checks in order: emergency stop, hourly headroom, daily headroom.
func (bt *BudgetTracker) CanLaunch(budget float64) (bool, string) {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	if bt.stopped {
		return false, "emergency stop is active"
	}

	if bt.cfg.HourlyBudget > 0 && bt.hourlySpent+budget > bt.cfg.HourlyBudget {
		return false, fmt.Sprintf("would exceed hourly budget: $%.2f spent + $%.2f request > $%.2f limit",
			bt.hourlySpent, budget, bt.cfg.HourlyBudget)
	}

	if bt.cfg.DailyBudget > 0 && bt.dailySpent+budget > bt.cfg.DailyBudget {
		return false, fmt.Sprintf("would exceed daily budget: $%.2f spent + $%.2f request > $%.2f limit",
			bt.dailySpent, budget, bt.cfg.DailyBudget)
	}

	return true, ""
}

// RecordLaunch inserts a new agent cost row and returns the row ID.
func (bt *BudgetTracker) RecordLaunch(agentID, automation, projectDir string, budget float64) int64 {
	id, err := db.DBInsertAgentCost(bt.db, agentID, automation, projectDir, budget, time.Now())
	if err != nil {
		return 0
	}
	return id
}

// RecordCost updates the running cost for an in-progress agent run.
func (bt *BudgetTracker) RecordCost(costRowID int64, inputTokens, outputTokens int, costUSD float64) {
	_, _ = bt.db.Exec(
		`UPDATE cc_agent_costs SET cost_usd = ?, input_tokens = ?, output_tokens = ? WHERE id = ?`,
		costUSD, inputTokens, outputTokens, costRowID,
	)

	// Refresh cached totals so CanLaunch sees updated spend.
	bt.mu.Lock()
	bt.refreshTotalsLocked()
	bt.mu.Unlock()
}

// RecordFinished marks an agent cost row as finished and refreshes cached totals.
func (bt *BudgetTracker) RecordFinished(costRowID int64, durationSec, exitCode int, finalCostUSD float64) {
	status := "completed"
	if exitCode != 0 {
		status = "failed"
	}
	_ = db.DBUpdateAgentCostFinished(bt.db, costRowID, durationSec, finalCostUSD, exitCode, status)

	bt.mu.Lock()
	bt.refreshTotalsLocked()
	bt.mu.Unlock()
}

// EmergencyStop activates the emergency stop, blocking all future launches.
func (bt *BudgetTracker) EmergencyStop() {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.stopped = true
	_ = db.DBSetBudgetState(bt.db, "emergency_stop", 1, "")
}

// Resume deactivates the emergency stop.
func (bt *BudgetTracker) Resume() {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.stopped = false
	_ = db.DBSetBudgetState(bt.db, "emergency_stop", 0, "")
}

// Status returns a point-in-time snapshot of budget usage.
func (bt *BudgetTracker) Status() BudgetStatus {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	wl := "none"
	if bt.cfg.HourlyBudget > 0 {
		ratio := bt.hourlySpent / bt.cfg.HourlyBudget
		if ratio >= 0.95 {
			wl = "critical"
		} else if bt.cfg.BudgetWarningPct > 0 && ratio >= bt.cfg.BudgetWarningPct {
			wl = "warning"
		}
	}

	return BudgetStatus{
		HourlySpent:      bt.hourlySpent,
		HourlyLimit:      bt.cfg.HourlyBudget,
		DailySpent:       bt.dailySpent,
		DailyLimit:       bt.cfg.DailyBudget,
		EmergencyStopped: bt.stopped,
		WarningLevel:     wl,
	}
}

// refreshTotalsLocked queries the DB for current hourly and daily totals.
// Caller must hold bt.mu (write lock).
func (bt *BudgetTracker) refreshTotalsLocked() {
	now := time.Now()

	hourly, err := db.DBSumCostsSince(bt.db, now.Add(-1*time.Hour))
	if err == nil {
		bt.hourlySpent = hourly
	}

	daily, err := db.DBSumCostsSince(bt.db, now.Add(-24*time.Hour))
	if err == nil {
		bt.dailySpent = daily
	}
}
