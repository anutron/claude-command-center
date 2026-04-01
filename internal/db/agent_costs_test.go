package db

import (
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Agent cost helpers
// ---------------------------------------------------------------------------

func TestDBInsertAgentCost(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	id, err := DBInsertAgentCost(db, "agent-1", "pr-review", "", 5.0, now)
	if err != nil {
		t.Fatalf("DBInsertAgentCost: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive row ID, got %d", id)
	}

	// Verify the row exists
	var agentID, status string
	var budget float64
	err = db.QueryRow(`SELECT agent_id, status, budget_usd FROM cc_agent_costs WHERE id = ?`, id).
		Scan(&agentID, &status, &budget)
	if err != nil {
		t.Fatalf("query inserted row: %v", err)
	}
	if agentID != "agent-1" {
		t.Errorf("agent_id = %q, want %q", agentID, "agent-1")
	}
	if status != "running" {
		t.Errorf("status = %q, want %q", status, "running")
	}
	if budget != 5.0 {
		t.Errorf("budget = %f, want %f", budget, 5.0)
	}
}

func TestDBUpdateAgentCostFinished(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	id, err := DBInsertAgentCost(db, "agent-2", "triage", "", 3.0, time.Now())
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	err = DBUpdateAgentCostFinished(db, id, 120, 1.50, 0, "completed")
	if err != nil {
		t.Fatalf("DBUpdateAgentCostFinished: %v", err)
	}

	var status string
	var dur int
	var cost float64
	var exitCode int
	err = db.QueryRow(`SELECT status, duration_sec, cost_usd, exit_code FROM cc_agent_costs WHERE id = ?`, id).
		Scan(&status, &dur, &cost, &exitCode)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "completed" {
		t.Errorf("status = %q, want %q", status, "completed")
	}
	if dur != 120 {
		t.Errorf("duration_sec = %d, want 120", dur)
	}
	if cost != 1.50 {
		t.Errorf("cost_usd = %f, want 1.5", cost)
	}
	if exitCode != 0 {
		t.Errorf("exit_code = %d, want 0", exitCode)
	}
}

func TestDBSumCostsSince(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	past := now.Add(-2 * time.Hour)
	ancient := now.Add(-48 * time.Hour)

	// Insert two recent, one old
	id1, _ := DBInsertAgentCost(db, "a", "auto", "", 5, now.Add(-1*time.Hour))
	id2, _ := DBInsertAgentCost(db, "b", "auto", "", 5, now.Add(-30*time.Minute))
	id3, _ := DBInsertAgentCost(db, "c", "auto", "", 5, ancient)

	DBUpdateAgentCostFinished(db, id1, 60, 2.0, 0, "completed")
	DBUpdateAgentCostFinished(db, id2, 30, 1.0, 0, "completed")
	DBUpdateAgentCostFinished(db, id3, 60, 10.0, 0, "completed")

	total, err := DBSumCostsSince(db, past)
	if err != nil {
		t.Fatalf("DBSumCostsSince: %v", err)
	}
	if total != 3.0 {
		t.Errorf("total = %f, want 3.0", total)
	}

	// No results case
	total, err = DBSumCostsSince(db, now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("DBSumCostsSince (empty): %v", err)
	}
	if total != 0 {
		t.Errorf("total = %f, want 0", total)
	}
}

func TestDBCountLaunchesSince(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	DBInsertAgentCost(db, "a", "pr-review", "", 5, now.Add(-1*time.Hour))
	DBInsertAgentCost(db, "b", "pr-review", "", 5, now.Add(-30*time.Minute))
	DBInsertAgentCost(db, "c", "triage", "", 5, now.Add(-30*time.Minute))

	count, err := DBCountLaunchesSince(db, "pr-review", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("DBCountLaunchesSince: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	count, err = DBCountLaunchesSince(db, "triage", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("DBCountLaunchesSince: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestDBLastAgentLaunch(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// No launches yet — should return zero time
	last, err := DBLastAgentLaunch(db, "agent-x")
	if err != nil {
		t.Fatalf("DBLastAgentLaunch (empty): %v", err)
	}
	if !last.IsZero() {
		t.Errorf("expected zero time for no launches, got %v", last)
	}

	t1 := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	t2 := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	DBInsertAgentCost(db, "agent-x", "auto", "", 5, t1)
	DBInsertAgentCost(db, "agent-x", "auto", "", 5, t2)

	last, err = DBLastAgentLaunch(db, "agent-x")
	if err != nil {
		t.Fatalf("DBLastAgentLaunch: %v", err)
	}
	// Compare UTC since FormatTime stores UTC
	if !last.UTC().Equal(t2.UTC()) {
		t.Errorf("last = %v, want %v", last.UTC(), t2.UTC())
	}
}

func TestDBCountRecentFailures(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	id1, _ := DBInsertAgentCost(db, "a", "pr-review", "", 5, now.Add(-1*time.Hour))
	id2, _ := DBInsertAgentCost(db, "b", "pr-review", "", 5, now.Add(-30*time.Minute))
	id3, _ := DBInsertAgentCost(db, "c", "pr-review", "", 5, now.Add(-15*time.Minute))

	DBUpdateAgentCostFinished(db, id1, 60, 1.0, 1, "failed")
	DBUpdateAgentCostFinished(db, id2, 30, 0.5, 1, "failed")
	DBUpdateAgentCostFinished(db, id3, 15, 0.3, 0, "completed")

	count, err := DBCountRecentFailures(db, "pr-review", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("DBCountRecentFailures: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

// ---------------------------------------------------------------------------
// Budget state helpers
// ---------------------------------------------------------------------------

func TestDBBudgetState_GetMissing(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	num, text, err := DBGetBudgetState(db, "nonexistent")
	if err != nil {
		t.Fatalf("DBGetBudgetState: %v", err)
	}
	if num != 0 || text != "" {
		t.Errorf("expected (0, \"\"), got (%f, %q)", num, text)
	}
}

func TestDBBudgetState_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	err = DBSetBudgetState(db, "daily_limit", 25.0, "active")
	if err != nil {
		t.Fatalf("DBSetBudgetState: %v", err)
	}

	num, text, err := DBGetBudgetState(db, "daily_limit")
	if err != nil {
		t.Fatalf("DBGetBudgetState: %v", err)
	}
	if num != 25.0 {
		t.Errorf("value_num = %f, want 25.0", num)
	}
	if text != "active" {
		t.Errorf("value_text = %q, want %q", text, "active")
	}
}

func TestDBBudgetState_Upsert(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	DBSetBudgetState(db, "cap", 10.0, "v1")
	DBSetBudgetState(db, "cap", 20.0, "v2")

	num, text, err := DBGetBudgetState(db, "cap")
	if err != nil {
		t.Fatalf("DBGetBudgetState: %v", err)
	}
	if num != 20.0 {
		t.Errorf("value_num = %f, want 20.0 (upsert)", num)
	}
	if text != "v2" {
		t.Errorf("value_text = %q, want %q (upsert)", text, "v2")
	}
}
