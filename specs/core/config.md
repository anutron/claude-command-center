# SPEC: Configuration System

## Purpose

Provides user-configurable settings for the CCC TUI, including color palettes, data source toggles, agent governance, and file path conventions. Centralizes all path resolution (config dir, data dir, DB path, credentials) with environment variable overrides. Also manages onboarding concerns: shell hook installation, skill symlinks, MCP server builds, crontab scheduling, and credential validation.

## Interface

- **Inputs**: YAML config file at `ConfigPath()`, environment variables `CCC_CONFIG_DIR` and `CCC_STATE_DIR`
- **Outputs**: `*Config` struct, `Palette` struct, resolved file paths
- **Dependencies**: `gopkg.in/yaml.v3`

## Behavior

### Config Loading
- `Load()` reads `ConfigPath()` and unmarshals YAML into a `Config` struct
- If the file does not exist, returns `DefaultConfig()` without error
- Other read errors are returned as-is
- `Load()` unmarshals into `DefaultConfig()`, so omitted fields retain their defaults (e.g. AgentConfig governance values)
- Sets internal `loadedFromFile = true` and stores `originalContent` for regression detection on save

### Config Saving (Atomic Write with Safety Checks)

`Save(cfg, force ...bool)` writes config to `ConfigPath()` with multiple safety layers:

1. **Default-overwrite guard**: If the config was not loaded from a file (`loadedFromFile == false`), refuses to overwrite an existing config file. Prevents data loss when a load error causes the in-memory config to be defaults.
2. **Regression detection**: Re-reads the existing file from disk and compares key fields. Rejects the save if it would:
   - Reset `Name` from a custom value back to the default ("Claude Command")
   - Drop all external plugins when the existing file has some
   - Disable a calendar source that has configured calendar entries
   - Drop all automations when the existing file has some
3. **Backup**: Creates `config.yaml.bak` from the existing file before writing
4. **Atomic write**: Writes to `config.yaml.tmp` then renames to `config.yaml`. Falls back to direct write if rename fails.
5. **Force flag**: When `force` is true (used by Settings UI saves), skips the regression detection check but still creates a backup

After a successful save, updates `loadedFromFile = true` and refreshes `originalContent`.

`MarkLoadedFromFile()` marks a default config as safe to save (used after initial onboarding save).

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
- Calendar, GitHub, Granola, Slack, Gmail: disabled
- No custom colors, no external plugins, no automations
- Agent: see AgentConfig section below
- Daemon: RefreshInterval "5m", SessionRetention "7d"
- Refresh: enabled by default (nil `Enabled` treated as true)

### AgentConfig

Controls Claude Code agent behavior and governance. All fields have sensible defaults set in `DefaultConfig()`.

**Session defaults:**
- `DefaultBudget`: $5.00 per agent session
- `DefaultPermission`: "default"
- `DefaultMode`: "normal"
- `MaxConcurrent`: 10 simultaneous agent sessions

**Sandbox settings:**
- `TodoWriteLearnedPaths`: defaults to true (nil = true). When true, agents can write to learned session paths.
- `TodoExtraWritePaths`: additional paths agents may write to (empty by default)
- `AutonomousAllowedDomains`: domains agents can access autonomously. Defaults to `["github.com", "api.github.com"]`.

**Budget caps (governance — prevents runaway spend):**
- `HourlyBudget`: $25.00 max per rolling hour
- `DailyBudget`: $100.00 max per rolling 24 hours
- `BudgetWarningPct`: 0.80 — warn at 80% of budget

**Rate limiting and backoff:**
- `MaxLaunchesPerAutomationPerHour`: 20
- `CooldownMinutes`: 15 — pause after budget hit
- `FailureBackoffBaseSec`: 60 — initial backoff on failure
- `FailureBackoffMaxSec`: 3600 — max backoff cap (1 hour)

### DaemonConfig

Controls the CCC daemon process (background service):
- `RefreshInterval`: how often the daemon triggers a refresh (default "5m")
- `SessionRetention`: how long to keep session data (default "7d")

### RefreshConfig

Controls `ai-cron` behavior:
- `Enabled`: defaults to true when omitted (nil = true, for backwards compatibility). `RefreshEnabled()` accessor handles the nil check.
- `Model`: LLM model for ai-cron prompt generation. Empty string = use CLI default.

### DisabledPlugins

A list of built-in plugin slugs the user has turned off (e.g. `["sessions", "commandcenter"]`).

- `PluginEnabled(slug)` checks both `DisabledPlugins` (for built-ins) and `ExternalPlugins` entries (for externals). Returns true if not in either disabled list.
- `SetPluginEnabled(slug, enabled)` adds/removes a slug from the `DisabledPlugins` list. Idempotent.

### Display Options

- `ShowBanner`: defaults to true (nil = true). `BannerVisible()` accessor.
- `BannerTopPadding`: blank lines above banner, defaults to 2 (nil = 2). Clamped to >= 0 on set.
- `Subtitle`: optional subtitle text below the banner name.

### RefreshInterval (top-level)

- `RefreshInterval` field on Config (not to be confused with `Daemon.RefreshInterval` which controls the daemon)
- `ParseRefreshInterval()` parses the duration string; returns `DefaultRefreshInterval` (5 minutes) if empty, unparseable, or below 1 minute minimum.

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

### Shell Hook (`internal/config/shell.go`)

Manages a zsh shell hook that auto-launches CCC on interactive shell startup.

**Hook behavior:** The snippet is appended to `~/.zshrc` between sentinel comments (`# BEGIN CCC` / `# END CCC`). When an interactive shell starts, the hook skips CCC if any of these conditions are true:
- `$CLAUDE_CODE` or `$CLAUDECODE` is set (direct Claude Code child process)
- `$CCC_SKIP` is set (user override)
- Any ancestor process in the process tree has "claude" in its command name (catches agent-spawned terminals where env vars aren't inherited, e.g., new iTerm2 windows opened by Claude Code's Agent tool)

If none of these skip conditions apply, it runs `ccc`. After CCC exits, if a `last-dir` file exists, the shell `cd`s into that directory and removes the file.

- `IsShellHookInstalled()`: checks `~/.zshrc` for the sentinel comment. Returns false if file doesn't exist.
- `InstallShellHook()`: appends the hook snippet to `~/.zshrc` if not already present. Creates the file if missing. Idempotent.
- `UninstallShellHook()`: removes everything between sentinel comments (inclusive), cleaning surrounding newlines. No-op if `~/.zshrc` doesn't exist or hook is absent. If only the begin marker is found but not the end marker, treats the hook as malformed and leaves the file untouched.

### Skills Management (`internal/config/skills.go`)

Manages symlinks from the CCC repo's `.claude/skills/` directory into the user's `~/.claude/skills/` directory.

- `SkillNames()`: lists entries in the repo's `.claude/skills/` directory (found via Repo Path Resolution). Returns nil if the directory is not found.
- `IsSkillInstalled(name)`: checks whether `~/.claude/skills/<name>` exists (via `os.Lstat`, so it detects broken symlinks too).
- `InstallSkills()`: symlinks every skill from the repo dir to `~/.claude/skills/`. Removes existing symlinks before creating new ones. Creates `~/.claude/skills/` if missing. Errors if repo skills dir not found.
- `UninstallSkills()`: removes symlinks from `~/.claude/skills/` only if each symlink points to the expected repo path. Does not remove symlinks that point elsewhere (e.g. installed by another project).

### MCP Server Management (`internal/config/setup.go`)

Manages building and configuring MCP servers. Currently only the Gmail server is supported.

- `IsMCPBuilt()`: returns a `map[string]bool` indicating whether each server's `dist/index.js` exists in the `servers/` directory (found via Repo Path Resolution). Currently checks: `["gmail"]`.
- `GenerateMCPConfig()`: writes MCP server entries to `~/.claude/mcp.json` (0o600 permissions). Merges into existing `mcp.json` — preserves other `mcpServers` entries. Each entry specifies `command: "node"` with the path to the server's `dist/index.js`. Errors if no built servers found.
- `BuildAndConfigureMCP()`: end-to-end onboarding helper. Requires `node` on PATH. For each server, if `dist/index.js` doesn't exist, runs `npm install && npm run build` in the server directory. Then calls `GenerateMCPConfig()`. Returns the list of configured server names.

**Context note:** These functions are onboarding/setup utilities. Normal CCC runtime does not call them — they are invoked by `ccc setup` or the onboarding flow.

### Crontab Schedule (`internal/config/schedule.go`)

Manages a crontab entry for the `ai-cron` refresh binary. Uses crontab instead of launchd to avoid macOS "Background Items Added" notifications on every binary rebuild.

- `IsScheduleInstalled()`: checks `crontab -l` output for the marker comment `# ai-cron schedule`
- `InstallSchedule()`: builds a crontab entry using the configured `RefreshInterval` (minimum 1 minute). Sources `ConfigDir()/.env` before running the binary. Logs output to `DataDir()/refresh.log`. Replaces any existing `ai-cron` entry. Also cleans up legacy launchd plists.
- `UninstallSchedule()`: removes the `ai-cron` crontab entry. If the crontab becomes empty, removes the crontab entirely. Also cleans up legacy launchd plists.

### Credential Validation (`internal/config/validate.go`)

Validates that external service credentials are present and parseable. Used by the onboarding flow and doctor checks.

- `ValidateCalendar()`: checks that `~/.config/google-calendar-mcp/credentials.json` exists and is valid JSON
- `ValidateGitHub()`: runs `gh auth token` to verify GitHub CLI authentication
- `ValidateSlack()`: checks that `LoadSlackToken()` returns a non-empty string
- `ValidateGmail()`: checks that `~/.gmail-mcp/work.json` exists and is valid JSON
- `ValidateGranola()`: checks that `~/Library/Application Support/Granola/stored-accounts.json` exists

`LoadSlackToken()` precedence: Config `Token` > Config `BotToken` (deprecated) > `$SLACK_TOKEN` > `$SLACK_BOT_TOKEN` (deprecated).

**Note on placeholder detection:** OAuth client ID placeholder detection (rejecting values like "placeholder", "test", "CLIENT_ID") lives in `internal/auth/flow.go`, not in the config package. See the auth spec for details.

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
- SaveRefusesDefaultsOverCustomConfig: regression detection blocks overwriting custom name, external plugins
- SaveAllowsLegitimateChanges: editing palette on a loaded config succeeds
- SaveRoundTripPreservesAllFields: load-then-save does not lose data
- RegressionDetectsDroppedAutomations: refuses to save when automations would be lost
- GetPalette: all 5 palettes exist, unknown falls back to aurora
- ConfigPaths: env vars override defaults
- CustomPalette: "custom" with colors uses them; "custom" without colors falls back to aurora
- ParseRefreshInterval: empty/invalid/below-minimum all return default 5m; valid durations parse correctly
- DaemonConfigDefaults: RefreshInterval "5m", SessionRetention "7d"
- AgentConfig_SandboxDefaults: TodoWriteLearnedPaths defaults true, AutonomousAllowedDomains populated
- AgentConfig_GovernanceDefaults: all budget/rate fields have correct defaults
- AgentConfig_GovernanceYAMLRoundTrip: custom governance values survive save/load
- AgentConfig_GovernanceDefaultsWhenNotConfigured: minimal YAML without agent section still gets governance defaults
- AutomationConfigRoundTrip: automations with settings survive save/load
- IsShellHookInstalled: false when no .zshrc, false without hook, true with hook
- InstallShellHook: appends to .zshrc, idempotent on second call
- UninstallShellHook: removes hook block, no-op when file missing
- IsSkillInstalled: false for missing, true for existing
- SkillNames: returns nil when dir missing, lists entries when present
- ValidateCalendar: errors when credentials missing
- ValidateGitHub: errors when gh CLI not authenticated
- ValidateSlack: errors when no token available
- ValidateGmail: errors when missing, errors when malformed, succeeds when valid
- ValidateGranola: errors when accounts missing
- IsScheduleInstalled: does not panic when no crontab

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
agent:
  default_budget: 5.0
  max_concurrent: 10
  hourly_budget: 25.0
  daily_budget: 100.0
  budget_warning_pct: 0.8
  cooldown_minutes: 15
daemon:
  refresh_interval: "5m"
  session_retention: "7d"
refresh:
  enabled: true
  model: ""
disabled_plugins:
  - commandcenter
automations:
  - name: daily-review
    command: "claude -p 'review todos'"
    enabled: true
    schedule: "0 9 * * *"
    config_scopes:
      - todos
      - calendar
```
