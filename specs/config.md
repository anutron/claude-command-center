# SPEC: Configuration System

## Purpose

Provides user-configurable settings for the CCC TUI, including color palettes, data source toggles, and file path conventions. Centralizes all path resolution (config dir, data dir, DB path, credentials) with environment variable overrides.

## Interface

- **Inputs**: YAML config file at `ConfigPath()`, environment variables `CCC_CONFIG_DIR` and `CCC_STATE_DIR`
- **Outputs**: `*Config` struct, `Palette` struct, resolved file paths
- **Dependencies**: `gopkg.in/yaml.v3`

## Behavior

### Config Loading
- `Load()` reads `ConfigPath()` and unmarshals YAML into a `Config` struct
- If the file does not exist, returns `DefaultConfig()` without error
- Other read errors are returned as-is

### Config Saving
- `Save()` marshals the config to YAML and writes to `ConfigPath()`
- Creates the config directory if it doesn't exist

### Path Resolution
- `ConfigDir()`: `$CCC_CONFIG_DIR` or `~/.config/ccc`
- `ConfigPath()`: `ConfigDir()/config.yaml`
- `DataDir()`: `$CCC_STATE_DIR` or `ConfigDir()/data`
- `DBPath()`: `DataDir()/ccc.db`
- `CredentialsDir()`: `ConfigDir()/credentials`

### Default Config
- Name: "Command Center"
- Palette: "aurora"
- Todos: enabled
- Calendar, GitHub, Granola: disabled
- No custom colors, no external plugins

### Palettes
- 5 built-in palettes: aurora, ocean, ember, neon, mono
- Each palette has 14 color fields (Fg, Highlight, SelectedBg, Pointer, Muted, Cyan, Yellow, White, Purple, Green, GradStart, GradMid, GradEnd, BgDark)
- `GetPalette(name, customColors)` returns the named palette, falls back to aurora for unknown names
- When name is "custom" and `CustomColors` is provided, builds a palette from Primary/Secondary/Accent
- `PaletteNames()` returns all built-in palette names

## Test Cases

- DefaultConfig: correct defaults (name, palette, todos enabled, others disabled)
- LoadMissingFile: returns default config, no error
- SaveAndLoad: round-trip preserves all fields
- GetPalette: all 5 palettes exist, unknown falls back to aurora
- ConfigPaths: env vars override defaults
- CustomPalette: "custom" with colors uses them; "custom" without colors falls back to aurora

## Examples

```yaml
# ~/.config/ccc/config.yaml
name: My Dashboard
palette: ocean
calendar:
  enabled: true
  calendars:
    - id: work@example.com
      label: Work
      color: "#ff6b6b"
github:
  enabled: true
  repos:
    - owner/repo1
  username: myuser
todos:
  enabled: true
granola:
  enabled: false
```
