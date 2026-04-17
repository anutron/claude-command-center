package knowledge

import (
	"database/sql"
	"testing"

	"github.com/anutron/claude-command-center/internal/plugin"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// runKnowledgeMigrations runs the knowledge plugin's migrations on the given db.
func runKnowledgeMigrations(t *testing.T, database *sql.DB) {
	t.Helper()
	p := New()
	migrations := p.Migrations()
	if len(migrations) == 0 {
		t.Fatal("knowledge plugin returned no migrations")
	}
	if err := plugin.RunMigrations(database, p.Slug(), migrations); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}

// tableExists checks whether a table exists in the database.
func tableExists(t *testing.T, database *sql.DB, tableName string) bool {
	t.Helper()
	var count int
	err := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tableName,
	).Scan(&count)
	if err != nil {
		t.Fatalf("checking table %q: %v", tableName, err)
	}
	return count > 0
}

// indexExists checks whether an index exists in the database.
func indexExists(t *testing.T, database *sql.DB, indexName string) bool {
	t.Helper()
	var count int
	err := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, indexName,
	).Scan(&count)
	if err != nil {
		t.Fatalf("checking index %q: %v", indexName, err)
	}
	return count > 0
}

// columnExists checks whether a column exists on a table.
func columnExists(t *testing.T, database *sql.DB, tableName, columnName string) bool {
	t.Helper()
	rows, err := database.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", tableName, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt *string
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == columnName {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Migration tests: verify all six tables are created with correct columns
// ---------------------------------------------------------------------------

func TestMigration_TopicsTableCreated(t *testing.T) {
	db := openTestDB(t)
	runKnowledgeMigrations(t, db)

	if !tableExists(t, db, "knowledge_topics") {
		t.Error("knowledge_topics table should exist after migrations")
	}

	for _, col := range []string{"id", "name", "description", "first_seen", "last_seen", "mention_count"} {
		if !columnExists(t, db, "knowledge_topics", col) {
			t.Errorf("knowledge_topics should have column %q", col)
		}
	}
}

func TestMigration_DecisionsTableCreated(t *testing.T) {
	db := openTestDB(t)
	runKnowledgeMigrations(t, db)

	if !tableExists(t, db, "knowledge_decisions") {
		t.Error("knowledge_decisions table should exist after migrations")
	}

	for _, col := range []string{"id", "title", "description", "alternatives", "reasoning", "participants", "aaron_present", "source", "source_ref", "decided_at", "extracted_at"} {
		if !columnExists(t, db, "knowledge_decisions", col) {
			t.Errorf("knowledge_decisions should have column %q", col)
		}
	}
}

func TestMigration_PositionsTableCreated(t *testing.T) {
	db := openTestDB(t)
	runKnowledgeMigrations(t, db)

	if !tableExists(t, db, "knowledge_positions") {
		t.Error("knowledge_positions table should exist after migrations")
	}

	for _, col := range []string{"id", "holder", "position", "topic_id", "source", "source_ref", "stated_at", "extracted_at"} {
		if !columnExists(t, db, "knowledge_positions", col) {
			t.Errorf("knowledge_positions should have column %q", col)
		}
	}
}

func TestMigration_OpenThreadsTableCreated(t *testing.T) {
	db := openTestDB(t)
	runKnowledgeMigrations(t, db)

	if !tableExists(t, db, "knowledge_open_threads") {
		t.Error("knowledge_open_threads table should exist after migrations")
	}

	for _, col := range []string{"id", "description", "blocking_on", "topic_id", "first_raised_by", "source", "source_ref", "first_raised_at", "last_activity_at", "status", "resolved_by"} {
		if !columnExists(t, db, "knowledge_open_threads", col) {
			t.Errorf("knowledge_open_threads should have column %q", col)
		}
	}
}

func TestMigration_EdgesTableCreated(t *testing.T) {
	db := openTestDB(t)
	runKnowledgeMigrations(t, db)

	if !tableExists(t, db, "knowledge_edges") {
		t.Error("knowledge_edges table should exist after migrations")
	}

	for _, col := range []string{"from_id", "from_type", "to_id", "to_type", "relationship", "created_at"} {
		if !columnExists(t, db, "knowledge_edges", col) {
			t.Errorf("knowledge_edges should have column %q", col)
		}
	}
}

func TestMigration_SurfacedInsightsTableCreated(t *testing.T) {
	db := openTestDB(t)
	runKnowledgeMigrations(t, db)

	if !tableExists(t, db, "knowledge_surfaced_insights") {
		t.Error("knowledge_surfaced_insights table should exist after migrations")
	}

	for _, col := range []string{"id", "type", "title", "body", "source_refs", "priority", "surfaced_at", "dismissed_at"} {
		if !columnExists(t, db, "knowledge_surfaced_insights", col) {
			t.Errorf("knowledge_surfaced_insights should have column %q", col)
		}
	}
}

// ---------------------------------------------------------------------------
// Migration tests: verify indexes exist
// ---------------------------------------------------------------------------

func TestMigration_IndexesExist(t *testing.T) {
	db := openTestDB(t)
	runKnowledgeMigrations(t, db)

	expectedIndexes := []string{
		"idx_knowledge_topics_name",
		"idx_knowledge_positions_holder_topic",
		"idx_knowledge_open_threads_last_activity",
		"idx_knowledge_open_threads_raised_by",
		"idx_knowledge_surfaced_insights_dismissed",
	}

	for _, idx := range expectedIndexes {
		if !indexExists(t, db, idx) {
			t.Errorf("index %q should exist after migrations", idx)
		}
	}
}

// ---------------------------------------------------------------------------
// Migration tests: verify backfill state tracking row
// ---------------------------------------------------------------------------

func TestMigration_BackfillStateRowCreated(t *testing.T) {
	db := openTestDB(t)
	runKnowledgeMigrations(t, db)

	// The migration should create a knowledge_backfill_state table with
	// tracking rows for resumability.
	if !tableExists(t, db, "knowledge_backfill_state") {
		t.Fatal("knowledge_backfill_state table should exist after migrations")
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM knowledge_backfill_state").Scan(&count)
	if err != nil {
		t.Fatalf("querying knowledge_backfill_state: %v", err)
	}
	if count == 0 {
		t.Error("knowledge_backfill_state should have at least one tracking row after migration")
	}
}
