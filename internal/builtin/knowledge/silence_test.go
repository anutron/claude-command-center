package knowledge

import (
	"database/sql"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupSilenceDB creates a test database with knowledge tables populated
// for silence analysis testing.
func setupSilenceDB(t *testing.T) *sql.DB {
	t.Helper()
	database := openTestDB(t)

	p := New()
	migrations := p.Migrations()
	if len(migrations) == 0 {
		t.Fatal("knowledge plugin migrations not implemented; silence tests require tables to exist")
	}
	if err := plugin.RunMigrations(database, p.Slug(), migrations); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return database
}

// daysAgo returns an RFC3339 timestamp n days in the past.
func daysAgo(n int) string {
	return time.Now().Add(-time.Duration(n) * 24 * time.Hour).Format(time.RFC3339)
}

// insertTestTopic inserts a topic with controlled timestamps and mention count.
func insertTestTopic(t *testing.T, database *sql.DB, id, name string, mentionCount int, lastSeen string) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO knowledge_topics (id, name, description, first_seen, last_seen, mention_count)
		VALUES (?, ?, 'test description', ?, ?, ?)`,
		id, name, daysAgo(30), lastSeen, mentionCount)
	if err != nil {
		t.Fatalf("insert test topic %q: %v", name, err)
	}
}

// insertTestThread inserts an open thread with controlled timestamps.
func insertTestThread(t *testing.T, database *sql.DB, id, description, firstRaisedBy, lastActivityAt, status string) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO knowledge_open_threads
		(id, description, blocking_on, topic_id, first_raised_by, source, source_ref, first_raised_at, last_activity_at, status, resolved_by)
		VALUES (?, ?, NULL, NULL, ?, 'granola', 'ref-1', ?, ?, ?, NULL)`,
		id, description, firstRaisedBy, daysAgo(30), lastActivityAt, status)
	if err != nil {
		t.Fatalf("insert test thread %q: %v", description, err)
	}
}

// countInsights counts the number of insights of a given type.
func countInsights(t *testing.T, database *sql.DB, insightType string) int {
	t.Helper()
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM knowledge_surfaced_insights WHERE type = ?", insightType).Scan(&count)
	if err != nil {
		t.Fatalf("count insights: %v", err)
	}
	return count
}

// countActiveInsights counts non-dismissed insights.
func countActiveInsights(t *testing.T, database *sql.DB) int {
	t.Helper()
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM knowledge_surfaced_insights WHERE dismissed_at IS NULL").Scan(&count)
	if err != nil {
		t.Fatalf("count active insights: %v", err)
	}
	return count
}

// ---------------------------------------------------------------------------
// Silence alert tests: qualifying conditions
// ---------------------------------------------------------------------------

func TestSilence_TopicWithHighMentionCountAndOldLastSeen(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Topic with mention_count > 3 and last_seen > 10 days ago -- should trigger alert
	insertTestTopic(t, database, "topic-1", "Important Topic", 5, daysAgo(15))

	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("RunSilenceAnalysis: %v", err)
	}

	if countInsights(t, database, "silence_alert") == 0 {
		t.Error("expected a silence alert for topic with mention_count > 3 and last_seen > 10 days")
	}
}

func TestSilence_ThreadRaisedByAaronAndOld(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Open thread raised by Aaron with last_activity_at > 7 days ago -- should trigger alert
	insertTestThread(t, database, "thread-1", "Rate limiter design", "Aaron", daysAgo(10), "open")

	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("RunSilenceAnalysis: %v", err)
	}

	if countInsights(t, database, "silence_alert") == 0 {
		t.Error("expected a silence alert for open thread raised by Aaron with last_activity > 7 days")
	}
}

// ---------------------------------------------------------------------------
// Silence alert tests: disqualified items
// ---------------------------------------------------------------------------

func TestSilence_TopicWithLowMentionCount(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Topic with mention_count <= 3 -- should NOT trigger alert even if old
	insertTestTopic(t, database, "topic-2", "Minor Topic", 2, daysAgo(15))

	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("RunSilenceAnalysis: %v", err)
	}

	if countInsights(t, database, "silence_alert") > 0 {
		t.Error("should not create silence alert for topic with mention_count <= 3")
	}
}

func TestSilence_TopicRecentlyMentioned(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Topic with high mention count but recently mentioned -- should NOT trigger
	insertTestTopic(t, database, "topic-3", "Active Topic", 10, daysAgo(3))

	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("RunSilenceAnalysis: %v", err)
	}

	if countInsights(t, database, "silence_alert") > 0 {
		t.Error("should not create silence alert for recently mentioned topic")
	}
}

func TestSilence_ThreadNotRaisedByAaron(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Open thread NOT raised by Aaron -- should NOT trigger
	insertTestThread(t, database, "thread-2", "Someone else's question", "Zach", daysAgo(10), "open")

	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("RunSilenceAnalysis: %v", err)
	}

	if countInsights(t, database, "silence_alert") > 0 {
		t.Error("should not create silence alert for thread not raised by Aaron")
	}
}

func TestSilence_ThreadRecentlyActive(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Open thread raised by Aaron but recently active -- should NOT trigger
	insertTestThread(t, database, "thread-3", "Recent thread", "Aaron", daysAgo(3), "open")

	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("RunSilenceAnalysis: %v", err)
	}

	if countInsights(t, database, "silence_alert") > 0 {
		t.Error("should not create silence alert for thread with recent activity")
	}
}

func TestSilence_ResolvedThreadNotAlerted(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Resolved thread -- should NOT trigger even if old and raised by Aaron
	insertTestThread(t, database, "thread-4", "Resolved issue", "Aaron", daysAgo(15), "resolved")

	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("RunSilenceAnalysis: %v", err)
	}

	if countInsights(t, database, "silence_alert") > 0 {
		t.Error("should not create silence alert for resolved thread")
	}
}

// ---------------------------------------------------------------------------
// Idempotence tests
// ---------------------------------------------------------------------------

func TestSilence_IdempotenceNoDuplicates(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	insertTestTopic(t, database, "topic-1", "Important Topic", 5, daysAgo(15))

	// Run analysis twice
	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("first RunSilenceAnalysis: %v", err)
	}

	err = RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("second RunSilenceAnalysis: %v", err)
	}

	count := countInsights(t, database, "silence_alert")
	if count > 1 {
		t.Errorf("expected at most 1 silence alert after running twice, got %d", count)
	}
}

func TestSilence_DismissedAlertNotResurfaced(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	insertTestTopic(t, database, "topic-1", "Important Topic", 5, daysAgo(15))

	// Run analysis to create the alert
	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("first RunSilenceAnalysis: %v", err)
	}

	// Dismiss the alert
	_, err = database.Exec("UPDATE knowledge_surfaced_insights SET dismissed_at = datetime('now') WHERE type = 'silence_alert'")
	if err != nil {
		t.Fatalf("dismiss alert: %v", err)
	}

	// Run analysis again -- should not re-surface the dismissed alert
	err = RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("second RunSilenceAnalysis: %v", err)
	}

	active := countActiveInsights(t, database)
	if active > 0 {
		t.Error("dismissed silence alert should not be re-surfaced")
	}
}

// ---------------------------------------------------------------------------
// Stale removal tests
// ---------------------------------------------------------------------------

func TestSilence_StaleInsightRemovedWhenTopicMentionedAgain(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Create a topic that qualifies for silence alert
	insertTestTopic(t, database, "topic-1", "Important Topic", 5, daysAgo(15))

	// Run analysis to create the alert
	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("first RunSilenceAnalysis: %v", err)
	}

	initialCount := countInsights(t, database, "silence_alert")
	if initialCount == 0 {
		t.Fatal("expected a silence alert to be created")
	}

	// Update the topic's last_seen to be recent (simulating it was mentioned again)
	_, err = database.Exec("UPDATE knowledge_topics SET last_seen = ? WHERE id = 'topic-1'", daysAgo(1))
	if err != nil {
		t.Fatalf("update last_seen: %v", err)
	}

	// Run analysis again -- the stale alert should be removed
	err = RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("second RunSilenceAnalysis: %v", err)
	}

	finalCount := countInsights(t, database, "silence_alert")
	if finalCount > 0 {
		t.Error("silence alert should be removed when topic is mentioned again (condition no longer holds)")
	}
}

func TestSilence_StaleInsightRemovedWhenThreadResolved(t *testing.T) {
	database := setupSilenceDB(t)
	bus := plugin.NewBus()

	// Create a qualifying thread
	insertTestThread(t, database, "thread-1", "Rate limiter design", "Aaron", daysAgo(10), "open")

	// Run analysis to create the alert
	err := RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("first RunSilenceAnalysis: %v", err)
	}

	if countInsights(t, database, "silence_alert") == 0 {
		t.Fatal("expected a silence alert for the open thread")
	}

	// Resolve the thread
	_, err = database.Exec("UPDATE knowledge_open_threads SET status = 'resolved' WHERE id = 'thread-1'")
	if err != nil {
		t.Fatalf("resolve thread: %v", err)
	}

	// Run analysis again -- stale alert should be removed
	err = RunSilenceAnalysis(database, bus, DefaultSilenceConfig())
	if err != nil {
		t.Fatalf("second RunSilenceAnalysis: %v", err)
	}

	finalCount := countInsights(t, database, "silence_alert")
	if finalCount > 0 {
		t.Error("silence alert should be removed when thread is resolved")
	}
}
