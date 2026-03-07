# SPEC: TUI Package (internal/tui)

## Purpose

The main Terminal User Interface for the Claude Command Center. Provides a 4-tab Bubbletea application for managing sessions, todos, threads, and calendar events. Adapted from AI-RON's airon-tui with all hardcoded values replaced by configuration.

## Interface

- **Inputs**: `*config.Config` and `*sql.DB` (passed to `NewModel`)
- **Outputs**: `Model` implementing `tea.Model` (Init/Update/View); `LaunchAction` set when user selects a session to launch
- **Dependencies**: `internal/config` (Config, Palette, CalendarEntry), `internal/db` (all types + DB operations), Bubbletea framework

## Architecture

### Files

- `model.go` -- Main model struct, NewModel constructor, Init/Update/View, all input handlers
- `styles.go` -- Styles struct derived from config.Palette (all colors configurable)
- `effects.go` -- Animation: tick messages, gradient interpolation, fade-in, pulsing pointer
- `banner.go` -- ASCII art banner with animated gradient, subtitle from config name
- `cc_view.go` -- Command center view: calendar panel, todo panel, warnings, suggestions, help overlay
- `threads_view.go` -- Threads tab: active/paused sections with type prefixes
- `items.go` -- List item types (newItem, sessionItem) and delegate renderer
- `launch.go` -- LaunchAction type and RunClaude function
- `claude_exec.go` -- Background claude CLI commands (edit, enrich, command, focus), prompt builders
- `refresh.go` -- Background refresh command (finds ccc-refresh binary)

### Key Design Decisions

1. **Colors from palette** -- No hardcoded color constants. All colors derived from `config.Palette` via `NewStyles()`.
2. **Calendar colors from config** -- Calendar event styling uses `config.CalendarEntry.Color` field instead of hardcoded calendar IDs.
3. **Config name in prompts** -- Claude command prompts use `config.Name` instead of hardcoded "AI-RON".
4. **CCC_STATE_DIR** -- Task context files written to CCC_STATE_DIR (not AIRON_STATE_DIR).
5. **Refresh binary** -- Looks for `ccc-refresh` next to the running executable, then falls back to PATH.
6. **DB-only** -- Sessions loaded from DB (DBLoadBookmarks), not winddown files. Paths from DBLoadPaths.
7. **No Aaron-specific content** -- Removed "lunch with becca" soft conflict detection, hardcoded calendar IDs, Aaron-specific paths.

## Behavior

### Tabs

1. **New Session** -- List of learned paths + Browse. Enter launches Claude in that dir. Delete removes path.
2. **Resume** -- List of bookmarked sessions. Enter resumes with `-r sessionID`.
3. **Command Center** -- Calendar + todo panels. Navigate todos, mark done/dismiss/defer/promote, create via rich textarea, view detail, schedule bookings.
4. **Threads** -- Active and paused thread sections. Pause/start/close/add threads.

### Animation

- 18 FPS tick drives gradient shimmer on banner, fade-in on startup, pulsing pointer on selected items.
- Gradient uses three configurable color stops (GradStart/GradMid/GradEnd) from palette.

### Claude Integration

- `c` key opens rich textarea. ctrl+d submits to Claude CLI for todo creation/completion.
- Space on a todo opens detail view with edit input.
- Focus suggestion auto-refreshes after todo mutations.
- All Claude calls run as background tea.Cmd (non-blocking).

### Refresh

- Auto-refresh triggers when data is older than 5 minutes.
- Manual refresh via `r` key.
- Spawns ccc-refresh binary, then reloads from DB.

## Test Cases

- NewModel creates model with correct config name and initial tab
- Tab navigation cycles through all 4 tabs and wraps
- Window resize updates dimensions
- View renders without panic for all tabs (nil cc and loaded cc)
- ccLoadedMsg updates model state
- Styles generated for all built-in palettes
- Gradient color interpolation produces valid hex
- extractJSON handles raw JSON, fenced JSON, and embedded JSON
- subtitleFromName generates spaced uppercase from config name
- formatDuration renders compact time strings
- truncateToWidth and flattenTitle work correctly
- calendarColorMap builds lookup from config entries
