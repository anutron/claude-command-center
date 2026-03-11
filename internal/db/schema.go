package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// OpenDB opens (or creates) the SQLite database at dbPath, enables WAL mode,
// and runs the idempotent schema migration.
func OpenDB(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := migrateSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrateSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cc_todos (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			source TEXT NOT NULL DEFAULT 'manual',
			source_ref TEXT,
			context TEXT,
			detail TEXT,
			who_waiting TEXT,
			project_dir TEXT,
			due TEXT,
			effort TEXT,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			updated_at TEXT NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_cc_todos_source_ref
			ON cc_todos(source_ref) WHERE source_ref IS NOT NULL AND source_ref != '';

		CREATE TABLE IF NOT EXISTS cc_threads (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL DEFAULT 'manual',
			title TEXT NOT NULL,
			url TEXT,
			repo TEXT,
			project_dir TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			summary TEXT,
			source_ref TEXT,
			created_at TEXT NOT NULL,
			paused_at TEXT,
			completed_at TEXT,
			updated_at TEXT NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_cc_threads_source_ref
			ON cc_threads(source_ref) WHERE source_ref IS NOT NULL AND source_ref != '';

		CREATE TABLE IF NOT EXISTS cc_calendar_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			day TEXT NOT NULL,
			title TEXT NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			all_day INTEGER NOT NULL DEFAULT 0,
			declined INTEGER NOT NULL DEFAULT 0,
			calendar_id TEXT NOT NULL DEFAULT '',
			cached_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_suggestions (
			id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			focus TEXT,
			ranked_todo_ids TEXT DEFAULT '[]',
			reasons TEXT DEFAULT '{}',
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_pending_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			todo_id TEXT NOT NULL,
			duration_minutes INTEGER,
			requested_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_meta (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_bookmarks (
			session_id TEXT PRIMARY KEY,
			project TEXT,
			repo TEXT,
			branch TEXT,
			label TEXT,
			summary TEXT,
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_learned_paths (
			path TEXT PRIMARY KEY,
			added_at TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	// Add calendar_id column if missing (added after initial schema)
	_, _ = db.Exec(`ALTER TABLE cc_calendar_cache ADD COLUMN calendar_id TEXT NOT NULL DEFAULT ''`)

	// Add session_id column if missing (added for CLI todo creation with session links)
	_, _ = db.Exec(`ALTER TABLE cc_todos ADD COLUMN session_id TEXT`)

	// Add sort_order column to learned paths if missing (added for manual reordering)
	_, _ = db.Exec(`ALTER TABLE cc_learned_paths ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`)
	// Backfill sort_order from added_at order for existing rows
	_, _ = db.Exec(`UPDATE cc_learned_paths SET sort_order = (
		SELECT COUNT(*) FROM cc_learned_paths p2 WHERE p2.added_at < cc_learned_paths.added_at
	) WHERE sort_order = 0`)

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func ParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return time.Time{}
		}
	}
	return t.Local()
}
