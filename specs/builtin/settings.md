# Settings Plugin

**Package:** `internal/builtin/settings`
**Slug:** `settings`
**Tab:** "Settings"

## Purpose

Provides a UI for managing plugins, data sources, logs, and color palettes. Uses a sidebar + content pane layout.

## Layout

Sidebar + content pane layout. The sidebar lists all items grouped by category (Appearance, Plugins, Data Sources, System). The content pane shows details for the selected item.

### NavItem

Each sidebar entry is a `NavItem` with:
- `Label` — display name
- `Slug` — unique identifier
- `Kind` — "appearance", "plugin", "datasource", "system"
- `Description` — short description shown below the title in the content pane
- `Enabled` — toggle state (nil = no toggle)
- `Valid` / `ValidHint` — credential validation status (data sources only)

Descriptions are hardcoded for built-in plugins and data sources. External plugins get descriptions from `config.ExternalPluginConfig.Description`.

### Content pane header

The content pane title (e.g., "POMODORO", "CALENDAR") has left padding to avoid touching the panel border. A description line in muted text appears below the title when available.

## Sub-Views

### Plugins (`settings`)

Lists all registered plugins and data sources with enable/disable status.

**Two sections:**

1. **Plugins** — Things with tabs/UI:
   - Built-in: Sessions and Command Center are toggleable; Settings is always on (not toggleable)
   - Disabling a built-in plugin hides its tabs from the tab bar (requires restart)
   - Disabled plugins stored in `config.DisabledPlugins` slug list
   - External — from `external_plugins` config, toggleable

2. **Data Sources** — Things that feed data during `ccc-refresh`:
   - Todos — always on, not toggleable
   - Calendar, GitHub, Granola, Slack — each toggleable

**Toggle behavior:**
- `space` toggles enable/disable on the selected item
- `enter` opens the detail view for the selected item
- Built-in plugins: saves to `config.DisabledPlugins`, flashes "Restart CCC to apply"
- External plugins: saves config, flashes "Restart CCC to apply"
- Data sources: when enabling, validates credentials first; reverts toggle on failure with error message; on success saves config, flashes "Changes apply on next refresh"

**Enabled-state sync:**
- The settings plugin syncs its displayed enabled states from the live `config.Config` at the start of each `View()` call
- This ensures that if another flow (e.g., onboarding) modifies config, settings reflects the current truth
- Without this, the settings items snapshot enabled state at Init() time and can show stale values

### Detail Views

Opened by pressing `enter` on any item in the plugins list.

**Settings plugin** (always-on core plugin):
- Read-only status display (always enabled, no configuration)

**External plugins:**
- Name, description (from config), command, and enable/disable status
- Space toggles enable/disable

**Calendar:**
- Credentials status (valid/missing/expired)
- Calendar list (configured calendar IDs)

**GitHub:**
- Credentials status (valid/missing)
- Username (editable with `u`)
- Repo list with add (`a`) and remove (`x`)

**Granola:**
- Credentials status (valid/missing/expired)

**Key bindings in detail view:**

| Key | Description |
|-----|-------------|
| esc | Back to plugin list |
| space | Toggle enable/disable |
| a | Add repo (GitHub detail only) |
| x | Remove selected repo (GitHub detail only) |
| u | Edit username (GitHub detail only) |

### Logs (`settings/logs`)

Shows recent log entries from `logger.Recent(100)` in reverse chronological order. Color-coded by level (error=red, warn=yellow, info=muted). Scrollable with up/down.

### Palette (`settings/palette`)

Shows all 5 built-in palettes with color swatches. Left/right to cycle, enter to apply and save. Publishes `settings.palette_changed` event.

## Key Bindings

| Key | Context | Description |
|-----|---------|-------------|
| up/down | all | Navigate |
| enter | plugins | Open detail view |
| space | plugins | Toggle enable/disable |
| l | any | Switch to logs |
| p | any | Switch to palette |
| s | any | Switch to plugins |
| left/right | palette | Cycle palettes |
| enter | palette | Apply + save |

## Constructor

```go
settings.New(registry *plugin.Registry) *Plugin
```

The registry is used to enumerate all plugins. Passed directly rather than via `plugin.Context`.

## Dependencies

- `plugin.Registry` — for listing all plugins
- `plugin.Logger` — for log viewer
- `plugin.EventBus` — for palette change events
- `config.Config` — for reading/writing toggle states and palette
