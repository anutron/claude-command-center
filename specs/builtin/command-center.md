# SPEC: Command Center Plugin (built-in)

## Purpose

The main productivity hub plugin. Manages todos, threads, calendar events, AI-powered suggestions, and Claude integration. Provides two routes: the command center view (calendar + todos) and the threads view.

## Slug: `commandcenter`

## Routes

- `commandcenter` — default view (calendar + todo panels)
- `commandcenter/threads` — threads sub-view (active + paused threads)

## File Organization

| File | Responsibility |
|------|---------------|
| `commandcenter.go` | Main plugin struct, Init, NavigateTo, HandleMessage, Refresh, state management |
| `cc_keys.go` | All key handling: `HandleKey`, sub-handlers for command tab, threads tab, detail view, rich todo creation, text input, booking mode |
| `cc_messages.go` | Message handling for async results (Claude responses, refresh finished, DB writes) |
| `cc_view.go` | Command center rendering: calendar panel, todo panel, warnings, suggestions, help overlay, detail view, booking UI |
| `threads_view.go` | Threads tab: active/paused sections with type prefixes |
| `styles.go` | Local style/gradient types populated from `config.Palette` (avoids circular imports with tui) |
| `refresh.go` | Background refresh command (finds and spawns `ccc-refresh` binary) |
| `claude.go` | Background Claude CLI/LLM commands (edit, enrich, command, focus), prompt builders |

## State

- `cc *db.CommandCenter` — loaded from DB, contains todos, threads, calendar, suggestions
- `ccCursor int` — selected todo index in command tab
- `threadCursor int` — selected thread index in threads tab
- `subView string` — active sub-view: `"command"` or `"threads"`
- `showHelp bool` — help overlay toggle
- `showBacklog bool` — show/hide completed todos
- `detailView bool` — viewing a single todo's detail with edit input
- `detailNotice string` — transient notice banner in detail view (auto-clears after 1s)
- `addingTodoRich bool` — rich textarea for AI-powered todo creation
- `addingThread bool` — text input for adding a new thread
- `bookingMode bool` — calendar event booking flow
- `ccExpanded bool` — expanded multi-column todo view
- `undoStack []undoEntry` — stack of undo-able todo actions
- `pendingLaunchTodo *db.Todo` — todo awaiting session navigation

## Key Bindings

### Command Center Tab

| Key | Context | Description |
|-----|---------|-------------|
| `up`/`k` | normal | Move cursor up |
| `down`/`j` | normal | Move cursor down |
| `shift+up` | normal | Swap todo with the one above |
| `shift+down` | normal | Swap todo with the one below |
| `left`/`h` | expanded | Move cursor left; paginates to previous page at left edge |
| `right`/`l` | expanded | Move cursor right; paginates to next page at right edge |
| `x` | normal | Complete selected todo (pushes to undo stack) |
| `X` | normal | Dismiss selected todo (pushes to undo stack) |
| `u` | normal | Undo last complete/dismiss |
| `d` | normal | Defer selected todo to bottom of list |
| `p` | normal | Promote selected todo to top of list |
| `space` | normal | Open detail view with edit text input |
| `c` | normal | Create todo via rich textarea (AI-powered) |
| `b` | normal | Toggle backlog (completed items) |
| `s` | normal | Enter booking mode for selected todo |
| `r` | normal | Manual refresh (spawns ccc-refresh) |
| `enter` | normal | Launch session for todo (by session_id, project_dir, or navigate to sessions) |
| `?` | any | Toggle help overlay |
| `esc` | expanded | Collapse expanded view |
| `esc` | pending launch | Cancel pending launch, return to command view |

### Detail View

Title bar shows "TODO #N" using the todo's `display_id`.

| Key | Context | Description |
|-----|---------|-------------|
| `enter` | detail | Submit edit instruction to Claude LLM |
| `j` | detail | Navigate to next todo |
| `k` | detail | Navigate to previous todo |
| `x` | detail | Complete todo (shows notice banner, auto-advances after 1s) |
| `X` | detail | Dismiss todo (shows notice banner, auto-advances after 1s) |
| `esc` | detail | Return to list |

While a notice banner is showing (1s after complete/dismiss), all keys except `esc` are blocked. After the notice clears, the view auto-advances to the next todo.

### Rich Todo Creation

| Key | Context | Description |
|-----|---------|-------------|
| `ctrl+d` | rich | Submit text to Claude for processing |
| `esc` | rich | Cancel and return to list |

### Booking Mode

| Key | Context | Description |
|-----|---------|-------------|
| `left`/`h` | booking | Select shorter duration |
| `right`/`l` | booking | Select longer duration |
| `enter` | booking | Confirm booking and trigger refresh |
| `esc` | booking | Cancel booking |

### Threads Tab

| Key | Context | Description |
|-----|---------|-------------|
| `up`/`k` | threads | Move cursor up |
| `down`/`j` | threads | Move cursor down |
| `p` | threads | Pause active thread |
| `s` | threads | Start (resume) paused thread |
| `x` | threads | Close thread |
| `a` | threads | Add new thread (text input) |
| `enter` | threads | Launch session in thread's project_dir |

## Event Bus

- Publishes: `todo.completed`, `todo.dismissed`, `todo.deferred`, `todo.promoted`, `pending.todo`
- Subscribes to lifecycle messages: `TabViewMsg`, `ReturnMsg`, `NotifyMsg`, `LaunchMsg`

## Migrations

None — uses existing `cc_todos`, `cc_threads`, `cc_calendar_cache`, `cc_suggestions`, `cc_pending_actions`, `cc_meta`, `cc_source_sync` tables created by `db.migrateSchema`.

### Display IDs

Todos have a `display_id` column (auto-incrementing integer) for stable, human-readable references. Used in the detail view title ("TODO #N") and anywhere a short identifier is needed.

## Behavior

### Command Center View

1. Left panel: calendar (today's events with times, colors from config)
2. Right panel: todos sorted by sort_order, with status indicators
3. Focus suggestion banner at top when available
4. Warning bar when data is stale or services are unreachable
5. Help overlay toggled with `?`
6. Expanded multi-column view when scrolling past visible todos. Rows per column use `(height - 7) / 2` to maximize vertical space (no panel borders/calendar/warnings chrome in expanded view). Left/right arrows paginate when at column edges.

### Todo Lifecycle

- Create via `c` (rich textarea, `ctrl+d` submits to Claude LLM for structured todo creation)
- Complete with `x` (moves to completed, undo with `u`)
- Dismiss with `X` (tombstoned, never recreated by refresh)
- Defer with `d` (moves to bottom of list)
- Promote with `p` (moves to top of list)
- Detail view with `space` (shows full context, edit input for Claude-powered enrichment)
- Launch with `enter` (resumes session_id, launches in project_dir, or navigates to sessions)

### Thread Lifecycle

- Active threads shown with type prefix (PR, issue, conversation)
- Pause with `p`, start/resume with `s`, close with `x`, add with `a`
- Launch in thread's project_dir with `enter`

### Claude Integration

- `c` key opens rich textarea; `ctrl+d` submits text to Claude LLM for todo creation
- `space` on todo opens detail view with edit input for Claude-powered enrichment
- Focus suggestion auto-refreshes after todo mutations
- All Claude calls run as background `tea.Cmd` (non-blocking)
- Uses `LLM` abstraction layer (not direct CLI calls)

### Data Loading (Lifecycle Messages)

Instead of polling on a timer, the command center uses lifecycle messages to reload data from the DB at the right moments:

- **TabViewMsg:** Reload from DB if stale (>2s since last read)
- **ReturnMsg:** Always reload from DB (returning from a Claude session)
- **NotifyMsg:** Reload from DB (cross-instance notifications)

### Refresh (ccc-refresh)

- Auto-refresh triggers when data is older than a threshold (tick-based)
- Manual refresh via `r` key
- Spawns `ccc-refresh` binary, then reloads from DB
- Refresh binary located next to running executable, then falls back to PATH
- **Incremental sync**: Granola and Slack sources check `cc_source_sync` for their last successful sync time and skip already-processed meetings/messages, reducing LLM calls
- **Deterministic source_ref (Granola)**: Source refs use `{meeting_id}-{sha256(title)[:8]}` instead of LLM-generated values, making deduplication reliable
- **Merge preserves completed todos**: Refresh merge logic preserves completed todos as-is rather than overwriting them with fresh data

### Cross-Plugin Navigation

When a todo has a `project_dir`, pressing enter launches a Claude session there. When a todo has no project_dir, the plugin sets `pendingLaunchTodo` and navigates to the sessions plugin via the host's "navigate" action.

## Test Cases

- Slug and tab name are correct
- Routes returns both routes
- Init loads command center data from DB
- Navigation (up/down) moves cursor correctly
- Complete todo updates status and pushes undo entry
- Dismiss todo (X) updates status and pushes undo entry
- Undo (u) restores previous state from undo stack
- Create todo (c) enters rich mode
- Enter on todo with session_id returns launch action with resume_id
- Enter on todo with project_dir returns launch action
- Enter on todo without project_dir navigates to sessions
- Sub-view switching between command and threads
- Thread navigation works independently
- Thread pause/start/close operations
- Defer (d) moves todo to bottom
- Promote (p) moves todo to top
- Shift+up/down swaps todo with neighbor, persists via DB sort_order swap
- Toggle backlog (b) shows/hides completed items
- Booking mode enter/exit and duration selection
- View renders without panic (with and without data)
- Help overlay toggles
- HandleMessage processes async results
- Add thread creates new thread
- Close thread updates status
- Expanded view navigation (left/right columns)
