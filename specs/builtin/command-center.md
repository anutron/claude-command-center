# SPEC: Command Center Plugin (built-in)

## Purpose

The main productivity hub plugin. Manages todos, threads, calendar events, AI-powered suggestions, and Claude integration. Provides two routes: the command center view (calendar + todos) and the threads view.

## Slug: `commandcenter`

## Routes

- `commandcenter` — default view (calendar + todo panels)
- `commandcenter/threads` — threads sub-view (active + paused threads)

## State

- `cc *db.CommandCenter` — loaded from DB, contains todos, threads, calendar, suggestions
- `cursor int` — selected todo/thread index
- `sub string` — active sub-view: `"cc"` or `"threads"`
- `showHelp bool` — help overlay toggle
- `showBacklog bool` — show/hide completed todos
- `detailMode bool` — viewing a single todo's detail
- `richMode bool` — rich textarea for AI-powered todo creation
- `bookingMode bool` — calendar event booking flow
- `spinners` — loading indicators for async operations

## Key Bindings

| Key | Mode | Description | Promoted |
|-----|------|-------------|----------|
| up/down/j/k | normal | Navigate todos/threads | yes |
| enter | normal | View todo detail / launch thread | yes |
| space | normal | Toggle todo detail with edit | yes |
| x | normal | Complete todo | yes |
| d | normal | Dismiss todo | yes |
| z | normal | Undo last completion | yes |
| c | normal | Create todo (rich textarea) | yes |
| p | normal | Promote todo to top | yes |
| D | normal | Defer todo to bottom | yes |
| r | normal | Refresh data from external sources | yes |
| b | normal | Toggle backlog (completed items) | yes |
| B | normal | Enter booking mode | yes |
| ? | normal | Toggle help overlay | yes |
| / | threads | Filter threads | yes |
| P | threads | Pause/resume thread | yes |
| X | threads | Close thread | yes |
| A | threads | Add new thread | yes |
| esc | detail/rich | Return to list | no |
| ctrl+d | rich | Submit to Claude for processing | no |

## Event Bus

- Publishes: `todo.created`, `todo.completed`, `todo.dismissed`, `focus.updated`
- Subscribes: `session.launch` (for context when launching from a todo)

## Migrations

None — uses existing `cc_todos`, `cc_threads`, `cc_calendar_cache`, `cc_suggestions` tables.

## Architecture

### Files

- `commandcenter.go` — Main plugin struct, Init, HandleKey, HandleMessage, state management
- `cc_view.go` — Command center rendering: calendar panel, todo panel, warnings, suggestions, help overlay
- `threads_view.go` — Threads tab: active/paused sections with type prefixes
- `claude_exec.go` — Background Claude CLI commands (edit, enrich, command, focus), prompt builders
- `refresh.go` — Background refresh command (finds `ccc-refresh` binary)
- `styles.go` — Local style/gradient types populated from `config.Palette` (avoids circular imports with tui)

### Local Style Types

Each plugin defines local style and gradient structs populated from the shared `config.Palette` during `Init()`. This avoids circular imports between plugins and the tui host package.

## Behavior

### Command Center View

1. Left panel: calendar (today's events with times, colors from config)
2. Right panel: todos sorted by sort_order, with status indicators
3. Focus suggestion banner at top when available
4. Warning bar when data is stale or services are unreachable
5. Help overlay toggled with `?`

### Todo Lifecycle

- Create via `c` (rich textarea → Claude CLI processes natural language → structured todo)
- Complete with `x` (moves to completed, undo with `z`)
- Dismiss with `d` (tombstoned, never recreated by refresh)
- Defer with `D` (moves to bottom of list)
- Promote with `p` (moves to top of list)
- Detail view with `space` or `enter` (shows full context, edit input)

### Thread Lifecycle

- Active threads shown with type prefix (PR, issue, conversation)
- Pause/resume with `P`, close with `X`, add with `A`
- Thread URLs can be opened in browser

### Claude Integration

- `c` key opens rich textarea; `ctrl+d` submits text to Claude CLI for todo creation
- `space` on todo opens detail view with edit input for Claude-powered enrichment
- Focus suggestion auto-refreshes after todo mutations
- All Claude calls run as background `tea.Cmd` (non-blocking)

### Data Loading (Lifecycle Messages)

Instead of polling on a 60-second timer, the command center uses lifecycle messages
to reload data from the DB at the right moments:

- **TabViewMsg:** Reload from DB if stale (>2s since last read). This covers tab switches.
- **ReturnMsg:** Always reload from DB. This covers returning from a Claude session.
- **NotifyMsg:** Reload from DB. This covers cross-instance notifications.

The 60-second tick-based DB reload has been removed.

### Refresh (ccc-refresh)

- Auto-refresh triggers when data is older than 5 minutes (tick-based, preserved)
- Manual refresh via `r` key
- Spawns `ccc-refresh` binary, then reloads from DB
- Refresh binary located next to running executable, then falls back to PATH

### Cross-Plugin Navigation

When a todo has a `project_dir`, pressing enter navigates to the sessions plugin with that todo as context (via the host's "navigate" action with payload "sessions").

## Test Cases

- Slug and tab name are correct
- Routes returns both routes
- Init loads command center data from DB
- Navigation (up/down) moves cursor and wraps
- Complete todo updates status and triggers undo state
- Dismiss todo updates status
- Undo completion restores previous state
- Create todo enters rich mode
- Enter on todo with project_dir returns navigate action
- Enter on todo with session_id returns launch action
- Enter on todo without project_dir returns noop
- Sub-view switching between cc and threads
- Thread navigation works independently
- Defer moves todo to bottom
- Promote moves todo to top
- Toggle backlog shows/hides completed items
- View renders without panic (with and without data)
- Help overlay toggles
- Booking mode enters and exits
- HandleMessage processes ccLoadedMsg
- HandleMessage processes refreshFinishedMsg
- Add thread creates new thread
- Close thread updates status
- extractJSON handles raw JSON, fenced JSON, and embedded JSON
