package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateCommandCenter_PropagatesExecErrors(t *testing.T) {
	// Set up a real DB with schema
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	// Drop cc_calendar_cache to force migrateCachedEvent to fail
	_, err = database.Exec("DROP TABLE IF EXISTS cc_calendar_cache")
	if err != nil {
		t.Fatalf("drop table: %v", err)
	}

	// Create a JSON file with calendar events that will fail to migrate
	cc := CommandCenter{
		Calendar: CalendarData{
			Today: []CalendarEvent{
				{
					Title: "Test Meeting",
					Start: time.Now(),
					End:   time.Now().Add(1 * time.Hour),
				},
			},
		},
		GeneratedAt: time.Now(),
	}
	data, err := json.Marshal(cc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	ccPath := filepath.Join(dir, "command-center.json")
	if err := os.WriteFile(ccPath, data, 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	// migrateCommandCenter should return an error because the calendar INSERT will fail
	// BUG: It uses _, _ = tx.Exec(...) and silently swallows the error
	err = migrateCommandCenter(database, ccPath)
	if err != nil {
		// Good — error was propagated. Test passes once bug is fixed.
		t.Logf("got expected error: %v", err)
		return
	}

	// If no error, verify the generated_at was actually saved (it should be, since
	// that part succeeds). But the calendar data was silently lost.
	var value string
	err = database.QueryRow("SELECT value FROM cc_meta WHERE key = 'generated_at'").Scan(&value)
	if err != nil {
		t.Fatalf("generated_at not saved despite 'successful' migration: %v", err)
	}

	// The real check: the transaction committed without error, but calendar data is gone.
	// This proves the bug — errors were silently swallowed.
	var count int
	// cc_calendar_cache was dropped, so this query itself will fail
	err = database.QueryRow("SELECT COUNT(*) FROM cc_calendar_cache").Scan(&count)
	if err != nil {
		// Table doesn't exist — the silent error in migrateCachedEvent was swallowed,
		// and the transaction still committed. This is the bug.
		t.Errorf("migration 'succeeded' but calendar table doesn't exist — error was silently swallowed: %v", err)
	}
}
