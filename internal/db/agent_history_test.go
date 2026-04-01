package db

import (
	"testing"
	"time"
)

func TestDBLoadAgentHistory(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()

	// Insert a todo — the todo's id IS the agent_id in cc_agent_costs
	_, err := db.Exec(`INSERT INTO cc_todos (id, display_id, title, status, source, created_at, updated_at)
		VALUES ('agent-abc', 113, 'Fix auth bug', 'running', 'github', ?, ?)`,
		FormatTime(now), FormatTime(now))
	if err != nil {
		t.Fatal(err)
	}

	// Insert an agent cost row
	_, err = db.Exec(`INSERT INTO cc_agent_costs (agent_id, automation, started_at, status, cost_usd, input_tokens, output_tokens)
		VALUES ('agent-abc', 'todo', ?, 'completed', 0.42, 1000, 500)`,
		FormatTime(now.Add(-10*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}

	// Insert a PR — the PR's id IS the agent_id in cc_agent_costs
	_, err = db.Exec(`INSERT INTO cc_pull_requests (id, number, repo, title, url, author, created_at, updated_at, last_activity_at, fetched_at, agent_status, agent_category)
		VALUES ('agent-pr-47', 47, 'owner/repo', 'Add feature', 'https://github.com/owner/repo/pull/47', 'user', ?, ?, ?, ?, 'completed', 'review')`,
		FormatTime(now), FormatTime(now), FormatTime(now), FormatTime(now))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO cc_agent_costs (agent_id, automation, started_at, status, cost_usd)
		VALUES ('agent-pr-47', 'pr-review', ?, 'completed', 0.18)`,
		FormatTime(now.Add(-5*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}

	// Insert an old agent (>24h ago) that should NOT appear
	_, err = db.Exec(`INSERT INTO cc_agent_costs (agent_id, automation, started_at, status, cost_usd)
		VALUES ('agent-old', 'todo', ?, 'completed', 1.00)`,
		FormatTime(now.Add(-25*time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	entries, err := DBLoadAgentHistory(db, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Most recent first (pr agent started 5m ago)
	if entries[0].AgentID != "agent-pr-47" {
		t.Errorf("expected first entry agent-pr-47, got %s", entries[0].AgentID)
	}
	if entries[0].OriginType != "pr" {
		t.Errorf("expected origin_type pr, got %s", entries[0].OriginType)
	}
	if entries[0].OriginLabel == "" {
		t.Error("expected non-empty origin label for PR agent")
	}

	if entries[1].AgentID != "agent-abc" {
		t.Errorf("expected second entry agent-abc, got %s", entries[1].AgentID)
	}
	if entries[1].OriginType != "todo" {
		t.Errorf("expected origin_type todo, got %s", entries[1].OriginType)
	}
	if entries[1].OriginLabel != "TODO #113 \u2014 Fix auth bug" {
		t.Errorf("unexpected origin label: %s", entries[1].OriginLabel)
	}
}
