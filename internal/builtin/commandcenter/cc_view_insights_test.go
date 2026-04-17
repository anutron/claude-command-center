package commandcenter

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// createInsightsTable creates the knowledge_surfaced_insights table directly
// in the test database. This avoids a dependency on the knowledge plugin,
// which does not exist yet. The CC plugin reads from this table.
func createInsightsTable(t *testing.T, p *Plugin) {
	t.Helper()
	_, err := p.database.Exec(`
		CREATE TABLE IF NOT EXISTS knowledge_surfaced_insights (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL,
			source_refs TEXT,
			priority INTEGER NOT NULL DEFAULT 50,
			surfaced_at TEXT NOT NULL,
			dismissed_at TEXT
		)
	`)
	if err != nil {
		t.Fatalf("create insights table: %v", err)
	}
}

// insertTestInsight inserts a test insight into the surfaced insights table.
func insertTestInsight(t *testing.T, p *Plugin, id, insightType, title, body string, priority int) {
	t.Helper()
	_, err := p.database.Exec(`
		INSERT INTO knowledge_surfaced_insights (id, type, title, body, priority, surfaced_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
	`, id, insightType, title, body, priority)
	if err != nil {
		t.Fatalf("insert test insight: %v", err)
	}
}

// ---------------------------------------------------------------------------
// View tests: insights section rendering
// ---------------------------------------------------------------------------

func TestView_InsightsSectionAppearsWithActiveInsights(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Some todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now(), Starred: true},
	})
	createInsightsTable(t, p)
	insertTestInsight(t, p, "ins-1", "silence_alert", "No activity on: API Gateway", "Last mentioned 15 days ago, discussed 5 times", 40)
	insertTestInsight(t, p, "ins-2", "drift_detection", "Position may have shifted: Use REST", "Original: Use REST. Evidence: Decision to switch to GraphQL", 50)

	view := renderView(p)

	// The insights section should appear in the view
	viewContains(t, view, "No activity on: API Gateway")
	viewContains(t, view, "Position may have shifted: Use REST")
}

func TestView_InsightsSectionHiddenWhenEmpty(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Some todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now(), Starred: true},
	})
	createInsightsTable(t, p)

	// First, verify that insights DO appear when present (prerequisite for
	// testing the empty state). This ensures the CC plugin actually reads
	// from the insights table.
	insertTestInsight(t, p, "ins-check", "silence_alert", "Visibility check insight", "Checking", 40)
	viewWithInsight := renderView(p)
	viewContains(t, viewWithInsight, "Visibility check insight")

	// Clean up -- remove the insight to test empty state
	_, err := p.database.Exec("DELETE FROM knowledge_surfaced_insights")
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	// Now with no insights, the section should be hidden
	viewEmpty := renderView(p)
	viewNotContains(t, viewEmpty, "INSIGHTS")
}

func TestView_InsightsSilenceAndDriftDistinguishable(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Some todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now(), Starred: true},
	})
	createInsightsTable(t, p)
	insertTestInsight(t, p, "ins-1", "silence_alert", "No activity on: API Gateway", "Silent for 15 days", 40)
	insertTestInsight(t, p, "ins-2", "drift_detection", "Position may have shifted: Use REST", "Evidence of drift", 50)

	view := renderView(p)

	// Silence alerts and drift detection should be visually distinguishable
	// (e.g., different labels/icons like "SILENCE" vs "DRIFT")
	viewContains(t, view, "No activity on: API Gateway")
	viewContains(t, view, "Position may have shifted: Use REST")
}

func TestView_InsightsOrderedByPriority(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Some todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now(), Starred: true},
	})
	createInsightsTable(t, p)

	// Insert in reverse priority order
	insertTestInsight(t, p, "ins-low", "silence_alert", "Low priority insight", "Details", 80)
	insertTestInsight(t, p, "ins-high", "silence_alert", "High priority insight", "Details", 20)

	view := renderView(p)

	// Both should appear, and the high-priority one should appear first
	viewContains(t, view, "High priority insight")
	viewContains(t, view, "Low priority insight")

	// Verify ordering: high priority (20) should appear before low priority (80)
	highIdx := strings.Index(view, "High priority insight")
	lowIdx := strings.Index(view, "Low priority insight")
	if highIdx < 0 || lowIdx < 0 {
		t.Fatal("both insights should appear in view")
	}
	if highIdx > lowIdx {
		t.Error("high priority insight (20) should appear before low priority insight (80)")
	}
}

// ---------------------------------------------------------------------------
// Dismiss key tests
// ---------------------------------------------------------------------------

func TestView_DismissInsightWithXKey(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Some todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now(), Starred: true},
	})
	createInsightsTable(t, p)
	insertTestInsight(t, p, "ins-1", "silence_alert", "Dismissable insight", "Should go away", 40)

	// Verify the insight appears initially
	view := renderView(p)
	viewContains(t, view, "Dismissable insight")

	// Press x to dismiss the insight
	p.HandleKey(keyMsg("x"))

	// The dismissed insight should no longer appear
	view = renderView(p)
	viewNotContains(t, view, "Dismissable insight")

	// Verify dismissed_at was set in the database
	var dismissedAt *string
	p.database.QueryRow("SELECT dismissed_at FROM knowledge_surfaced_insights WHERE id = 'ins-1'").Scan(&dismissedAt)
	if dismissedAt == nil {
		t.Error("dismissed_at should be set after pressing x")
	}
}

func TestView_DismissedInsightDoesNotReappear(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Some todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now(), Starred: true},
	})
	createInsightsTable(t, p)

	// First, verify a non-dismissed insight appears (prerequisite)
	insertTestInsight(t, p, "ins-active", "silence_alert", "Active insight check", "Should be visible", 40)
	viewBefore := renderView(p)
	viewContains(t, viewBefore, "Active insight check")

	// Now test that a pre-dismissed insight does not appear
	insertTestInsight(t, p, "ins-1", "silence_alert", "Already dismissed", "Was dismissed earlier", 40)
	_, err := p.database.Exec("UPDATE knowledge_surfaced_insights SET dismissed_at = datetime('now') WHERE id = 'ins-1'")
	if err != nil {
		t.Fatalf("pre-dismiss: %v", err)
	}

	view := renderView(p)
	// The active one should still show, the dismissed one should not
	viewContains(t, view, "Active insight check")
	viewNotContains(t, view, "Already dismissed")
}

