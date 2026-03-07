package plugin

import (
	"database/sql"
	"fmt"
)

// RunMigrations executes all pending migrations for a plugin.
// Migrations are tracked in the ccc_plugin_migrations table.
func RunMigrations(db *sql.DB, slug string, migrations []Migration) error {
	if db == nil || len(migrations) == 0 {
		return nil
	}

	// Ensure the migrations tracking table exists
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS ccc_plugin_migrations (
		plugin_slug TEXT NOT NULL,
		version INTEGER NOT NULL,
		applied_at TEXT NOT NULL,
		PRIMARY KEY (plugin_slug, version)
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Find the highest applied version for this plugin
	var maxVersion int
	err = db.QueryRow(
		`SELECT COALESCE(MAX(version), 0) FROM ccc_plugin_migrations WHERE plugin_slug = ?`,
		slug,
	).Scan(&maxVersion)
	if err != nil {
		return fmt.Errorf("query max version: %w", err)
	}

	// Apply pending migrations
	for _, m := range migrations {
		if m.Version <= maxVersion {
			continue
		}
		if _, err := db.Exec(m.SQL); err != nil {
			return fmt.Errorf("migration v%d for %s: %w", m.Version, slug, err)
		}
		if _, err := db.Exec(
			`INSERT INTO ccc_plugin_migrations (plugin_slug, version, applied_at) VALUES (?, ?, datetime('now'))`,
			slug, m.Version,
		); err != nil {
			return fmt.Errorf("record migration v%d for %s: %w", m.Version, slug, err)
		}
	}

	return nil
}
