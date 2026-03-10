# Large Files & Complexity Audit

## Summary

The codebase has 6 production files over 300 lines, with the command center plugin (1330 lines) being the most severe offender. The biggest complexity risks are two monolithic functions in `commandcenter.go` -- `handleCommandTab` (293 lines) and `HandleMessage` (252 lines) -- which combine input handling, state mutation, DB writes, and LLM dispatch into single switch statements.

## Large Files (>300 lines)

| File | Lines | Responsibilities | Verdict |
|------|-------|-----------------|---------|
| `internal/builtin/commandcenter/commandcenter.go` | 1330 | Plugin lifecycle, key dispatch, command-tab key handling (todos CRUD, undo, booking, navigation), threads-tab key handling, message handling (DB loads, claude responses, refresh, focus), view routing, helpers | **Split needed** -- at least 5 distinct responsibilities |
| `internal/builtin/settings/settings.go` | 1022 | Plugin lifecycle, sub-view routing (plugins/logs/palette), key handling, config toggle logic, detail views for 4 data sources (calendar/github/granola/builtin), styles | **Split needed** -- detail views and styles should extract |
| `internal/builtin/commandcenter/cc_view.go` | 805 | Calendar rendering, todo panel, expanded todo view, detail view, warning/suggestion banners, footer, help overlay | **Acceptable** -- all rendering, but could split calendar vs todo views |
| `internal/db/db.go` | 781 | Schema migration, all read queries (todos/threads/calendar/suggestions/actions/bookmarks/paths), all write queries, bulk save, helpers | **Split needed** -- read/write/migration are 3 clear modules |
| `internal/builtin/sessions/sessions.go` | 775 | Plugin lifecycle, new/resume tab handling, fzf integration, item delegate rendering, confirmation dialog, session loading, styles | **Borderline** -- manageable, but styles and delegates could extract |
| `internal/db/types.go` | 666 | Domain types (CommandCenter, Todo, Thread, etc.), mutation methods, time helpers, file-based session parsing, path CRUD, bookmark file I/O | **Split needed** -- session/bookmark file parsing is unrelated to domain types |
| `internal/refresh/auth.go` | 362 | OAuth2 for Calendar, Gmail, Granola, GitHub, Slack token loading, calendar auth flow, credential migration, env file loading | **Borderline** -- all auth, but RunCalendarAuth (HTTP server) is distinct from token loading |
| `internal/tui/model.go` | 356 | Host model, plugin init/wiring, tab navigation, message broadcast, action processing, view | **Acceptable** -- single responsibility as host orchestrator |
| `internal/external/external.go` | 300 | External plugin adapter: subprocess lifecycle, IPC for all plugin interface methods | **Acceptable** -- single adapter pattern |

## Long Functions (>50 lines)

### Critical (>100 lines)

| File | Function | Lines | Issue |
|------|----------|-------|-------|
| `commandcenter.go` | `handleCommandTab` | L447-739 (293) | Monolithic switch with 13 cases, each containing state mutation + DB write + optional LLM trigger. Repeated undo/cursor-fix/focus-refresh pattern across cases. |
| `commandcenter.go` | `HandleMessage` | L959-1210 (252) | Giant type switch handling 10+ message types. Each case does JSON parsing, state mutation, DB writes. The `claudeCommandFinishedMsg` case alone is ~75 lines. |
| `cmd/ccc/main.go` | `main` | L20-168 (149) | Config loading, DB init, plugin wiring, launch loop, signal handling. Acceptable for a main function but could extract launch loop. |
| `db/db.go` | `DBSaveRefreshResult` | L618-730 (113) | Transaction with 6 sequential table operations. Long but linear -- acceptable for a bulk save. |
| `db/migrate.go` | `migrateCommandCenter` | L45-150 (106) | JSON-to-SQLite migration. One-time code, acceptable. |
| `refresh/refresh.go` | `Run` | L28-130 (103) | Orchestrates all data source fetches. Linear pipeline, acceptable. |

### Notable (50-100 lines)

| File | Function | Lines | Issue |
|------|----------|-------|-------|
| `settings.go` | `handleDetailKey` | L428-521 (94) | Handles text input editing for repos/username + general detail keys + GitHub-specific keys. Three concerns in one function. |
| `cc_view.go` | `renderTodoPanel` | L324-416 (93) | Builds todo list with scroll window, cursor highlight, details, and completed section. Moderately complex but cohesive rendering. |
| `config/setup.go` | `RunSetup` | L13-104 (92) | Interactive setup wizard. Linear flow, acceptable. |
| `external.go` | `startProcess` | L52-137 (86) | Subprocess init with handshake, route/key/migration conversion. Could extract conversion helpers. |
| `commandcenter.go` | `handleThreadsTab` | L876-956 (81) | Thread key handling -- simpler version of handleCommandTab pattern. |
| `refresh/granola.go` | `granolaListMeetings` | L117-196 (80) | HTTP request + nested JSON parsing with 3 struct definitions inline. |
| `refresh/slack.go` | `extractSlackCommitments` | L205-283 (79) | Iterates messages, builds LLM prompt, parses JSON response. |
| `cc_view.go` | `renderDetailView` | L484-560 (77) | Todo detail panel with field display and input. Cohesive. |
| `cc_view.go` | `renderExpandedTodoItem` | L606-681 (76) | Single todo item in expanded view. Long due to styling logic. |
| `commandcenter.go` | `Init` | L218-293 (76) | Plugin initialization. Long but linear setup. |
| `auth.go` | `RunCalendarAuth` | L202-275 (74) | OAuth2 flow with HTTP callback server. Distinct from rest of auth.go. |

## Deep Nesting Hotspots

### `commandcenter.go` -- 205 deeply nested lines, max depth 7

The worst nesting is in `HandleMessage` inside the `claudeEditFinishedMsg` case (L993-1017):
```
func HandleMessage > switch msg > case claudeEditFinishedMsg > if no error > for todos > if match > if zero time
```
This 7-level nesting makes the code hard to follow. The `claudeCommandFinishedMsg` case (L1050-1124) is similarly deep with nested loops and conditionals for todo creation and completion.

In `handleCommandTab`, the navigation cases (up/down/left/right) reach depth 6 due to expanded-view column math nested inside key-case blocks (L464-492).

### `settings.go` -- 81 deeply nested lines, max depth 6

The `handlePaletteKey` function nests event bus publishing 6 levels deep (L297-305). The `applyToggle` function has validation error handling at depth 6 (L340-349).

### `external.go` -- 21 deeply nested lines, max depth 6

The `HandleMessage` drain loop reaches depth 6 for event publishing (L244-251). Could be extracted into a helper.

## Recommendations

### Priority 1: Split `commandcenter.go` (1330 lines)

This file does too much. Suggested extraction:

1. **`cc_keys.go`** -- Extract `handleCommandTab`, `handleThreadsTab`, `handleBooking`, `handleDetailView`, `handleAddingTodoRich`, `handleTextInput`. These are all key-handling functions that share state but are logically distinct input modes.
2. **`cc_messages.go`** -- Extract `HandleMessage` into a separate file. Each `case` block (claude edit/enrich/command/focus finished, DB writes, refresh) could become a named method like `handleClaudeEditFinished(msg)`.
3. **Reduce `handleCommandTab`** -- The 13 switch cases share a pattern: bounds-check, mutate state, build DB cmd, optionally trigger focus refresh. Extract a helper like `todoAction(id string, mutate func(), dbFn func(*sql.DB) error) plugin.Action`.

### Priority 2: Split `db/db.go` (781 lines)

- **`db/read.go`** -- All `dbLoad*` functions and `LoadCommandCenterFromDB`
- **`db/write.go`** -- All `DB*` write functions
- **`db/schema.go`** -- `migrateSchema` (already has separate `migrate.go` for JSON migration)

### Priority 3: Extract session parsing from `db/types.go` (666 lines)

`ParseSessionFile`, `LoadWinddownSessions`, `LoadBookmarks`, `LoadAllSessions`, `RemoveBookmark`, and the `Session`/`Bookmark` types are session-file I/O, not domain types. Move to `db/sessions.go`.

### Priority 4: Reduce nesting in message handlers

The `claudeCommandFinishedMsg` handler in `HandleMessage` (L1050-1124) should use early returns and extracted helpers:
- Extract JSON parsing into `parseCommandResponse(output string) (*commandResponse, error)`
- Extract todo creation loop into `applyCommandTodos(resp *commandResponse) []tea.Cmd`

### Priority 5: Extract `settings.go` detail views

Each `viewDetail*` function (Calendar, GitHub, Granola, ExternalPlugin, BuiltinPlugin) plus the styles struct could live in `settings_views.go` or `settings_styles.go`, bringing the main file under 700 lines.
