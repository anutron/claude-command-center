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

### HomeDir
- Optional `home_dir` config field specifying the default project directory
- On Sessions plugin Init, if set and not already in learned paths, it is prepended to the paths list and persisted to DB
- No special styling or behavior — treated as a regular path entry that can be reordered or deleted

### Default Config
- Name: "Claude Command"
- HomeDir: "" (empty, defaults to $HOME via ResolveHomeDir)
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

### Repo Path Resolution

Several features (MCP servers, project skills) need to locate directories relative to the repo root. The binary may be installed via symlink (e.g. `/usr/local/bin/ccc → /path/to/repo/ccc`).

Resolution order:
1. Resolve the current executable path, following symlinks with `filepath.EvalSymlinks`
2. Check for the target directory next to the resolved binary location
3. Fall back to checking the current working directory

This ensures the repo's `servers/` and `.claude/skills/` directories are found even when the binary is invoked via symlink from outside the repo.

## File Permissions

Sensitive files are written with restricted permissions to prevent local information disclosure:

- **`config.yaml`** and **`config.yaml.bak`**: `0o600` — contains Slack token in plaintext
- **`mcp.json`** (`~/.claude/mcp.json`): `0o600` — contains MCP server configuration
- **Database directory** (`~/.config/ccc/data/`): `0o700` — DB contains calendar events, email subjects, meeting transcripts
- **Socket directory**: `0o700` — unix socket allows sending notify events to the TUI
- **Lock file** (`refresh.lock`): `0o600`
- **Credentials directory** (`~/.config/ccc/credentials/`): follows parent directory permissions

Note: Go OAuth token files (`token.json`) already use `0o600`. The Gmail MCP TypeScript server writes tokens with `{ mode: 0o600 }`.

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
home_dir: /Users/me/Projects/my-project
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
