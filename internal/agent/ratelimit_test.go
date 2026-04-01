package agent

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func testRateLimiterDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	database, err := db.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestCanLaunch_AllowsWhenNoPriorLaunches(t *testing.T) {
	database := testRateLimiterDB(t)
	rl := NewRateLimiter(database, defaultAgentCfg())

	ok, reason := rl.CanLaunch("agent-1", "pr-review")
	if !ok {
		t.Fatalf("expected allow, got deny: %s", reason)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestCanLaunch_RefusesWhenHourlyCapExceeded(t *testing.T) {
	database := testRateLimiterDB(t)
	cfg := defaultAgentCfg()
	cfg.MaxLaunchesPerAutomationPerHour = 3
	rl := NewRateLimiter(database, cfg)

	now := time.Now()
	// Insert 3 launches within the last hour (at the cap)
	for i := 0; i < 3; i++ {
		db.DBInsertAgentCost(database, "other-agent", "pr-review", "", 5, now.Add(-time.Duration(10*(i+1))*time.Minute))
	}

	ok, reason := rl.CanLaunch("new-agent", "pr-review")
	if ok {
		t.Fatal("expected deny due to hourly cap, got allow")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
	t.Logf("denial reason: %s", reason)
}

func TestCanLaunch_RefusesWhenAgentInCooldown(t *testing.T) {
	database := testRateLimiterDB(t)
	cfg := defaultAgentCfg()
	cfg.CooldownMinutes = 15
	rl := NewRateLimiter(database, cfg)

	// Agent launched 5 minutes ago — still in 15-minute cooldown
	db.DBInsertAgentCost(database, "agent-1", "pr-review", "", 5, time.Now().Add(-5*time.Minute))

	ok, reason := rl.CanLaunch("agent-1", "pr-review")
	if ok {
		t.Fatal("expected deny due to cooldown, got allow")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
	t.Logf("denial reason: %s", reason)
}

func TestCanLaunch_RefusesWhenFailureBackoffActive(t *testing.T) {
	database := testRateLimiterDB(t)
	cfg := defaultAgentCfg()
	cfg.CooldownMinutes = 1 // short cooldown so cooldown check passes
	cfg.FailureBackoffBaseSec = 60
	cfg.FailureBackoffMaxSec = 3600
	rl := NewRateLimiter(database, cfg)

	now := time.Now()
	// Insert a failed run 30 seconds ago — with base=60s, backoff should be 60s
	id, _ := db.DBInsertAgentCost(database, "other-agent", "pr-review", "", 5, now.Add(-30*time.Second))
	db.DBUpdateAgentCostFinished(database, id, 10, 0.5, 1, "failed")

	// Use a different agent ID so cooldown check passes
	ok, reason := rl.CanLaunch("new-agent", "pr-review")
	if ok {
		t.Fatal("expected deny due to failure backoff, got allow")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
	t.Logf("denial reason: %s", reason)
}

func TestCanLaunch_AllowsWhenCooldownExpired(t *testing.T) {
	database := testRateLimiterDB(t)
	cfg := defaultAgentCfg()
	cfg.CooldownMinutes = 15
	rl := NewRateLimiter(database, cfg)

	// Agent launched 20 minutes ago — cooldown of 15 minutes has passed
	db.DBInsertAgentCost(database, "agent-1", "pr-review", "", 5, time.Now().Add(-20*time.Minute))

	ok, reason := rl.CanLaunch("agent-1", "pr-review")
	if !ok {
		t.Fatalf("expected allow after cooldown expired, got deny: %s", reason)
	}
}

func TestCanLaunch_EmptyAutomationSkipsAutomationChecks(t *testing.T) {
	database := testRateLimiterDB(t)
	cfg := defaultAgentCfg()
	cfg.MaxLaunchesPerAutomationPerHour = 1
	rl := NewRateLimiter(database, cfg)

	now := time.Now()
	// Insert a launch — but with empty automation, the per-automation cap should be skipped
	db.DBInsertAgentCost(database, "other-agent", "", "", 5, now.Add(-5*time.Minute))

	// Also insert a failure to make sure failure backoff is skipped too
	id, _ := db.DBInsertAgentCost(database, "fail-agent", "", "", 5, now.Add(-10*time.Second))
	db.DBUpdateAgentCostFinished(database, id, 5, 0.1, 1, "failed")

	// Use a fresh agent ID so cooldown passes
	ok, reason := rl.CanLaunch("brand-new-agent", "")
	if !ok {
		t.Fatalf("expected allow with empty automation, got deny: %s", reason)
	}
}

func TestCanLaunch_FailureBackoffExponential(t *testing.T) {
	database := testRateLimiterDB(t)
	cfg := defaultAgentCfg()
	cfg.CooldownMinutes = 1
	cfg.FailureBackoffBaseSec = 10
	cfg.FailureBackoffMaxSec = 3600
	rl := NewRateLimiter(database, cfg)

	now := time.Now()
	// Insert 3 failures in the last hour — backoff = min(10 * 2^2, 3600) = 40 seconds
	for i := 0; i < 3; i++ {
		id, _ := db.DBInsertAgentCost(database, "fail-agent", "auto", "", 5, now.Add(-time.Duration(5*(i+1))*time.Second))
		db.DBUpdateAgentCostFinished(database, id, 5, 0.1, 1, "failed")
	}

	// Last failure was 5 seconds ago, backoff is 40 seconds — should deny
	ok, reason := rl.CanLaunch("new-agent", "auto")
	if ok {
		t.Fatal("expected deny due to exponential backoff, got allow")
	}
	t.Logf("denial reason: %s", reason)
}
