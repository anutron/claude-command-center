package plugin

import (
	"database/sql"
	"strings"
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

// --- ValidateExternalMigrationSQL tests ---

func TestValidateExternalMigrationSQL_AllowedStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{"create table", "CREATE TABLE IF NOT EXISTS myplugin_items (id TEXT PRIMARY KEY)"},
		{"create table lowercase", "create table if not exists myplugin_items (id text)"},
		{"create table mixed case", "Create Table If Not Exists myplugin_items (id TEXT)"},
		{"create index", "CREATE INDEX IF NOT EXISTS myplugin_idx_name ON myplugin_items (name)"},
		{"create unique index", "CREATE UNIQUE INDEX IF NOT EXISTS myplugin_idx_u ON myplugin_items (name)"},
		{"alter table", "ALTER TABLE myplugin_items ADD COLUMN name TEXT"},
		{"drop table", "DROP TABLE IF EXISTS myplugin_items"},
		{"drop index", "DROP INDEX IF EXISTS myplugin_idx_name"},
		{"multiple statements", "CREATE TABLE IF NOT EXISTS myplugin_a (id TEXT); CREATE INDEX IF NOT EXISTS myplugin_a_idx ON myplugin_a (id);"},
		{"leading whitespace", "  CREATE TABLE IF NOT EXISTS myplugin_items (id TEXT)"},
		{"trailing semicolons", "CREATE TABLE IF NOT EXISTS myplugin_items (id TEXT);;;"},
		{"extra internal spaces", "CREATE  TABLE   IF  NOT  EXISTS  myplugin_items (id TEXT)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateExternalMigrationSQL("myplugin", tt.sql); err != nil {
				t.Errorf("expected allowed, got error: %v", err)
			}
		})
	}
}

func TestValidateExternalMigrationSQL_RejectedStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{"select", "SELECT * FROM myplugin_items"},
		{"insert", "INSERT INTO myplugin_items VALUES ('a')"},
		{"update", "UPDATE myplugin_items SET name = 'x'"},
		{"delete", "DELETE FROM myplugin_items"},
		{"attach", "ATTACH DATABASE '/tmp/evil.db' AS evil"},
		{"pragma", "PRAGMA table_info(myplugin_items)"},
		{"drop other table", "DROP TABLE IF EXISTS other_items"},
		{"create other table", "CREATE TABLE IF NOT EXISTS other_items (id TEXT)"},
		{"create table wrong prefix", "CREATE TABLE IF NOT EXISTS notmyplugin_items (id TEXT)"},
		{"create table no if not exists", "CREATE TABLE myplugin_items (id TEXT)"},
		{"alter other table", "ALTER TABLE other_items ADD COLUMN x TEXT"},
		{"mixed valid and invalid", "CREATE TABLE IF NOT EXISTS myplugin_a (id TEXT); DROP TABLE other;"},
		{"hidden statement after comment", "CREATE TABLE IF NOT EXISTS myplugin_x (id TEXT); /* harmless */ DROP TABLE ccc_plugin_migrations"},
		{"create index wrong prefix", "CREATE INDEX IF NOT EXISTS other_idx ON myplugin_items (id)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExternalMigrationSQL("myplugin", tt.sql)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestValidateExternalMigrationSQL_EmptySQL(t *testing.T) {
	if err := ValidateExternalMigrationSQL("slug", ""); err != nil {
		t.Errorf("expected empty SQL to be valid, got: %v", err)
	}
	if err := ValidateExternalMigrationSQL("slug", "  ; ;  "); err != nil {
		t.Errorf("expected whitespace-only SQL to be valid, got: %v", err)
	}
}

func TestValidateExternalMigrationSQL_CommentsStripped(t *testing.T) {
	sql := `-- This is a comment
CREATE TABLE IF NOT EXISTS myplugin_items (id TEXT);
/* block comment */
CREATE INDEX IF NOT EXISTS myplugin_idx ON myplugin_items (id)`
	if err := ValidateExternalMigrationSQL("myplugin", sql); err != nil {
		t.Errorf("expected comments to be stripped, got: %v", err)
	}
}

func TestValidateExternalMigrationSQL_SlugWithSpecialChars(t *testing.T) {
	// Slug with regex-special characters should be escaped
	err := ValidateExternalMigrationSQL("my.plugin", "CREATE TABLE IF NOT EXISTS myXplugin_items (id TEXT)")
	if err == nil {
		t.Error("expected error — dot in slug should be escaped, not match arbitrary char")
	}

	// But the literal slug should work
	err = ValidateExternalMigrationSQL("my.plugin", "CREATE TABLE IF NOT EXISTS my.plugin_items (id TEXT)")
	if err != nil {
		t.Errorf("expected literal slug to work, got: %v", err)
	}
}

func TestValidateExternalMigrationSQL_ErrorMessageTruncated(t *testing.T) {
	longSQL := "SELECT " + strings.Repeat("x", 200) + " FROM myplugin_items"
	err := ValidateExternalMigrationSQL("myplugin", longSQL)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(err.Error()) > 200 {
		t.Errorf("error message should be truncated, got length %d", len(err.Error()))
	}
}
