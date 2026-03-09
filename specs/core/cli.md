# SPEC: CLI Subcommands

## Purpose

CCC provides several CLI subcommands beyond the default TUI launcher for setup, diagnostics, and operational tasks.

## Subcommands

### `ccc` (default)

Launches the TUI dashboard. Requires a working database — exits with error if DB can't be opened.

### `ccc setup`

Interactive setup wizard that walks through:

1. Dashboard name and color palette
2. Calendar credentials check (prints OAuth setup instructions if missing)
3. GitHub CLI authentication check (prints `gh auth login` instructions if missing)
4. Granola configuration check
5. Saves config to `~/.config/ccc/config.yaml`

Loads existing config as defaults if one exists.

### `ccc doctor`

Diagnostic command that checks system health. Prints `[OK]` or `[!!]` per check with actionable fix instructions.

**Checks:**
1. Config file exists and parses
2. Database opens successfully
3. Calendar credentials present and valid
4. GitHub CLI authenticated (`gh auth token`)
5. Granola configured (stored-accounts.json exists)
6. `ccc-refresh` binary found (next to executable or on PATH)
7. `claude` CLI on PATH
8. Data freshness — warns if `generated_at` > 30 minutes stale

Exit code 0 if all pass, 1 if any fail.

### `ccc install-schedule`

Generates and loads a launchd plist for scheduled background refresh.

- Plist at `~/Library/LaunchAgents/com.ccc.refresh.plist`
- Interval from `config.refresh_interval` (default 5m)
- Logs to `~/.config/ccc/data/refresh.log`
- Runs `launchctl load` to activate
- `RunAtLoad: true` for immediate first run

### `ccc uninstall-schedule`

Unloads and removes the launchd plist.

- Runs `launchctl unload`
- Deletes the plist file
- No-op if no schedule is installed

### `ccc sessions`

Alias for default (launches TUI).

### `ccc help` / `ccc -h` / `ccc --help`

Prints usage information.

## Test Cases

- Doctor: all checks return DoctorCheck with correct OK/fail states
- Doctor: missing config → `[!!]` with "run 'ccc setup'" message
- Doctor: stale data (>30m) → `[!!]` with age warning
- Schedule: plist template generates valid XML with correct binary path and interval
- Schedule: uninstall with no plist → prints "No schedule installed"
- Config: ParseRefreshInterval with valid durations
- Config: ParseRefreshInterval with empty/invalid → returns default
- Config: ParseRefreshInterval with <1m → returns default
