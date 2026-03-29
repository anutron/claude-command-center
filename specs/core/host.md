# SPEC: TUI Host (internal/tui)

## Purpose

The thin host shell for the Claude Command Center. Manages the Bubbletea application lifecycle, tab bar, banner animation, and plugin dispatch. Contains no domain logic — all functionality lives in plugins.

## Interface

- **Inputs**: `*sql.DB`, `*config.Config`, `plugin.EventBus`, `plugin.Logger`, `llm.LLM` (passed to `NewModel`); optional external plugins via variadic `extPlugins`
- **Outputs**: `Model` implementing `tea.Model` (Init/Update/View); `LaunchAction` set when a plugin requests a session launch
- **Dependencies**: `internal/config`, `internal/plugin`, `internal/builtin/sessions`, `internal/builtin/commandcenter`, `internal/builtin/settings`, Bubbletea framework

## Architecture

### Files

- `model.go` — Main model struct, plugin wiring, Init/Update/View, action dispatch
- `styles.go` — Styles struct derived from `config.Palette` (all colors configurable)
- `effects.go` — Animation: tick messages, gradient interpolation, fade-in, pulsing pointer
- `banner.go` — ASCII art banner with animated gradient, subtitle from config name
- `launch.go` — `LaunchAction` type and `RunClaude` function

### Model

```go
type Model struct {
    cfg       *config.Config
    styles    Styles
    grad      GradientColors
    tabs      []tabEntry       // label + plugin + route
    activeTab tab
    width, height, frame int
    Launch    *LaunchAction
    allPlugins []plugin.Plugin  // every unique plugin for lifecycle management
    returnedFromLaunch bool     // set when TUI restarts after a Claude session
    db        *sql.DB
}
```

### Tab Entries

Each tab maps a label to a plugin and a route within that plugin. Multiple tabs can reference the same plugin with different routes:

| Tab | Plugin | Route |
|-----|--------|-------|
| New Session | sessions | `new` |
| Resume | sessions | `resume` |
| Command Center | commandcenter | `commandcenter` |
| Threads | commandcenter | `commandcenter/threads` |
| *(external plugin tabs)* | *(external)* | *(plugin-defined)* |
| Settings | settings | `settings` |

## Behavior

### Initialization

1. Build styles and gradient colors from the config palette
2. Create plugin instances (sessions, command center)
3. Build a `plugin.Registry` with all plugins (built-in + external + settings)
4. Create settings plugin with registry reference: `settings.New(registry)`
5. Create shared plugin context with the **shared bus and logger from main.go** (not a local bus — this ensures all plugins communicate via the same event bus)
6. Call `Init(ctx)` on each plugin
7. Wire tab entries to plugins and routes (external plugin tabs before settings, settings always last)
8. Collect all unique plugins into `allPlugins` for lifecycle management
9. In `Init()`, start animation tick, plugin startup commands (`Starter` interface), initial data load, and emit `ReturnMsg` if `returnedFromLaunch` is set

### Input Dispatch

- **Tab/Shift+Tab**: Cycle active tab; sends `TabLeaveMsg` to previous plugin, calls `NavigateTo(route)` on new plugin, sends `TabViewMsg` to new plugin
- **Esc**: Offer to active plugin first; if plugin returns "unhandled" or "quit", exit the TUI
- **All other keys**: Forward to active plugin's `HandleKey`, process the returned action

### Message Broadcast

Non-key messages (ticks, window resize, `NotifyMsg`, custom plugin messages) are broadcast to all unique plugins via `HandleMessage`. Each plugin slug is visited once.

**Daemon events:** When a `DaemonEventMsg` arrives, the host routes it through the event bus AND broadcasts `plugin.NotifyMsg{Event: evt.Type}` to all plugins. This allows plugins to handle daemon events via `HandleMessage` (dispatching async tea.Cmds) instead of mutating state directly in event bus handlers, which would race with concurrent tea.Cmd goroutines.

### Action Processing

Plugins return `plugin.Action` values. The host processes them:

| Action Type | Host Behavior |
|-------------|---------------|
| `launch` | Build `LaunchAction` from args (dir, resume_id, initial_prompt), broadcast `LaunchMsg` to all plugins, quit TUI |
| `quit` | Quit TUI |
| `navigate` | Switch to target tab (`sessions` or `command`), activate plugin route |
| `unhandled` | Quit TUI (esc fallthrough) |
| `noop` | Execute `TeaCmd` if present, otherwise no-op |

### Cross-Plugin Communication

All cross-plugin communication uses the event bus exclusively. The host does not hold direct references to specific plugin types — it only interacts with plugins through the `plugin.Plugin` interface. The `allPlugins` slice holds every unique plugin instance for shutdown and lifecycle management.

### Rendering

1. Banner with animated gradient (top, with top padding)
2. Tab bar with active tab highlighted (center-aligned, `> label` format)
3. Active plugin's `View(width, contentHeight, frame)` output (below tab bar)
4. Centered in terminal via `lipgloss.Place`

**Content height calculation**: The host computes the overhead (banner + spacing + tab bar) by rendering the header sections and counting newlines, then passes `terminalHeight - overhead` as `contentHeight` to the plugin. This prevents plugins from sizing their layouts to the full terminal height and overflowing past the banner/tabs.

### Animation

- Tick-driven gradient shimmer on banner, fade-in on startup, pulsing pointer on selected items
- Gradient uses three configurable color stops (GradStart/GradMid/GradEnd) from palette

### Cross-Instance Notification

Multiple CCC instances share the same SQLite DB. A unix socket notification system keeps them in sync:

- Each TUI instance creates a PID-scoped socket at `~/.config/ccc/data/ccc-<PID>.sock`
- A goroutine listens for newline-delimited event strings on the socket
- Incoming events are injected as `plugin.NotifyMsg` into the bubbletea program via `p.Send()`
- Plugins handle `NotifyMsg` by reloading data from DB
- `ccc notify [event]` connects to all `ccc-*.sock` files and sends the event (default: "reload")
- Stale sockets (connection refused) are automatically cleaned up

### Shutdown & Error Handling

- **Shutdown**: Calls `Shutdown()` on every unique plugin (deduplicated by slug)
- **Database required**: If the database cannot be opened, the process exits with a clear error message
- **Signal handling**: SIGINT and SIGTERM trigger graceful shutdown — all external plugin subprocesses are cleaned up
- **Claude exit errors**: If `claude` exits non-zero, the error is printed to stderr but the TUI loop continues
- **RunClaude error propagation**: `launch.go:RunClaude()` returns errors from `cmd.Run()`
- **Interactive launch with InitialPrompt**: When `LaunchAction.InitialPrompt` is set, `RunClaude` passes the prompt via `--append-system-prompt` (persistent context across the session) and a short kickoff message as the positional prompt argument so Claude starts working immediately instead of waiting for user input

## Key Design Decisions

1. **No domain logic in host** — The host knows nothing about todos, threads, calendar, or sessions. It only knows about tabs, plugins, and actions.
2. **Colors from palette** — No hardcoded color constants. All colors derived from `config.Palette` via `NewStyles()`.
3. **Multiple tabs per plugin** — A single plugin can power multiple tabs via different routes.
4. **Plugin registration order** — Tab order is defined by the host, not the plugins.
5. **Event-bus-only communication** — No direct plugin-to-plugin references; all cross-plugin communication goes through the shared event bus.

## Test Cases

- NewModel creates model with correct config name and initial tab
- Tab navigation cycles through all tabs and wraps
- Tab switching sends TabLeaveMsg/TabViewMsg
- Window resize updates dimensions
- View renders without panic
- Styles generated for all built-in palettes
- Gradient color interpolation produces valid hex
- subtitleFromName generates spaced uppercase from config name
- Esc key quits the TUI
- Tab entries map to correct plugins
- allPlugins contains all unique plugin instances
- returnedFromLaunch emits ReturnMsg on Init
- Shutdown calls Shutdown on each unique plugin once
