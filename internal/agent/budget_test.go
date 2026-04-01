package agent

import (
	"path/filepath"
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
)

func newTestBudgetTracker(t *testing.T, cfg *config.AgentConfig) *BudgetTracker {
	t.Helper()
	dir := t.TempDir()
	database, err := db.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewBudgetTracker(database, cfg)
}

func defaultAgentCfg() *config.AgentConfig {
	return &config.AgentConfig{
		HourlyBudget:     25.00,
		DailyBudget:      100.00,
		BudgetWarningPct: 0.80,
	}
}

func TestCanLaunch_UnderBudget(t *testing.T) {
	bt := newTestBudgetTracker(t, defaultAgentCfg())

	ok, reason := bt.CanLaunch(5.0)
	if !ok {
		t.Fatalf("expected CanLaunch to allow, got denied: %s", reason)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestCanLaunch_ExceedsHourlyBudget(t *testing.T) {
	cfg := defaultAgentCfg()
	cfg.HourlyBudget = 10.0
	bt := newTestBudgetTracker(t, cfg)

	// Record a launch that costs $8
	rowID := bt.RecordLaunch("agent-1", "test", "", 10.0)
	bt.RecordFinished(rowID, 60, 0, 8.0)

	ok, reason := bt.CanLaunch(5.0)
	if ok {
		t.Fatal("expected CanLaunch to deny when over hourly budget")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestCanLaunch_ExceedsDailyBudget(t *testing.T) {
	cfg := defaultAgentCfg()
	cfg.HourlyBudget = 100.0 // high hourly so it doesn't trip
	cfg.DailyBudget = 10.0
	bt := newTestBudgetTracker(t, cfg)

	// Record a launch that costs $8
	rowID := bt.RecordLaunch("agent-1", "test", "", 10.0)
	bt.RecordFinished(rowID, 60, 0, 8.0)

	ok, reason := bt.CanLaunch(5.0)
	if ok {
		t.Fatal("expected CanLaunch to deny when over daily budget")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestCanLaunch_EmergencyStopped(t *testing.T) {
	bt := newTestBudgetTracker(t, defaultAgentCfg())

	bt.EmergencyStop()

	ok, reason := bt.CanLaunch(1.0)
	if ok {
		t.Fatal("expected CanLaunch to deny when emergency stopped")
	}
	if reason != "emergency stop is active" {
		t.Errorf("unexpected reason: %q", reason)
	}
}

func TestEmergencyStop_Resume_PersistsToDb(t *testing.T) {
	dir := t.TempDir()
	database, err := db.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	cfg := defaultAgentCfg()

	// Create tracker and emergency stop
	bt := NewBudgetTracker(database, cfg)
	bt.EmergencyStop()

	// Create a new tracker from the same DB — should load stopped state
	bt2 := NewBudgetTracker(database, cfg)
	if !bt2.stopped {
		t.Error("expected new tracker to load emergency stop state from DB")
	}

	// Resume and verify persistence
	bt2.Resume()
	bt3 := NewBudgetTracker(database, cfg)
	if bt3.stopped {
		t.Error("expected new tracker to load resumed state from DB")
	}
}

func TestRecordFinished_RefreshesTotals(t *testing.T) {
	bt := newTestBudgetTracker(t, defaultAgentCfg())

	rowID := bt.RecordLaunch("agent-1", "test", "", 5.0)
	bt.RecordFinished(rowID, 60, 0, 3.50)

	bt.mu.RLock()
	hourly := bt.hourlySpent
	daily := bt.dailySpent
	bt.mu.RUnlock()

	if hourly != 3.50 {
		t.Errorf("hourlySpent = %f, want 3.50", hourly)
	}
	if daily != 3.50 {
		t.Errorf("dailySpent = %f, want 3.50", daily)
	}
}

func TestStatus_WarningLevels(t *testing.T) {
	tests := []struct {
		name     string
		spent    float64
		limit    float64
		warnPct  float64
		expected string
	}{
		{"none - under warning", 5.0, 25.0, 0.80, "none"},
		{"warning - at 80%", 20.0, 25.0, 0.80, "warning"},
		{"warning - at 85%", 21.25, 25.0, 0.80, "warning"},
		{"critical - at 95%", 23.75, 25.0, 0.80, "critical"},
		{"critical - at 100%", 25.0, 25.0, 0.80, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.AgentConfig{
				HourlyBudget:     tt.limit,
				DailyBudget:      100.0,
				BudgetWarningPct: tt.warnPct,
			}
			bt := newTestBudgetTracker(t, cfg)

			// Record a finished launch with the desired cost
			rowID := bt.RecordLaunch("agent-1", "test", "", tt.limit)
			bt.RecordFinished(rowID, 60, 0, tt.spent)

			status := bt.Status()
			if status.WarningLevel != tt.expected {
				t.Errorf("WarningLevel = %q, want %q (spent=%.2f, limit=%.2f)",
					status.WarningLevel, tt.expected, tt.spent, tt.limit)
			}
		})
	}
}

func TestStatus_Fields(t *testing.T) {
	cfg := defaultAgentCfg()
	bt := newTestBudgetTracker(t, cfg)

	status := bt.Status()
	if status.HourlyLimit != 25.0 {
		t.Errorf("HourlyLimit = %f, want 25.0", status.HourlyLimit)
	}
	if status.DailyLimit != 100.0 {
		t.Errorf("DailyLimit = %f, want 100.0", status.DailyLimit)
	}
	if status.EmergencyStopped {
		t.Error("expected EmergencyStopped = false")
	}
	if status.WarningLevel != "none" {
		t.Errorf("WarningLevel = %q, want %q", status.WarningLevel, "none")
	}

	bt.EmergencyStop()
	status = bt.Status()
	if !status.EmergencyStopped {
		t.Error("expected EmergencyStopped = true after EmergencyStop()")
	}
}

func TestRecordCost_UpdatesRunningTotal(t *testing.T) {
	bt := newTestBudgetTracker(t, defaultAgentCfg())

	rowID := bt.RecordLaunch("agent-1", "test", "", 10.0)

	// Incremental cost update
	bt.RecordCost(rowID, 1000, 500, 2.50)

	bt.mu.RLock()
	hourly := bt.hourlySpent
	bt.mu.RUnlock()

	if hourly != 2.50 {
		t.Errorf("hourlySpent after RecordCost = %f, want 2.50", hourly)
	}
}
