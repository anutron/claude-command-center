# Settings Plugin

**Package:** `internal/builtin/settings`
**Slug:** `settings`
**Tab:** "Settings"

## Purpose

Provides a UI for managing plugins, data sources, logs, and color palettes.

## Sub-Views

### Plugins (`settings`)

Lists all registered plugins and data sources with enable/disable status.

**Two sections:**

1. **Plugins** — Things with tabs/UI:
   - Built-in (Sessions, Command Center, Settings) — always on, not toggleable
   - External — from `external_plugins` config, toggleable

2. **Data Sources** — Things that feed data during `ccc-refresh`:
   - Todos — always on, not toggleable
   - Calendar, GitHub, Granola — each toggleable

**Toggle behavior:**
- `space` toggles enable/disable on the selected item
- `enter` opens the detail view for the selected item
- External plugins: saves config, flashes "Restart CCC to apply"
- Data sources: when enabling, validates credentials first; reverts toggle on failure with error message; on success saves config, flashes "Changes apply on next refresh"

### Detail Views

Opened by pressing `enter` on any item in the plugins list.

**Core plugins** (Sessions, Command Center, Settings):
- Read-only status display (always enabled, no configuration)

**External plugins:**
- Name, command, and enable/disable status
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
