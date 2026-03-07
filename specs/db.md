# SPEC: internal/db Package

## Purpose

Provides all data persistence for the Claude Command Center (CCC). Manages SQLite database operations (schema, CRUD, migration from legacy JSON files) and shared domain types used across the entire application.

## Interface

### Inputs
- **Database path**: Passed to `OpenDB(dbPath string)` -- creates parent directories as needed
- **JSON file paths**: Passed to `MigrateFromJSON(db, ccPath, bookmarksPath, pathsPath)` for legacy migration
- **Domain objects**: `Todo`, `Thread`, `Session`, `PendingAction`, `CalendarData` passed to DB write functions

### Outputs
- `*sql.DB` connection (WAL mode, 5s busy timeout)
- `*CommandCenter` aggregate loaded from DB or JSON
- Individual query results (bookmarks, paths, etc.)

### Dependencies
- `modernc.org/sqlite` (pure-Go SQLite driver, no CGO)
- Standard library only otherwise

## Types

All types are exported for use by other packages:

- `CommandCenter` -- top-level aggregate: calendar, todos, threads, suggestions, pending actions, warnings
- `CalendarData` -- today/tomorrow event lists
- `CalendarEvent` -- title, start/end times, all-day, declined, calendar_id
- `CalendarConflict` -- overlap between two events
- `Todo` -- task with status, source, due date, effort, etc.
- `Thread` -- PR/issue/conversation tracker with pause/resume/close lifecycle
- `Suggestions` -- AI-generated focus and ranked todo ordering
- `PendingAction` -- queued actions (e.g., calendar bookings)
- `Warning` -- system warnings
- `Session` -- resumable Claude Code session (bookmark or winddown)
- `SessionType` -- enum: `SessionWinddown`, `SessionBookmark`
- `Bookmark` -- JSON serialization format for bookmarks file

## Behavior

### Database Lifecycle
1. `OpenDB(dbPath)` creates directories, opens SQLite, sets WAL + busy_timeout, runs `migrateSchema`
2. Schema creates 8 tables: `cc_todos`, `cc_threads`, `cc_calendar_cache`, `cc_suggestions`, `cc_pending_actions`, `cc_meta`, `cc_bookmarks`, `cc_learned_paths`
3. Unique indexes on `source_ref` for todos and threads (WHERE NOT NULL/empty)

### Todo Operations
- `DBInsertTodo` -- auto-assigns sort_order = max+1
- `DBCompleteTodo` -- sets status=completed, completed_at=now
- `DBDismissTodo` -- sets status=dismissed
- `DBRestoreTodo` -- restores to given status (for undo)
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

### Bookmarks & Paths (DB)
- `DBLoadBookmarks`, `DBInsertBookmark`, `DBRemoveBookmark`
- `DBLoadPaths`, `DBAddPath` (INSERT OR IGNORE), `DBRemovePath`

### JSON File Operations (Legacy)
- `LoadCommandCenter(path)` -- reads JSON, runs `EnforceDismissed`
- `SaveCommandCenter(path, cc)` -- atomic write via temp file + rename
- `MutateSave(path, fallback, fn)` -- reload-modify-save pattern
- `LoadPaths`, `SavePaths`, `AddPath`, `RemovePath` -- newline-delimited file
- `LoadBookmarks`, `RemoveBookmark` -- JSON array file
- `LoadWinddownSessions`, `LoadAllSessions` -- markdown frontmatter files
- `ParseSessionFile` -- parses YAML-ish frontmatter from markdown

### Migration
- `MigrateFromJSON(db, ccPath, bookmarksPath, pathsPath)` -- one-time import from legacy files
- Only runs if DB is empty (no todos)
- Idempotent: uses INSERT OR IGNORE
- Preserves sort_order from array position

### EnforceDismissed
- Auto-dismisses any active todo whose source_ref matches a dismissed todo's source_ref
- Runs on every `LoadCommandCenter` call
- Prevents refresh from re-adding user-dismissed items

### In-Memory Mutations (on CommandCenter)
- `CompleteTodo`, `RestoreTodo`, `AddTodo`, `RemoveTodo`, `DeferTodo`, `PromoteTodo`
- `PauseThread`, `StartThread`, `CloseThread`, `AddThread`
- `ActiveTodos`, `CompletedTodos`, `ActiveThreads`, `PausedThreads` -- filtered/sorted views
- `AddPendingBooking` -- appends a booking action
- `FindConflicts` -- detects overlapping calendar events

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
- Todo CRUD round-trip (insert, complete, dismiss, defer, promote)
- Thread lifecycle (insert, pause, start, close)
- Path CRUD (add, duplicate ignore, remove)
- Bookmark CRUD (insert, load, remove)
- JSON migration (full data, idempotent re-run)
- DBIsEmpty on fresh vs populated DB
- CommandCenter JSON load/save round-trip
- Atomic write (no .tmp file left behind)
- In-memory mutations (complete, remove, add, defer, pause, start, close, add thread)
- ActiveTodos/CompletedTodos/ActiveThreads/PausedThreads filtering
- DueUrgency for overdue/soon/later/none/bad-date
- RelativeTime for minutes/hours/days
- FindConflicts with overlaps and no overlaps
- EnforceDismissed (matching refs, no refs, via LoadCommandCenter)
- Session file parsing (valid, missing fields, no frontmatter)
- Winddown sessions (multiple files, empty dir, missing dir)
- Bookmarks from JSON file (valid, missing file)
- LoadAllSessions merged and sorted
- RemoveBookmark from JSON file
