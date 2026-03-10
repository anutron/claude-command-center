# SPEC: Plugin Migrations

## Purpose

Provides a versioned migration system for plugin-specific SQLite tables. Each plugin can define its own migrations, which are tracked separately in a shared `ccc_plugin_migrations` table.

## Interface

```go
// Migration is defined in plugin.go
type Migration struct {
    Version int
    SQL     string
}

func RunMigrations(db *sql.DB, slug string, migrations []Migration) error
```

## Behavior

1. If `db` is nil or `migrations` is empty, return nil (no-op)
2. Create the tracking table if it does not exist:
   ```sql
   CREATE TABLE IF NOT EXISTS ccc_plugin_migrations (
       plugin_slug TEXT NOT NULL,
       version INTEGER NOT NULL,
       applied_at TEXT NOT NULL,
       PRIMARY KEY (plugin_slug, version)
   )
   ```
3. Query the highest applied version for the given plugin slug
4. For each migration with `Version > maxVersion`, in order:
   - Begin a transaction
   - Execute the migration SQL
   - Record the migration in `ccc_plugin_migrations` with `datetime('now')`
   - Commit the transaction
5. If any step fails, the transaction is rolled back and an error is returned

## Key Design Decisions

- **Per-plugin versioning**: Each plugin's migrations are tracked independently by slug. Version numbers are scoped to the plugin.
- **Transactional**: Each migration runs in its own transaction. If a migration fails, only that migration is rolled back; previously applied migrations remain.
- **Forward-only**: No rollback/down migration support. Migrations only move forward.
- **Called during Init**: Plugins call `RunMigrations` during their `Init(ctx)` method, before any queries.

## Test Cases

- No-op when db is nil
- No-op when migrations slice is empty
- Creates tracking table on first run
- Applies pending migrations in version order
- Skips already-applied migrations
- Records applied version in tracking table
- Rolls back on SQL error (partial migration)
- Multiple plugins can have independent version tracks
- Idempotent re-run applies nothing new
