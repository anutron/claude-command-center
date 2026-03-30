# Spec Coverage Audit: internal/db

- **Date:** 2026-03-29
- **Spec:** `specs/core/db.md`
- **Code files:** `internal/db/*.go` (excluding tests)

## Summary

- **Covered:** 68 / 97 behavioral branches
- **Uncovered-Behavioral:** 25 gaps
- **Uncovered-Implementation:** 4 (no spec needed)
- **Contradictions:** 4

---

## schema.go

### `OpenDB(dbPath)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| Happy path: create dirs, open, WAL, busy_timeout, synchronous, max conns, migrate | **[COVERED]** | Spec "Database Lifecycle" S1: "creates directories, opens SQLite, sets WAL + busy_timeout + synchronous=NORMAL, max 1 connection, runs migrateSchema" |
| Error: MkdirAll fails | **[UNCOVERED-IMPLEMENTATION]** | Internal error propagation |
| Error: sql.Open fails | **[UNCOVERED-IMPLEMENTATION]** | Internal error propagation |
| Error: any PRAGMA fails, db closed | **[UNCOVERED-IMPLEMENTATION]** | Cleanup detail |

### `migrateSchema(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| Creates 8 initial tables | **[CONTRADICTS]** | Spec says "8 tables: cc_todos, cc_threads, cc_calendar_cache, cc_suggestions, cc_pending_actions, cc_meta, cc_bookmarks, cc_learned_paths". Code drops cc_threads (`DROP TABLE IF EXISTS cc_threads`) and creates additional tables: cc_source_sync, cc_todo_merges, cc_pull_requests, cc_sessions, cc_automation_runs, cc_agent_costs, cc_budget_state, cc_archived_sessions, cc_ignored_repos. The "8 tables" count and list is stale. |
| Post-DDL ALTER TABLE migrations (calendar_id, session_id, sort_order, etc.) | **[COVERED]** | Spec "Database Lifecycle" S4 lists these |
| sort_order dedup via ROW_NUMBER | **[COVERED]** | Spec S5: "fixes duplicate sort_order values" |
| display_id column + backfill (NULL and 0) | **[UNCOVERED-BEHAVIORAL]** | Spec does not mention display_id column migration, backfill for NULL, or BUG-101 backfill for display_id=0. **Intent question:** Should the spec document the display_id auto-assignment and backfill migrations? |
| triage_status column + FSM migration | **[UNCOVERED-BEHAVIORAL]** | `migrateTodoStatusFSM` remaps three-field status model to single status FSM. Spec mentions none of this. **Intent question:** Should the spec document the todo status FSM migration (status redesign from triage_status+session_status to single status)? |
| cc_source_sync table creation | **[UNCOVERED-BEHAVIORAL]** | Table for sync tracking, not in spec's table list. **Intent question:** Should the spec document the source sync tracking table and its operations? |
| cc_todo_merges table creation | **[UNCOVERED-BEHAVIORAL]** | Table for duplicate detection, not in spec's table list. **Intent question:** Should the spec document the merge tracking table? |
| cc_sessions table creation | **[UNCOVERED-BEHAVIORAL]** | Daemon session registry table, not in spec's table list. |
| cc_automation_runs table creation | **[UNCOVERED-BEHAVIORAL]** | Automation framework table, not in spec's table list. |
| cc_agent_costs table creation | **[UNCOVERED-BEHAVIORAL]** | Agent governance table, not in spec's table list. |
| cc_budget_state table creation | **[UNCOVERED-BEHAVIORAL]** | Budget cap table, not in spec's table list. |
| cc_archived_sessions table creation | **[UNCOVERED-BEHAVIORAL]** | Auto-saved ended sessions, not in spec's table list. |
| launch_mode, session_log_path, proposed_prompt, session_summary columns | **[UNCOVERED-BEHAVIORAL]** | Post-DDL migrations for todo agent features, not in spec's ALTER TABLE list. |

### `FormatTime(t)` / `ParseTime(s)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| FormatTime returns UTC RFC3339 | **[COVERED]** | Spec "Helpers": "FormatTime(t) -- UTC RFC3339" |
| ParseTime RFC3339 success | **[COVERED]** | Spec "Helpers": "ParseTime(s) -- parses RFC3339 or bare datetime, returns local time" |
| ParseTime bare datetime fallback | **[COVERED]** | Same spec line |
| ParseTime both fail, returns zero | **[UNCOVERED-IMPLEMENTATION]** | Edge case, zero-value return |

### `columnExists(db, table, column)` / `migrateTodoStatusFSM(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[UNCOVERED-BEHAVIORAL]** | Entire status FSM migration is unspecced (see above) |

---

## types.go

### Type Definitions

| Item | Classification | Notes |
|------|---------------|-------|
| CommandCenter, CalendarData, CalendarEvent, CalendarConflict, Todo, Suggestions, PendingAction, Warning, PathEntry | **[COVERED]** | Spec "Types" section lists all of these |
| PullRequest type | **[COVERED]** | Spec "Pull Requests" section documents fields |
| SessionRecord type | **[UNCOVERED-BEHAVIORAL]** | Daemon session registry type not in spec's Types list. **Intent question:** Should the spec document SessionRecord? |
| SourceSync type | **[UNCOVERED-BEHAVIORAL]** | Source sync tracking type not in spec's Types list. |
| TodoMerge type | **[UNCOVERED-BEHAVIORAL]** | Merge tracking type not in spec's Types list. |
| ArchivedSession type | **[UNCOVERED-BEHAVIORAL]** | Archived session type not in spec's Types list. |
| Todo status constants (StatusNew, StatusBacklog, etc.) | **[UNCOVERED-BEHAVIORAL]** | Entire status FSM with 9 states and transition rules is unspecced. **Intent question:** Should the spec document the todo status state machine? |
| ValidTransition, IsTerminalStatus, IsAgentStatus | **[UNCOVERED-BEHAVIORAL]** | Status classification helpers are unspecced. |

### `GenID()`

| Branch | Classification | Notes |
|--------|---------------|-------|
| Returns 8-char hex | **[COVERED]** | Spec "Helpers": "GenID() -- 8-char random hex" |

### `RelativeTime(t)`, `DueUrgency(due)`, `FormatDueLabel(due)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[COVERED]** | Spec "Helpers" section documents all three |

### In-Memory Mutations

| Method | Classification | Notes |
|--------|---------------|-------|
| CompleteTodo | **[COVERED]** | Spec "In-Memory Mutations" |
| RestoreTodo | **[COVERED]** | Spec "In-Memory Mutations" |
| AddTodo (assigns DisplayID) | **[COVERED]** | Spec "In-Memory Mutations": "AddTodo" |
| RemoveTodo | **[COVERED]** | Spec "In-Memory Mutations" |
| DeferTodo (insert after last non-terminal) | **[COVERED]** | Spec: "DeferTodo inserts after the last active (non-terminal) item in the slice" |
| PromoteTodo | **[COVERED]** | Spec "In-Memory Mutations" |
| SwapTodos | **[UNCOVERED-BEHAVIORAL]** | Not mentioned in spec. **Intent question:** Should the spec document SwapTodos? |
| AcceptTodo | **[UNCOVERED-BEHAVIORAL]** | Not in spec's mutation list. Transitions to StatusBacklog. |
| FindTodo | **[UNCOVERED-BEHAVIORAL]** | Not in spec. Returns pointer to todo by ID. |
| VisibleTodos (filters by non-vetoed merges) | **[UNCOVERED-BEHAVIORAL]** | Not in spec. Merge-aware visible filtering. |
| ActiveTodos | **[CONTRADICTS]** | Spec says "ActiveTodos" is a "filtered/sorted view", but code now uses `VisibleTodos()` which filters out merge-hidden todos. Spec doesn't mention merge-based filtering in ActiveTodos. |
| CompletedTodos | **[COVERED]** | Spec "In-Memory Mutations" |
| AddPendingBooking | **[COVERED]** | Spec "In-Memory Mutations" |
| FindConflicts | **[COVERED]** | Spec: "FindConflicts -- detects overlapping calendar events (skips declined, all-day, ended)" |

### `DBGetOriginalIDs`, `WerePreviouslyMergedAndVetoed`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[UNCOVERED-BEHAVIORAL]** | Merge-related helpers not in spec. |

### File-Based Path Functions

| Function | Classification | Notes |
|----------|---------------|-------|
| LoadPaths, SavePaths, AddPath, RemovePath | **[COVERED]** | Spec "File-Based Path Functions" |

---

## read.go

### `LoadCommandCenterFromDB(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| Happy path: loads todos, calendar, suggestions, PRs, pending actions, generated_at, merges | **[CONTRADICTS]** | Spec says it returns `*CommandCenter` with "calendar, todos, threads, suggestions, pending actions, warnings, generated_at". Code loads PRs and merges (not threads or warnings). Spec is stale. |
| Any sub-load error propagates | **[UNCOVERED-IMPLEMENTATION]** | Error propagation detail |

### `dbLoadCalendar(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| Day-clamping of multi-day events | **[COVERED]** | Spec "Calendar & Suggestions": detailed clamping description |
| All-day promotion (wasClamped + >=12h) | **[UNCOVERED-BEHAVIORAL]** | Code promotes clamped events spanning >=12h to all-day. Spec doesn't mention this heuristic. **Intent question:** Should the spec document the all-day promotion heuristic for clamped events? |
| Sort: all-day first, then by start time | **[COVERED]** | Spec: "Events are then re-sorted by effective start time" (partially — doesn't mention all-day-first ordering) |

### `DBLoadPullRequests(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| Filters archived, ignored, ignored-repo PRs | **[COVERED]** | Spec "Pull Requests - Load" |
| Ordered by last_activity_at DESC | **[COVERED]** | Spec "Pull Requests - Load" |

### `DBLoadBookmarks(db)`, `DBLoadPaths(db)`, `DBLoadPathsFull(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[COVERED]** | Spec "Bookmarks & Paths (DB)" |

### `DBLoadSourceSync(db, source)`, `DBLoadAllSourceSync(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[UNCOVERED-BEHAVIORAL]** | Source sync read operations not in spec. |

### `DBLoadTodoByID(db, id)`, `DBLoadTodoByDisplayID(db, displayID)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| Returns nil,nil on not found | **[UNCOVERED-BEHAVIORAL]** | Individual todo lookup functions not in spec. |
| Returns populated todo on found | **[UNCOVERED-BEHAVIORAL]** | Same |

### `DBLoadMerges(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[UNCOVERED-BEHAVIORAL]** | Merge loading not in spec. |

### `DBLoadSessions(db)`, `DBLoadVisibleSessions(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[UNCOVERED-BEHAVIORAL]** | Daemon session registry reads not in spec. |

### `DBLoadIgnoredRepos(db)`, `DBLoadIgnoredPRs(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[COVERED]** | Spec "Pull Requests" section |

### `DBIsEmpty(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[COVERED]** | Spec "Helpers": "DBIsEmpty(db) -- checks if any todos exist" |

### `DBLoadArchivedSessions(db)`

| Branch | Classification | Notes |
|--------|---------------|-------|
| All branches | **[UNCOVERED-BEHAVIORAL]** | Archived sessions not in spec. |

---

## write.go

### Todo Write Operations

| Function | Classification | Notes |
|----------|---------------|-------|
| DBInsertTodo (auto display_id, sort_order) | **[COVERED]** | Spec "Todo Operations": "auto-assigns sort_order = max+1" |
| DBCompleteTodo | **[COVERED]** | Spec "Todo Operations" |
| DBDismissTodo | **[COVERED]** | Spec "Todo Operations" |
| DBRestoreTodo | **[COVERED]** | Spec "Todo Operations" |
| DBDeferTodo | **[COVERED]** | Spec "Todo Operations" |
| DBPromoteTodo | **[COVERED]** | Spec "Todo Operations" |
| DBUpdateTodo | **[COVERED]** | Spec "Todo Operations" |
| DBUpdateTodoSourceContext | **[COVERED]** | Spec "Todo Operations" |
| DBAcceptTodo | **[UNCOVERED-BEHAVIORAL]** | Transitions todo to "backlog". Not in spec. |
| DBUpdateTodoStatus | **[UNCOVERED-BEHAVIORAL]** | Generic status updater. Not in spec. |
| DBUpdateTodoProjectDir | **[UNCOVERED-BEHAVIORAL]** | Focused project_dir update. Not in spec. |
| DBUpdateTodoLaunchMode | **[UNCOVERED-BEHAVIORAL]** | Focused launch_mode update. Not in spec. |
| DBUpdateTodoSessionID | **[UNCOVERED-BEHAVIORAL]** | Focused session_id update. Not in spec. |
| DBUpdateTodoSessionSummary | **[UNCOVERED-BEHAVIORAL]** | Focused session_summary update. Not in spec. |

### Calendar & Suggestions

| Function | Classification | Notes |
|----------|---------------|-------|
| DBReplaceCalendar | **[COVERED]** | Spec "Calendar & Suggestions" |
| DBSaveFocus | **[COVERED]** | Spec "Calendar & Suggestions" |
| DBSaveSuggestions | **[COVERED]** | Spec "Calendar & Suggestions" |

### Other Write Operations

| Function | Classification | Notes |
|----------|---------------|-------|
| DBInsertPendingAction | **[UNCOVERED-BEHAVIORAL]** | Individual pending action insert, not in spec (spec only has DBClearPendingActions). |
| DBInsertBookmark, DBRemoveBookmark | **[COVERED]** | Spec "Bookmarks & Paths (DB)" |
| DBAddPath, DBRemovePath, DBUpdatePathDescription | **[COVERED]** | Spec "Bookmarks & Paths (DB)" |
| DBSwapPathOrder | **[UNCOVERED-BEHAVIORAL]** | Transactional sort_order swap. Not in spec. |
| DBSwapTodoOrder | **[UNCOVERED-BEHAVIORAL]** | Transactional sort_order swap. Not in spec. |
| DBSetMeta | **[COVERED]** | Spec "Meta & Pending Actions" |
| DBClearPendingActions | **[COVERED]** | Spec "Meta & Pending Actions" |
| DBSaveRefreshResult | **[COVERED]** | Spec "Bulk Refresh" |

### Pull Request Write Operations

| Function | Classification | Notes |
|----------|---------------|-------|
| DBSavePullRequests (merge upsert, archive, reactivate) | **[COVERED]** | Spec "Pull Requests - Save strategy" |
| DBSetPRIgnored | **[COVERED]** | Spec "Pull Requests" |
| DBAddIgnoredRepo, DBRemoveIgnoredRepo | **[COVERED]** | Spec "Pull Requests" |
| DBUpdatePRAgentStatus | **[COVERED]** | Spec "Pull Requests" |

### Source Sync

| Function | Classification | Notes |
|----------|---------------|-------|
| DBUpsertSourceSync | **[UNCOVERED-BEHAVIORAL]** | Source sync write not in spec. |

### Merge Write Operations

| Function | Classification | Notes |
|----------|---------------|-------|
| DBInsertMerge, DBSetMergeVetoed, DBDeleteSynthesisMerges, DBDeleteTodo | **[UNCOVERED-BEHAVIORAL]** | All merge write operations unspecced. |

### Session Write Operations (Daemon Registry)

| Function | Classification | Notes |
|----------|---------------|-------|
| DBInsertSession, DBUpdateSession, DBUpdateSessionState | **[UNCOVERED-BEHAVIORAL]** | Daemon session registry writes not in spec. |

### Archived Session Write Operations

| Function | Classification | Notes |
|----------|---------------|-------|
| DBInsertArchivedSession, DBDeleteArchivedSession | **[UNCOVERED-BEHAVIORAL]** | Archived session writes not in spec. |

---

## agent_costs.go

| Function | Classification | Notes |
|----------|---------------|-------|
| DBInsertAgentCost | **[UNCOVERED-BEHAVIORAL]** | Agent cost tracking not in spec at all. |
| DBUpdateAgentCostFinished | **[UNCOVERED-BEHAVIORAL]** | Same |
| DBSumCostsSince | **[UNCOVERED-BEHAVIORAL]** | Same |
| DBCountLaunchesSince | **[UNCOVERED-BEHAVIORAL]** | Same |
| DBLastAgentLaunch | **[UNCOVERED-BEHAVIORAL]** | Same |
| DBCountRecentFailures | **[UNCOVERED-BEHAVIORAL]** | Same |
| DBGetBudgetState | **[UNCOVERED-BEHAVIORAL]** | Same |
| DBSetBudgetState | **[UNCOVERED-BEHAVIORAL]** | Same |

---

## sessions.go (file-based)

| Function | Classification | Notes |
|----------|---------------|-------|
| ParseSessionFile | **[COVERED]** | Spec "File-Based Session Functions" |
| LoadWinddownSessions | **[COVERED]** | Spec "File-Based Session Functions" |
| LoadBookmarks (file) | **[COVERED]** | Spec "File-Based Session Functions" |
| LoadAllSessions | **[COVERED]** | Spec "File-Based Session Functions" |
| RemoveBookmark | **[COVERED]** | Spec "File-Based Session Functions" |

---

## skill_discover.go

| Function | Classification | Notes |
|----------|---------------|-------|
| DiscoverSkills | **[COVERED]** | Spec "Skills Discovery" |
| DiscoverGlobalSkills | **[COVERED]** | Spec "Skills Discovery" |
| Frontmatter parsing (name required, skip invalid) | **[COVERED]** | Spec "Skills Discovery" |
| LoadCachedSkills / WriteCachedSkills | **[COVERED]** | Spec "Skills Discovery - Disk cache" |
| GetProjectSkills / GetGlobalSkills (cache lifecycle) | **[COVERED]** | Spec "Skills Discovery - Disk cache" |

---

## routing_rules.go

| Function | Classification | Notes |
|----------|---------------|-------|
| LoadRoutingRules (happy, missing file, malformed) | **[COVERED]** | Spec "Routing Rules" |
| SaveRoutingRules | **[COVERED]** | Spec "Routing Rules" |
| AddRoutingRule (valid types, invalid type rejection) | **[COVERED]** | Spec "Routing Rules" |
| SetPromptHint | **[UNCOVERED-BEHAVIORAL]** | Not in spec. Sets prompt_hint field on a routing rule. **Intent question:** Should the spec document SetPromptHint and the prompt_hint field? |
| RoutingRule.PromptHint field | **[CONTRADICTS]** | Spec says RoutingRule has "use_for and not_for string lists". Code also has `PromptHint string`. |

---

## path_describe.go

| Function | Classification | Notes |
|----------|---------------|-------|
| AutoDescribePath (all file checks, empty result) | **[COVERED]** | Spec "Path Descriptions" |

---

## migrate.go

| Function | Classification | Notes |
|----------|---------------|-------|
| MigrateFromJSON (empty guard, idempotent, all sub-migrations) | **[COVERED]** | Spec "Migration (Legacy)" |

---

## Spec-to-Code Direction (items in spec not found in code)

| Spec Claim | Status | Notes |
|------------|--------|-------|
| "cc_threads" table in schema list | **[CONTRADICTS]** | Code drops this table. Threads feature removed. |
| Thread Operations (DBInsertThread, DBPauseThread, DBStartThread, DBCloseThread) | **[CONTRADICTS]** | None of these exist in code. Threads feature removed. |
| In-memory PauseThread, StartThread, CloseThread, AddThread | **[CONTRADICTS]** | None exist. Threads feature removed. |
| ActiveThreads, PausedThreads filtered views | **[CONTRADICTS]** | None exist. Threads feature removed. |

---

## Contradictions Summary

1. **Stale table list:** Spec says "8 tables" including cc_threads. Code has ~15 tables, no cc_threads.
2. **Thread operations exist in spec but not code:** Entire Thread subsystem was removed (preserved on branch). Spec still documents it.
3. **ActiveTodos uses VisibleTodos (merge filtering):** Spec doesn't mention merge-aware filtering.
4. **RoutingRule.PromptHint field:** Exists in code, not in spec.

## Top Behavioral Gaps (by impact)

1. **Todo status FSM** -- 9 states, transition rules, ValidTransition, IsTerminalStatus, IsAgentStatus -- entirely unspecced
2. **Agent cost tracking** -- 8 exported functions for budget/cost governance -- entirely unspecced
3. **Daemon session registry** -- SessionRecord type, insert/update/load operations -- entirely unspecced
4. **Todo merge system** -- TodoMerge type, DBInsertMerge, DBSetMergeVetoed, DBDeleteSynthesisMerges, VisibleTodos, DBGetOriginalIDs, WerePreviouslyMergedAndVetoed -- entirely unspecced
5. **Source sync tracking** -- SourceSync type, DBUpsertSourceSync, DBLoadSourceSync, DBLoadAllSourceSync -- entirely unspecced
6. **Archived sessions** -- ArchivedSession type, CRUD operations -- entirely unspecced
7. **Focused todo updaters** -- DBUpdateTodoStatus, DBAcceptTodo, DBUpdateTodoProjectDir, DBUpdateTodoLaunchMode, DBUpdateTodoSessionID, DBUpdateTodoSessionSummary -- unspecced
