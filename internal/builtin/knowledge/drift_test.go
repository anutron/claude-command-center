package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupDriftDB creates a test database with knowledge tables and seed data
// for drift detection testing.
func setupDriftDB(t *testing.T) *sql.DB {
	t.Helper()
	database := openTestDB(t)

	p := New()
	migrations := p.Migrations()
	if len(migrations) == 0 {
		t.Fatal("knowledge plugin migrations not implemented; drift tests require tables to exist")
	}
	if err := plugin.RunMigrations(database, p.Slug(), migrations); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return database
}

// seedDriftPositions inserts test positions for drift detection.
func seedDriftPositions(t *testing.T, database *sql.DB) {
	t.Helper()

	// Insert a topic
	_, err := database.Exec(`INSERT INTO knowledge_topics (id, name, description, first_seen, last_seen, mention_count)
		VALUES ('topic-api', 'API Gateway', 'API gateway discussion', ?, ?, 5)`,
		daysAgo(45), daysAgo(5))
	if err != nil {
		t.Fatalf("insert topic: %v", err)
	}

	// Insert Aaron's position from 30 days ago (within 60-day scope)
	_, err = database.Exec(`INSERT INTO knowledge_positions (id, holder, position, topic_id, source, source_ref, stated_at, extracted_at)
		VALUES ('pos-aaron-1', 'Aaron', 'We should use REST for the API gateway', 'topic-api', 'granola', 'meeting-100', ?, ?)`,
		time.Now().Add(-30*24*time.Hour).Format(time.RFC3339),
		time.Now().Add(-30*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert Aaron position: %v", err)
	}

	// Insert a newer decision that contradicts the position
	_, err = database.Exec(`INSERT INTO knowledge_decisions (id, title, description, alternatives, reasoning, participants, aaron_present, source, source_ref, decided_at, extracted_at)
		VALUES ('dec-1', 'Switch to GraphQL', 'Decision to switch the API gateway to GraphQL', 'REST', 'Better developer experience', '["Aaron","Zach"]', 1, 'granola', 'meeting-200', ?, ?)`,
		daysAgo(5), daysAgo(5))
	if err != nil {
		t.Fatalf("insert decision: %v", err)
	}
}

// seedOldPosition inserts a position that is outside the 60-day scope.
func seedOldPosition(t *testing.T, database *sql.DB) {
	t.Helper()

	_, err := database.Exec(`INSERT INTO knowledge_topics (id, name, description, first_seen, last_seen, mention_count)
		VALUES ('topic-old', 'Old Topic', 'An old topic', ?, ?, 2)`,
		daysAgo(120), daysAgo(90))
	if err != nil {
		t.Fatalf("insert old topic: %v", err)
	}

	_, err = database.Exec(`INSERT INTO knowledge_positions (id, holder, position, topic_id, source, source_ref, stated_at, extracted_at)
		VALUES ('pos-old', 'Aaron', 'Old position from 90 days ago', 'topic-old', 'granola', 'meeting-old', ?, ?)`,
		time.Now().Add(-90*24*time.Hour).Format(time.RFC3339),
		time.Now().Add(-90*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert old position: %v", err)
	}
}

// seedNonAaronPosition inserts a position by someone other than Aaron.
func seedNonAaronPosition(t *testing.T, database *sql.DB) {
	t.Helper()

	_, err := database.Exec(`INSERT INTO knowledge_positions (id, holder, position, topic_id, source, source_ref, stated_at, extracted_at)
		VALUES ('pos-zach-1', 'Zach', 'Zach prefers gRPC', 'topic-api', 'granola', 'meeting-300', ?, ?)`,
		daysAgo(20), daysAgo(20))
	if err != nil {
		t.Fatalf("insert Zach position: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mock LLM for drift detection
// ---------------------------------------------------------------------------

// mockDriftLLM returns pre-canned drift detection results.
type mockDriftLLM struct {
	response string
}

func (m *mockDriftLLM) Complete(ctx context.Context, prompt string) (string, error) {
	return m.response, nil
}

// newDriftDetectedLLM creates a mock that reports drift for a position.
func newDriftDetectedLLM() *mockDriftLLM {
	result := []map[string]string{
		{
			"position_id":       "pos-aaron-1",
			"original_position": "We should use REST for the API gateway",
			"evidence_of_shift": "A newer decision 'Switch to GraphQL' was made, contradicting the REST position",
			"shifted_to":        "GraphQL for the API gateway",
		},
	}
	data, _ := json.Marshal(result)
	return &mockDriftLLM{response: string(data)}
}

// newNoDriftLLM creates a mock that reports no drift.
func newNoDriftLLM() *mockDriftLLM {
	return &mockDriftLLM{response: "[]"}
}

// ---------------------------------------------------------------------------
// Drift detection tests: happy path
// ---------------------------------------------------------------------------

func TestDrift_WritesInsightWhenDriftDetected(t *testing.T) {
	database := setupDriftDB(t)
	bus := plugin.NewBus()
	seedDriftPositions(t, database)

	model := newDriftDetectedLLM()

	err := RunDriftDetection(context.Background(), database, model, bus)
	if err != nil {
		t.Fatalf("RunDriftDetection: %v", err)
	}

	count := countInsights(t, database, "drift_detection")
	if count == 0 {
		t.Error("expected a drift detection insight when LLM reports drift")
	}

	// Verify the insight contains meaningful content
	var title, body string
	database.QueryRow("SELECT title, body FROM knowledge_surfaced_insights WHERE type = 'drift_detection'").Scan(&title, &body)
	if title == "" {
		t.Error("drift insight should have a non-empty title")
	}
	if body == "" {
		t.Error("drift insight should have a non-empty body")
	}
}

// ---------------------------------------------------------------------------
// Drift detection tests: no drift
// ---------------------------------------------------------------------------

func TestDrift_NoInsightWhenNoDriftDetected(t *testing.T) {
	database := setupDriftDB(t)
	bus := plugin.NewBus()
	seedDriftPositions(t, database)

	model := newNoDriftLLM()

	err := RunDriftDetection(context.Background(), database, model, bus)
	if err != nil {
		t.Fatalf("RunDriftDetection: %v", err)
	}

	count := countInsights(t, database, "drift_detection")
	if count > 0 {
		t.Error("should not write drift insights when LLM reports no drift")
	}
}

// ---------------------------------------------------------------------------
// Drift detection tests: scope
// ---------------------------------------------------------------------------

func TestDrift_OnlyConsidersAaronPositions(t *testing.T) {
	database := setupDriftDB(t)
	bus := plugin.NewBus()
	seedDriftPositions(t, database)
	seedNonAaronPosition(t, database)

	// The mock always reports drift for pos-aaron-1.
	// If it also evaluated Zach's position, that would be a bug.
	model := newDriftDetectedLLM()

	err := RunDriftDetection(context.Background(), database, model, bus)
	if err != nil {
		t.Fatalf("RunDriftDetection: %v", err)
	}

	// There should be exactly 1 drift insight (for Aaron's position only)
	count := countInsights(t, database, "drift_detection")
	if count != 1 {
		t.Errorf("expected exactly 1 drift insight (Aaron only), got %d", count)
	}
}

func TestDrift_OnlyConsidersLast60Days(t *testing.T) {
	database := setupDriftDB(t)
	bus := plugin.NewBus()
	seedOldPosition(t, database)

	// Mock that would report drift if the old position were evaluated
	result := []map[string]string{
		{
			"position_id":       "pos-old",
			"original_position": "Old position from 90 days ago",
			"evidence_of_shift": "Something changed",
			"shifted_to":        "New stance",
		},
	}
	data, _ := json.Marshal(result)
	model := &mockDriftLLM{response: string(data)}

	err := RunDriftDetection(context.Background(), database, model, bus)
	if err != nil {
		t.Fatalf("RunDriftDetection: %v", err)
	}

	// Should not create a drift insight for a position older than 60 days
	count := countInsights(t, database, "drift_detection")
	if count > 0 {
		t.Error("should not evaluate positions older than 60 days for drift detection")
	}
}
