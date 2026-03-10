# SPEC: internal/db Package

## Purpose

Provides all data persistence for the Claude Command Center (CCC). Manages SQLite database operations (schema, CRUD, migration from legacy JSON files) and shared domain types used across the entire application.

## Interface

### Inputs
- **Database path**: Passed to `OpenDB(dbPath string)` -- creates parent directories as needed
- **JSON file paths**: Passed to `MigrateFromJSON(db, ccPath, bookmarksPath, pathsPath)` for legacy migration
- **Domain objects**: `Todo`, `Thread`, `Session`, `PendingAction`, `CalendarData` passed to DB write functions

### Outputs
- `*sql.DB` connection (WAL mode, 5s busy timeout, NORMAL synchronous, max 1 conn)
- `*CommandCenter` aggregate loaded from DB
- Individual query results (bookmarks, paths, etc.)

### Dependencies
- `modernc.org/sqlite` (pure-Go SQLite driver, no CGO)
- Standard library only otherwise

## File Organization

| File | Responsibility |
|------|---------------|
| `schema.go` | `OpenDB`, `migrateSchema` (table DDL), `FormatTime`, `ParseTime` helpers |
| `types.go` | All exported domain types, ID generation, time helpers, in-memory `CommandCenter` mutation methods, file-based path CRUD |
| `read.go` | `LoadCommandCenterFromDB` and all `dbLoad*` query functions, `DBLoadBookmarks`, `DBLoadPaths`, `DBIsEmpty` |
| `write.go` | All `DB*` write functions for todos, threads, calendar, suggestions, pending actions, bookmarks, paths, meta; `DBSaveRefreshResult` bulk write |
| `sessions.go` | Session types, file-based session parsing (`ParseSessionFile`, `LoadWinddownSessions`, `LoadBookmarks`, `LoadAllSessions`, `RemoveBookmark`) -- used by sessions plugin only |
| `migrate.go` | `MigrateFromJSON` -- one-time import from legacy JSON/text files into SQLite |

## Types

All types are exported for use by other packages:

- `CommandCenter` -- top-level aggregate: calendar, todos, threads, suggestions, pending actions, warnings, generated_at
- `CalendarData` -- today/tomorrow event lists
- `CalendarEvent` -- title, start/end times, all-day, declined, calendar_id
- `CalendarConflict` -- overlap between two events
- `Todo` -- task with status, source, due date, effort, project_dir, session_id, etc.
- `Thread` -- PR/issue/conversation tracker with pause/resume/close lifecycle
- `Suggestions` -- AI-generated focus and ranked todo ordering with per-todo reasons
- `PendingAction` -- queued actions (e.g., calendar bookings)
- `Warning` -- system warnings (source, message, timestamp)
- `Session` -- resumable Claude Code session (bookmark or winddown) -- used by sessions plugin only
- `SessionType` -- enum: `SessionWinddown`, `SessionBookmark`
- `Bookmark` -- JSON serialization format for bookmarks file

## Behavior

### Database Lifecycle
1. `OpenDB(dbPath)` creates directories, opens SQLite, sets WAL + busy_timeout + synchronous=NORMAL, max 1 connection, runs `migrateSchema`
2. Schema creates 8 tables: `cc_todos`, `cc_threads`, `cc_calendar_cache`, `cc_suggestions`, `cc_pending_actions`, `cc_meta`, `cc_bookmarks`, `cc_learned_paths`
3. Unique indexes on `source_ref` for todos and threads (WHERE NOT NULL/empty)
4. Post-DDL migration adds `calendar_id` column if missing (ALTER TABLE, errors ignored)

### Todo Operations
- `DBInsertTodo` -- auto-assigns sort_order = max+1
- `DBCompleteTodo` -- sets status=completed, completed_at=now
- `DBDismissTodo` -- sets status=dismissed
- `DBRestoreTodo` -- restores to given status and completed_at (for undo)
- `DBDeferTodo` -- sets sort_order to max+1 (moves to bottom)
- `DBPromoteTodo` -- sets sort_order to min-1 (moves to top)
- `DBUpdateTodo` -- updates all fields except sort_order

### Thread Operations
- `DBInsertThread` -- creates with status=active
- `DBPauseThread` -- sets status=paused, paused_at=now
- `DBStartThread` -- sets status=active, clears paused_at
- `DBCloseThread` -- sets status=completed, completed_at=now

### Calendar & Suggestions
- `DBReplaceCalendar` -- transactional replace of all cached events
- `DBSaveFocus` -- upserts focus text, preserving ranked_todo_ids/reasons
- `DBSaveSuggestions` -- replaces the full suggestions row

### Bulk Refresh
- `DBSaveRefreshResult` -- atomically replaces all refresh-managed data (todos, threads, calendar, suggestions, pending actions, generated_at) in a single transaction. Used by `ccc-refresh`.

### Bookmarks & Paths (DB)
- `DBLoadBookmarks`, `DBInsertBookmark`, `DBRemoveBookmark`
- `DBLoadPaths`, `DBAddPath` (INSERT OR IGNORE), `DBRemovePath`

### Meta & Pending Actions
- `DBSetMeta` -- upserts a key-value pair in `cc_meta`
- `DBClearPendingActions` -- removes all pending actions

### File-Based Session Functions (used by sessions plugin only)
- `ParseSessionFile` -- parses YAML-ish frontmatter from markdown winddown files
- `LoadWinddownSessions` -- reads all `.md` files from sessions directory
- `LoadBookmarks` -- reads JSON array from bookmarks file
- `LoadAllSessions` -- merges winddowns + bookmarks, sorted by created desc
- `RemoveBookmark` -- removes a bookmark from the JSON file

### File-Based Path Functions
- `LoadPaths`, `SavePaths` -- newline-delimited text file
- `AddPath`, `RemovePath` -- in-memory list manipulation

### Migration (Legacy)
- `MigrateFromJSON(db, ccPath, bookmarksPath, pathsPath)` -- one-time import from legacy JSON files
- Only runs if DB is empty (no todos)
- Idempotent: uses INSERT OR IGNORE
- Preserves sort_order from array position
- Migrates: command-center.json, bookmarks.json, learned-paths.txt

### Dismissed-Todo Filtering
- Dismissed-todo filtering (preventing refresh from re-adding user-dismissed items) lives in the merge layer (`internal/refresh/merge.go`), not in the db package

### In-Memory Mutations (on CommandCenter)
- `CompleteTodo`, `RestoreTodo`, `AddTodo`, `RemoveTodo`, `DeferTodo`, `PromoteTodo`
- `PauseThread`, `StartThread`, `CloseThread`, `AddThread`
- `ActiveTodos`, `CompletedTodos`, `ActiveThreads`, `PausedThreads` -- filtered/sorted views
- `AddPendingBooking` -- appends a booking action
- `FindConflicts` -- detects overlapping calendar events (skips declined, all-day, ended)

### Helpers
- `FormatTime(t)` -- UTC RFC3339
- `ParseTime(s)` -- parses RFC3339 or bare datetime, returns local time
- `GenID()` -- 8-char random hex
- `RelativeTime(t)` -- "5m ago", "2h ago", "3d ago"
- `DueUrgency(due)` -- "none", "overdue", "soon", "later"
- `FormatDueLabel(due)` -- "overdue", "due today", "due tomorrow", "due Mon"
- `DBIsEmpty(db)` -- checks if any todos exist

## Test Cases

- DB open/close and table creation
- Todo CRUD round-trip (insert, complete, dismiss, defer, promote, restore)
- Thread lifecycle (insert, pause, start, close)
- Path CRUD (add, duplicate ignore, remove)
- Bookmark CRUD (insert, load, remove)
- JSON migration (full data, idempotent re-run, empty DB guard)
- DBIsEmpty on fresh vs populated DB
- DBSaveRefreshResult round-trip
- In-memory mutations (complete, remove, add, defer, promote, pause, start, close, add thread)
- ActiveTodos/CompletedTodos/ActiveThreads/PausedThreads filtering
- DueUrgency for overdue/soon/later/none/bad-date
- RelativeTime for minutes/hours/days
- FindConflicts with overlaps and no overlaps
- Session file parsing (valid, missing fields, no frontmatter)
- Winddown sessions (multiple files, empty dir, missing dir)
- Bookmarks from JSON file (valid, missing file)
- LoadAllSessions merged and sorted
- RemoveBookmark from JSON file
