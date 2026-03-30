# SPEC: internal/db Package

## Purpose

Provides all data persistence for the Claude Command Center (CCC). Manages SQLite database operations (schema, CRUD, migration from legacy JSON files) and shared domain types used across the entire application.

## Interface

### Inputs
- **Database path**: Passed to `OpenDB(dbPath string)` -- creates parent directories as needed
- **JSON file paths**: Passed to `MigrateFromJSON(db, ccPath, bookmarksPath, pathsPath)` for legacy migration
- **Domain objects**: `Todo`, `PullRequest`, `TodoMerge`, `Session`, `PendingAction`, `CalendarData` passed to DB write functions

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
| `write.go` | All `DB*` write functions for todos, calendar, suggestions, pending actions, bookmarks, paths, meta; `DBSaveRefreshResult` bulk write |
| `sessions.go` | Session types, file-based session parsing (`ParseSessionFile`, `LoadWinddownSessions`, `LoadBookmarks`, `LoadAllSessions`, `RemoveBookmark`) -- used by sessions plugin only |
| `migrate.go` | `MigrateFromJSON` -- one-time import from legacy JSON/text files into SQLite |
| `skill_discover.go` | `SkillInfo`, `SkillCache` types, `DiscoverSkills`, `DiscoverGlobalSkills`, disk cache read/write, `GetProjectSkills`, `GetGlobalSkills` |
| `routing_rules.go` | `RoutingRule` type, file-based routing rules CRUD (`LoadRoutingRules`, `SaveRoutingRules`, `AddRoutingRule`) |
| `path_describe.go` | `AutoDescribePath` — heuristic project description from common project files (go.mod, package.json, etc.) |

## Types

All types are exported for use by other packages:

- `CommandCenter` -- top-level aggregate: calendar, todos, pull requests, merges, suggestions, pending actions, warnings, generated_at
- `CalendarData` -- today/tomorrow event lists
- `CalendarEvent` -- title, start/end times, all-day, declined, calendar_id
- `CalendarConflict` -- overlap between two events
- `Todo` -- task with status, source, due date, effort, project_dir, session_id, source_context, source_context_at, etc.
- `Suggestions` -- AI-generated focus and ranked todo ordering with per-todo reasons
- `PendingAction` -- queued actions (e.g., calendar bookings)
- `Warning` -- system warnings (source, message, timestamp)
- `Session` -- resumable Claude Code session (bookmark or winddown) -- used by sessions plugin only
- `SessionType` -- enum: `SessionWinddown`, `SessionBookmark`
- `Bookmark` -- JSON serialization format for bookmarks file
- `PathEntry` -- full metadata for a learned path row: path, description, added_at, sort_order
- `SkillInfo` -- name + description parsed from a SKILL.md frontmatter
- `SkillCache` -- list of `SkillInfo` with a `ScannedAt` timestamp for TTL-based disk caching
- `RoutingRule` -- `use_for` and `not_for` string lists, `PromptHint` string describing which tasks a path handles

## Behavior

### Database Lifecycle
1. `OpenDB(dbPath)` creates directories, opens SQLite, sets WAL + busy_timeout + synchronous=NORMAL, max 1 connection, runs `migrateSchema`
2. Schema creates 16 tables: `cc_todos`, `cc_calendar_cache`, `cc_suggestions`, `cc_pending_actions`, `cc_meta`, `cc_bookmarks`, `cc_learned_paths`, `cc_source_sync`, `cc_todo_merges`, `cc_pull_requests`, `cc_sessions`, `cc_automation_runs`, `cc_agent_costs`, `cc_budget_state`, `cc_archived_sessions`, `cc_ignored_repos`
3. Unique indexes on `source_ref` for todos and pull requests (WHERE NOT NULL/empty)
4. Post-DDL migrations add columns if missing (ALTER TABLE, errors ignored): `calendar_id` on events, `session_id` on todos, `sort_order` on learned paths, `description` (TEXT, default '') on learned paths, worktree columns on bookmarks, `source_context` and `source_context_at` on todos
5. Post-DDL migration fixes duplicate `sort_order` values on `cc_learned_paths` using `ROW_NUMBER()` window function

### Todo Operations
- `DBInsertTodo` -- auto-assigns sort_order = max+1
- `DBCompleteTodo` -- sets status=completed, completed_at=now
- `DBDismissTodo` -- sets status=dismissed
- `DBRestoreTodo` -- restores to given status and completed_at (for undo)
- `DBDeferTodo` -- sets sort_order to max+1 (moves to bottom)
- `DBPromoteTodo` -- sets sort_order to min-1 (moves to top)
- `DBUpdateTodo` -- updates all fields except sort_order
- `DBUpdateTodoSourceContext` -- focused update for source_context and source_context_at columns

### Calendar & Suggestions
- **Calendar day-clamping on load**: `dbLoadCalendar` clamps multi-day event times to their day boundaries. Events whose start is before the day start are clamped to midnight; events whose end extends past the day end are clamped to end-of-day. Events are then re-sorted by effective start time. This ensures multi-day events sort and display correctly (e.g., a 3-day conference shows as starting at midnight today, not at its original start time days ago).
- `DBReplaceCalendar` -- transactional replace of all cached events
- `DBSaveFocus` -- upserts focus text, preserving ranked_todo_ids/reasons
- `DBSaveSuggestions` -- replaces the full suggestions row

### Pull Requests

- **Schema**: `cc_pull_requests` table gains 8 new columns: `state` (TEXT, default `"open"`), `ignored` (BOOLEAN, NOT NULL, default 0), `head_sha` (TEXT), `agent_session_id` (TEXT), `agent_status` (TEXT), `agent_category` (TEXT), `agent_head_sha` (TEXT), `agent_summary` (TEXT)
- **New table**: `cc_ignored_repos (repo TEXT PRIMARY KEY)` — stores repos whose PRs should be globally hidden
- **Save strategy**: `DBSavePullRequests` uses merge-based upsert (was delete-all/re-insert):
  1. Upsert each fresh PR by ID (`owner/repo#number`) — updates all GitHub-sourced fields while **preserving** agent columns (`agent_session_id`, `agent_status`, `agent_category`, `agent_head_sha`, `agent_summary`)
  2. Archive PRs not in the fresh batch — set `state = "archived"` (do not delete)
  3. Reactivate archived PRs that reappear — set `state = "open"`
- **Load**: `DBLoadPullRequests` filters to `state != "archived"` AND `ignored = 0` AND `repo NOT IN (SELECT repo FROM cc_ignored_repos)`, ordered by `last_activity_at DESC`
- `DBUpdatePRAgentStatus(db, prID, agentStatus, agentSessionID, agentCategory, agentHeadSHA, agentSummary)` — focused update for agent tracking columns on a single PR
- `DBSetPRIgnored(db, prID, ignored bool)` — sets the `ignored` flag on a single PR
- `DBAddIgnoredRepo(db, repo string)` — inserts a repo into `cc_ignored_repos` (INSERT OR IGNORE)
- `DBRemoveIgnoredRepo(db, repo string)` — removes a repo from `cc_ignored_repos`
- `DBLoadIgnoredRepos(db)` — returns all repos in `cc_ignored_repos` as `[]string`
- `DBLoadIgnoredPRs(db)` — returns all non-archived PRs with `ignored = 1`

### Bulk Refresh
- `DBSaveRefreshResult` -- atomically replaces all refresh-managed data (todos, pull requests, calendar, suggestions, pending actions, generated_at) in a single transaction. Used by `ai-cron`.

### Bookmarks & Paths (DB)
- `DBLoadBookmarks`, `DBInsertBookmark`, `DBRemoveBookmark`
- `DBLoadPaths` (returns `[]string`), `DBLoadPathsFull` (returns `[]PathEntry` with description, added_at, sort_order), `DBAddPath` (INSERT OR IGNORE), `DBRemovePath`, `DBUpdatePathDescription`

### Path Descriptions
- `AutoDescribePath(dir)` -- heuristic description from project files (go.mod, package.json, Cargo.toml, pyproject.toml, setup.py, Gemfile, pom.xml, build.gradle, Package.swift)
- Returns language/framework label + module name if available (e.g., "Go project (github.com/user/repo)")
- Returns empty string if no recognizable project files found
- `DBUpdatePathDescription` -- persists a description for a learned path; fails if path not in DB

### Skills Discovery
- Skills are defined by `SKILL.md` files with YAML frontmatter containing `name` and `description`
- `DiscoverSkills(dir)` -- scans `<dir>/.claude/skills/*/SKILL.md`
- `DiscoverGlobalSkills()` -- scans `~/.claude/skills/*/SKILL.md`
- Frontmatter must start with `---\n` and end with `\n---`; `name` field is required, files without it are skipped
- **Disk cache**: skills are cached per-project as JSON files in `~/.config/ccc/cache/skills/` with 1-hour TTL
  - Cache key is SHA-256 hash (first 16 bytes, hex) of the directory path
  - Global skills cached as `global.json`
  - `GetProjectSkills(dir, forceRefresh)` and `GetGlobalSkills(forceRefresh)` manage the cache lifecycle
  - Cache miss or TTL expiry triggers a fresh scan; results are written back even on forceRefresh

### Routing Rules
- Stored in `~/.config/ccc/routing-rules.yaml` (file-based, not in SQLite)
- Map of directory path -> `RoutingRule` with `use_for` and `not_for` string lists
- `LoadRoutingRules()` -- returns empty map if file is missing or malformed (logs warning on parse failure)
- `SaveRoutingRules(rules)` -- writes full map to YAML, creates parent directories
- `AddRoutingRule(path, ruleType, text)` -- appends a single entry; ruleType must be `"use_for"` or `"not_for"`
- Rules inform the LLM routing prompt but are not hard constraints

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
- `DeferTodo` inserts after the last active (non-terminal) item in the slice — terminal items retain their original sort_order so they can appear anywhere in `cc.Todos`
- `ActiveTodos`, `CompletedTodos` -- filtered/sorted views
- `VisibleTodos` -- returns `ActiveTodos` further filtered to exclude merge-hidden todos (todos whose IDs appear in active `cc_todo_merges` entries). This is the primary view used by the UI.
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
- Path CRUD (add, duplicate ignore, remove)
- Bookmark CRUD (insert, load, remove)
- JSON migration (full data, idempotent re-run, empty DB guard)
- DBIsEmpty on fresh vs populated DB
- DBSaveRefreshResult round-trip
- In-memory mutations (complete, remove, add, defer, promote)
- ActiveTodos/CompletedTodos filtering
- DueUrgency for overdue/soon/later/none/bad-date
- RelativeTime for minutes/hours/days
- FindConflicts with overlaps and no overlaps
- Session file parsing (valid, missing fields, no frontmatter)
- Winddown sessions (multiple files, empty dir, missing dir)
- Bookmarks from JSON file (valid, missing file)
- LoadAllSessions merged and sorted
- RemoveBookmark from JSON file
- Path description: set and load round-trip via `DBUpdatePathDescription` / `DBLoadPathsFull`
- Path description: update on nonexistent path returns error
- AutoDescribePath: detects Go, Node, Rust, Python, Ruby, Java, Swift projects
- AutoDescribePath: returns empty string for unrecognized directory
- Skills discovery: finds SKILL.md files with valid frontmatter
- Skills discovery: returns empty for directory without `.claude/skills/`
- Skills discovery: skips malformed YAML, missing frontmatter, missing name
- Skills disk cache: returns cached skills within TTL
- Skills disk cache: misses on expired TTL (>1 hour)
- Skills disk cache: misses on missing file
- Skills disk cache: write then load round-trip
- Routing rules: load from valid YAML with use_for and not_for entries
- Routing rules: missing file returns empty map (no error)
- Routing rules: malformed YAML returns empty map (logs warning)
- Routing rules: AddRoutingRule creates file and appends entries
- Routing rules: AddRoutingRule rejects invalid rule type
- Routing rules: save and load round-trip preserves all entries
- PR upsert preserves agent columns when GitHub-sourced fields update
- PRs missing from fresh batch get `state = "archived"`, not deleted
- Archived PRs reappearing get `state = "open"` (reactivation)
- `DBLoadPullRequests` returns only `state = "open"` PRs
- `DBUpdatePRAgentStatus` updates agent columns without touching GitHub fields
- `head_sha` round-trips correctly through save/load
- `DBSetPRIgnored` sets ignored flag; `DBLoadPullRequests` excludes ignored PRs
- `DBAddIgnoredRepo` + `DBLoadPullRequests` excludes all PRs from that repo
- `DBRemoveIgnoredRepo` restores PRs from that repo to query results
- `DBLoadIgnoredRepos` returns all ignored repos
- `DBLoadIgnoredPRs` returns non-archived PRs with `ignored = 1`
