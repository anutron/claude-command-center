package plugin

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunMigrationsCreatesTable(t *testing.T) {
	db := openTestDB(t)

	migrations := []Migration{
		{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS plugin_test_items (id TEXT PRIMARY KEY)`},
	}

	if err := RunMigrations(db, "test", migrations); err != nil {
		t.Fatal(err)
	}

	// Verify the items table was created
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM plugin_test_items`).Scan(&count)
	if err != nil {
		t.Fatal("expected plugin_test_items table to exist:", err)
	}

	// Verify the migration was recorded
	err = db.QueryRow(
		`SELECT COUNT(*) FROM ccc_plugin_migrations WHERE plugin_slug = 'test' AND version = 1`,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 migration record, got %d", count)
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	db := openTestDB(t)

	migrations := []Migration{
		{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS plugin_idem_items (id TEXT PRIMARY KEY)`},
	}

	if err := RunMigrations(db, "idem", migrations); err != nil {
		t.Fatal(err)
	}
	// Run again — should be a no-op
	if err := RunMigrations(db, "idem", migrations); err != nil {
		t.Fatal("expected idempotent re-run, got:", err)
	}
}

func TestRunMigrationsIncremental(t *testing.T) {
	db := openTestDB(t)

	// Apply v1
	v1 := []Migration{
		{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS plugin_inc_items (id TEXT PRIMARY KEY)`},
	}
	if err := RunMigrations(db, "inc", v1); err != nil {
		t.Fatal(err)
	}

	// Apply v1 + v2
	v2 := []Migration{
		{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS plugin_inc_items (id TEXT PRIMARY KEY)`},
		{Version: 2, SQL: `ALTER TABLE plugin_inc_items ADD COLUMN name TEXT`},
	}
	if err := RunMigrations(db, "inc", v2); err != nil {
		t.Fatal(err)
	}

	// Verify v2 applied
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM ccc_plugin_migrations WHERE plugin_slug = 'inc'`,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 migration records, got %d", count)
	}
}

func TestRunMigrationsNilDB(t *testing.T) {
	err := RunMigrations(nil, "test", []Migration{{Version: 1, SQL: "SELECT 1"}})
	if err != nil {
		t.Errorf("expected nil error for nil db, got: %v", err)
	}
}

func TestRunMigrationsEmpty(t *testing.T) {
	db := openTestDB(t)
	err := RunMigrations(db, "test", nil)
	if err != nil {
		t.Errorf("expected nil error for empty migrations, got: %v", err)
	}
}

func TestRunMigrationsIsolatesPlugins(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db, "alpha", []Migration{
		{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS plugin_alpha_t (id TEXT)`},
	}); err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(db, "beta", []Migration{
		{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS plugin_beta_t (id TEXT)`},
	}); err != nil {
		t.Fatal(err)
	}

	// Both should have version 1 independently
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM ccc_plugin_migrations`).Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 total records, got %d", count)
	}
}
