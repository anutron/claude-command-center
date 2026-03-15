package plugin

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// allowedDDLPatterns defines the SQL statement patterns allowed for external
// plugin migrations. Each pattern is anchored and case-insensitive. The
// placeholder %s is replaced with the escaped plugin slug.
var allowedDDLPatterns = []string{
	`CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+%s_`,
	`CREATE\s+INDEX\s+IF\s+NOT\s+EXISTS\s+%s_`,
	`CREATE\s+UNIQUE\s+INDEX\s+IF\s+NOT\s+EXISTS\s+%s_`,
	`ALTER\s+TABLE\s+%s_`,
	`DROP\s+TABLE\s+IF\s+EXISTS\s+%s_`,
	`DROP\s+INDEX\s+IF\s+EXISTS\s+%s_`,
}

// stripSQLComments removes SQL line comments (-- ...) and block comments (/* ... */).
func stripSQLComments(sql string) string {
	// Remove block comments
	re := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	sql = re.ReplaceAllString(sql, " ")
	// Remove line comments
	re = regexp.MustCompile(`--[^\n]*`)
	sql = re.ReplaceAllString(sql, " ")
	return sql
}

// ValidateExternalMigrationSQL checks that every statement in the migration SQL
// is an allowed DDL statement namespaced to the given plugin slug. Returns an
// error describing the first disallowed statement, or nil if all are valid.
func ValidateExternalMigrationSQL(slug, sqlText string) error {
	cleaned := stripSQLComments(sqlText)

	// Split on semicolons and validate each statement
	statements := strings.Split(cleaned, ";")
	escapedSlug := regexp.QuoteMeta(slug)

	// Build compiled patterns for this slug
	patterns := make([]*regexp.Regexp, len(allowedDDLPatterns))
	for i, pat := range allowedDDLPatterns {
		full := fmt.Sprintf(`^\s*`+pat, escapedSlug)
		patterns[i] = regexp.MustCompile("(?i)" + full)
	}

	for _, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}

		// Normalize internal whitespace for matching
		normalized := regexp.MustCompile(`\s+`).ReplaceAllString(trimmed, " ")

		matched := false
		for _, p := range patterns {
			if p.MatchString(normalized) {
				matched = true
				break
			}
		}
		if !matched {
			// Truncate for readability
			preview := normalized
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			return fmt.Errorf("disallowed SQL in external plugin migration: %q", preview)
		}
	}
	return nil
}

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
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration v%d of %s: %w", m.Version, slug, err)
		}
		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration v%d for %s: %w", m.Version, slug, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO ccc_plugin_migrations (plugin_slug, version, applied_at) VALUES (?, ?, datetime('now'))`,
			slug, m.Version,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration v%d for %s: %w", m.Version, slug, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration v%d for %s: %w", m.Version, slug, err)
		}
	}

	return nil
}
