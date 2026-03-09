# SPEC: TUI Host (internal/tui)

## Purpose

The thin host shell for the Claude Command Center. Manages the Bubbletea application lifecycle, tab bar, banner animation, and plugin dispatch. Contains no domain logic — all functionality lives in plugins.

## Interface

- **Inputs**: `*sql.DB`, `*config.Config`, `plugin.EventBus`, `plugin.Logger` (passed to `NewModel`); optional external plugins
- **Outputs**: `Model` implementing `tea.Model` (Init/Update/View); `LaunchAction` set when a plugin requests a session launch
- **Dependencies**: `internal/config`, `internal/plugin`, `internal/builtin/sessions`, `internal/builtin/commandcenter`, `internal/builtin/settings`, Bubbletea framework

## Architecture

### Files

- `model.go` — Main model struct, plugin wiring, Init/Update/View, action dispatch (~240 lines)
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
    db        *sql.DB
    // Direct references for cross-plugin communication
    sessionsPlugin      *sessions.Plugin
    commandCenterPlugin *commandcenter.Plugin
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
7. Wire tab entries to plugins and routes (settings tab at the end)
8. In `Init()`, start animation tick and plugin startup commands

### Input Dispatch

- **Tab/Shift+Tab**: Cycle active tab, call `NavigateTo(route)` on the newly active plugin
- **Esc**: Offer to active plugin first; if plugin returns "unhandled" or "quit", exit the TUI
- **All other keys**: Forward to active plugin's `HandleKey`, process the returned action

### Message Broadcast

Non-key messages (ticks, window resize, custom plugin messages) are broadcast to all unique plugins via `HandleMessage`. Each plugin slug is visited once.

### Action Processing

Plugins return `plugin.Action` values. The host processes them:

| Action Type | Host Behavior |
|-------------|---------------|
| `launch` | Build `LaunchAction` from args (dir, resume_id, prompt), quit TUI |
| `quit` | Quit TUI |
| `navigate` | Switch to target tab, activate plugin route |
| `unhandled` | Quit TUI (esc fallthrough) |
| `noop` | Execute `TeaCmd` if present, otherwise no-op |

### Cross-Plugin Communication

The host holds direct references to the sessions and command center plugins for cases where the event bus is insufficient (e.g., passing a pending launch todo from CC to sessions before navigating).

### Rendering

1. Banner with animated gradient (top)
2. Tab bar with active tab highlighted (center-aligned)
3. Active plugin's `View()` output (below tab bar)
4. Centered in terminal via `lipgloss.Place`

### Animation

- 18 FPS tick drives gradient shimmer on banner, fade-in on startup, pulsing pointer on selected items
- Gradient uses three configurable color stops (GradStart/GradMid/GradEnd) from palette

## Key Design Decisions

1. **No domain logic in host** — The host knows nothing about todos, threads, calendar, or sessions. It only knows about tabs, plugins, and actions.
2. **Colors from palette** — No hardcoded color constants. All colors derived from `config.Palette` via `NewStyles()`.
3. **Multiple tabs per plugin** — A single plugin can power multiple tabs via different routes.
4. **Plugin registration order** — Tab order is defined by the host, not the plugins.

## Test Cases

- NewModel creates model with correct config name and initial tab
- Tab navigation cycles through all tabs and wraps
- Window resize updates dimensions
- View renders without panic
- Styles generated for all built-in palettes
- Gradient color interpolation produces valid hex
- subtitleFromName generates spaced uppercase from config name
- Esc key quits the TUI
- Tab entries map to correct plugins
