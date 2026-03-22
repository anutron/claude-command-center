# Database & Data Layer Audit

## Summary

The database layer is well-structured with proper parameterized queries and good use of transactions for bulk operations. The main concerns are: missing indexes on frequently queried columns, the plugin migration system lacking transactional atomicity, no prepared statements in bulk insert loops, and a stale query in the doctor check that references a non-existent table.

## Schema Review

**Files:** `/Users/aaron/Personal/claude-command-center/internal/db/db.go` (lines 40-127), `/Users/aaron/Personal/claude-command-center/internal/plugin/migrations.go`

### Missing Indexes

1. **`cc_todos.status`** — Frequently filtered by status (`ActiveTodos()`, `CompletedTodos()`), but no index exists. The `sort_order` column is used in `ORDER BY` but also unindexed.

2. **`cc_threads.status`** — Same issue; `ActiveThreads()` and `PausedThreads()` filter by status.

3. **`cc_calendar_cache.day`** — Calendar loading at line 265 reads all rows and partitions by `day` in Go code. An index on `day` would help if a `WHERE` clause were added, but currently all rows are loaded regardless.

4. **`cc_pending_actions.todo_id`** — No index on `todo_id`, though the table is likely small.

### Schema Design Notes

- **Singleton pattern for `cc_suggestions`** (line 92, `CHECK (id = 1)`) is a reasonable approach for a single-row config table.

- **No foreign keys** — `cc_pending_actions.todo_id` references `cc_todos.id` conceptually but has no FK constraint. `PRAGMA foreign_keys` is never enabled. This is acceptable given the delete-and-reinsert pattern used by `DBSaveRefreshResult`, but orphaned pending actions could accumulate if a todo is deleted without clearing its actions.

- **Text-based timestamps** — All timestamps are stored as `TEXT` in RFC3339 format. This works but prevents SQLite date functions from being used efficiently in queries. Not a problem currently since no date-range queries exist.

- **`cc_calendar_cache` has no unique constraint** — Events are identified by an autoincrement `id` rather than a natural key (like title+start_time). This is fine because the table is always bulk-replaced, but it means duplicate events could theoretically be inserted without the delete-first pattern.

### Schema Migration Approach

The core schema uses `CREATE TABLE IF NOT EXISTS` (line 40) with a manual `ALTER TABLE` fallback at line 133 for the `calendar_id` column. The `ALTER TABLE` error is silently discarded (`_, _ = db.Exec(...)`), which is the correct approach for idempotent column additions but makes debugging harder if the ALTER fails for a different reason.

## SQL Injection Risk

**Risk level: None detected.**

Every SQL query in the codebase uses parameterized placeholders (`?`). A grep for `fmt.Sprintf` combined with SQL keywords returned zero results. All user-supplied values (todo titles, IDs, paths, etc.) flow through `db.Exec` or `db.Query` as bind parameters.

**Files checked:**
- `/Users/aaron/Personal/claude-command-center/internal/db/db.go` — all queries parameterized
- `/Users/aaron/Personal/claude-command-center/internal/db/migrate.go` — all queries parameterized
- `/Users/aaron/Personal/claude-command-center/internal/plugin/migrations.go` — parameterized for tracking; plugin SQL is raw but comes from code, not user input
- `/Users/aaron/Personal/claude-command-center/internal/builtin/sessions/sessions.go` — uses `db` package functions, no direct SQL
- `/Users/aaron/Personal/claude-command-center/internal/config/doctor.go` — one hardcoded query at line 146

## Query Patterns

### N+1 Queries

No N+1 patterns found. `LoadCommandCenterFromDB` (line 142) makes 5 sequential queries (todos, threads, calendar, suggestions, pending actions), which is reasonable — each loads a full table. These could theoretically be combined but the data structures are distinct enough that separate queries are clearer.

### Unnecessary Queries

1. **`LoadCommandCenterFromDB` loads all todos/threads regardless of status** — Active filtering happens in Go (`ActiveTodos()`, `CompletedTodos()` in `types.go`). For small datasets this is fine, but if the tables grow large, pushing status filters into SQL would be more efficient.

2. **`DBSaveRefreshResult` uses per-row `tx.Exec` in a loop** (lines 631-651, 657-679, 686-699) — No prepared statements are used for the bulk insert of todos, threads, and calendar events within the transaction. The `DBReplaceCalendar` function (line 437) correctly uses `tx.Prepare` with a prepared statement for its loop, but `DBSaveRefreshResult` does not. This means the SQL is re-parsed for each row insertion.

3. **`checkDataFreshness` references wrong table** — `/Users/aaron/Personal/claude-command-center/internal/config/doctor.go` line 146 queries `SELECT generated_at FROM command_center LIMIT 1`, but no `command_center` table exists in the schema. The correct table is `cc_meta` with key `'generated_at'`. This query will always fail silently.

### Subquery in Hot Path

`DBDeferTodo` (line 381) and `DBPromoteTodo` (line 387) use a correlated subquery (`SELECT COALESCE(MAX/MIN(sort_order)...`) inside the UPDATE. This is fine for single-row updates but is worth noting.

## Transaction Handling

### Good

- **`DBSaveRefreshResult`** (line 618) — Correctly wraps the entire bulk replace in a single transaction with `defer tx.Rollback()`. This ensures atomicity: either all data is replaced or none is.

- **`DBReplaceCalendar`** (line 437) — Properly uses a transaction for delete-then-insert with a prepared statement.

- **`migrateCommandCenter`** in `migrate.go` (line 59) — JSON-to-SQLite migration is transactional.

### Issues

1. **Individual todo/thread mutations are not transacted** — `DBCompleteTodo`, `DBDismissTodo`, `DBDeferTodo`, etc. (lines 354-391) each issue a single `db.Exec`. These are inherently atomic as single statements, which is fine. However, the TUI could theoretically issue conflicting writes if the user acts while `ai-cron` is running (both write to the same tables). The `busy_timeout=5000` PRAGMA mitigates this.

2. **Plugin migrations are not transacted** — `/Users/aaron/Personal/claude-command-center/internal/plugin/migrations.go` line 41: each migration SQL is executed individually, then its version is recorded. If the SQL succeeds but the version INSERT fails (e.g., disk full), the migration will be re-applied on next startup. If the migration SQL is not idempotent (e.g., `ALTER TABLE ADD COLUMN` without `IF NOT EXISTS`), this could cause errors. Each migration+record pair should be wrapped in a transaction.

3. **`migrateBookmarks` in `migrate.go` (line 158) is not transacted** — Bookmarks are inserted one-by-one outside a transaction. If the process crashes mid-migration, some bookmarks will exist and others won't. Uses `INSERT OR IGNORE` so it is at least idempotent.

4. **`migrateLearnedPaths` in `migrate.go` (line 183) is not transacted** — Same issue as bookmarks.

## Migration System

**Files:** `/Users/aaron/Personal/claude-command-center/internal/plugin/migrations.go`, `/Users/aaron/Personal/claude-command-center/internal/db/db.go` (lines 39-136)

### Core Schema

The core schema migration is a single `CREATE TABLE IF NOT EXISTS` block. This is simple and works well for the initial schema, but has no versioning — there is no way to run incremental ALTER statements in a managed way. The `calendar_id` column addition at line 133 is a manual workaround for this limitation.

**Recommendation:** Consider using the plugin migration system (`RunMigrations`) for the core schema as well, so future schema changes are tracked and versioned.

### Plugin Migration System

- **Version tracking** is per-plugin via `ccc_plugin_migrations` table — good design.
- **Versions are sequential integers** — migrations are skipped if version <= max applied. This works but requires that plugin authors never reorder or skip version numbers.
- **No downgrade support** — typical for this kind of system, acceptable.
- **No transaction wrapping** (see Transaction Handling above) — the migration SQL and version record are separate statements.
- **Migration SQL is free-form** — any SQL string. This is flexible but means a broken migration could leave the DB in an inconsistent state.

## Resource Management

### Good

- **All `rows` are closed with `defer rows.Close()`** — Checked in `dbLoadTodos` (line 190), `dbLoadThreads` (line 230), `dbLoadCalendar` (line 270), `dbLoadPendingActions` (line 322), `DBLoadBookmarks` (line 539), `DBLoadPaths` (line 583).

- **All `rows.Err()` are checked** after iteration — Every query function returns `rows.Err()` at the end.

- **Prepared statement in `DBReplaceCalendar`** is closed with `defer stmt.Close()` (line 454).

- **`tx.Rollback()` is deferred** in all transaction-using functions (lines 442, 623).

- **`OpenDB` closes the DB handle on error** (lines 25, 29, 34).

### Issues

1. **No `sql.DB.Close()` in main application lifecycle** — Not visible in the DB package itself, but worth verifying that the TUI and refresh binaries close the DB connection on shutdown. The `sql.DB` handle is passed into plugins via `plugin.Context.DB` but plugin `Shutdown()` methods don't close it (which is correct — the host should close it).

2. **No connection pool configuration** — `sql.DB` defaults are used (no `SetMaxOpenConns`, `SetMaxIdleConns`). For SQLite with WAL mode, `SetMaxOpenConns(1)` is a common best practice to prevent "database is locked" errors, especially when both the TUI and `ai-cron` may access the same DB file. The `busy_timeout=5000` helps but does not eliminate the issue.

## WAL Mode and Concurrency

**Configuration** (`db.go` lines 24-31):
- `PRAGMA journal_mode=WAL` — Correctly enabled for concurrent read/write.
- `PRAGMA busy_timeout=5000` — 5-second retry on lock contention. Good.

### Concerns

1. **Two processes share the same DB** — The TUI (`ccc`) and the refresh binary (`ai-cron`) both call `OpenDB` on the same path. WAL mode allows concurrent readers with one writer, and `busy_timeout` handles contention. However, `DBSaveRefreshResult` holds a write lock for the entire bulk replace (which could take noticeable time if there are many todos/threads). During this window, TUI writes (complete/dismiss/defer) will block up to 5 seconds.

2. **No `PRAGMA synchronous` setting** — Defaults to `FULL` in WAL mode, which is safe but slower. `PRAGMA synchronous=NORMAL` is generally recommended for WAL mode as it provides a good safety/performance tradeoff.

3. **File-based locking for refresh** — `/Users/aaron/Personal/claude-command-center/internal/refresh/lock.go` uses PID-file locking to prevent concurrent refresh runs. This is a TOCTOU race (check-then-write at lines 24-36) — two processes could both read the stale lock and both proceed. Using `flock()` or SQLite advisory locks would be more robust.

## Recommendations

### High Priority

1. **Fix broken doctor query** — `/Users/aaron/Personal/claude-command-center/internal/config/doctor.go` line 146: change `SELECT generated_at FROM command_center LIMIT 1` to `SELECT value FROM cc_meta WHERE key = 'generated_at'`.

2. **Wrap plugin migrations in transactions** — `/Users/aaron/Personal/claude-command-center/internal/plugin/migrations.go`: wrap each migration SQL + version record in a `tx.Begin()`/`tx.Commit()` pair.

3. **Use prepared statements in `DBSaveRefreshResult`** — `/Users/aaron/Personal/claude-command-center/internal/db/db.go` lines 631-699: use `tx.Prepare` for the todo, thread, and calendar insert statements inside the loop, similar to how `DBReplaceCalendar` does it.

### Medium Priority

4. **Add index on `cc_todos(status, sort_order)`** — Speeds up the common query pattern of loading active todos in sorted order.

5. **Add index on `cc_threads(status, created_at)`** — Speeds up active/paused thread queries.

6. **Set `MaxOpenConns(1)` on `sql.DB`** — Prevents potential concurrent write issues with SQLite.

7. **Add `PRAGMA synchronous=NORMAL`** — Safe performance improvement for WAL mode.

8. **Use proper file locking for refresh** — Replace PID-file check in `lock.go` with `syscall.Flock()` to eliminate TOCTOU race.

### Low Priority

9. **Version the core schema** — Use the `RunMigrations` system for core tables so future ALTER statements are managed consistently.

10. **Wrap bookmark/path migrations in transactions** — `migrate.go` lines 158-196: wrap the loop in a transaction for atomicity.

11. **Consider `SetMaxIdleConns(1)`** — Reduces resource usage for a single-user SQLite application.
