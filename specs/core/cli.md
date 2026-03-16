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

Adds a crontab entry for scheduled background refresh. Uses crontab instead of launchd to avoid macOS "Background Items Added" notifications that re-trigger on every binary rebuild.

- Crontab entry with `# ccc-refresh schedule` marker comment
- Interval from `config.refresh_interval` (default 5m), converted to cron `*/N * * * *`
- Sources `~/.config/ccc/.env` before running (for API keys in cron environment)
- Logs to `~/.config/ccc/data/refresh.log`
- Cleans up legacy launchd plist (`~/Library/LaunchAgents/com.ccc.refresh.plist`) if present
- Idempotent: skips if identical entry already exists, replaces if entry differs

### `ccc uninstall-schedule`

Removes the ccc-refresh crontab entry.

- Removes lines containing the `# ccc-refresh schedule` marker
- Clears crontab entirely if no other entries remain
- Also cleans up legacy launchd plist if present
- No-op if no schedule is installed

### `ccc notify [event]`

Sends a notification event to all running CCC instances, causing them to reload from DB.

- Scans `~/.config/ccc/data/` for `ccc-*.sock` files
- Connects to each unix socket and sends the event string (default: "reload")
- Stale sockets (connection refused) are automatically cleaned up
- Prints count of instances notified
- Useful for external scripts (e.g., after ccc-refresh runs in launchd)

### `ccc update-todo`

Updates fields on an existing todo. Designed for headless agents to submit structured session summaries.

- Flags: `--id` (required), `--session-summary`, `--session-status`
- `--session-summary -` reads summary text from stdin (for long summaries)
- Calls `tui.SendNotify("reload")` after update so all running CCC instances refresh
- Exits with error if `--id` is empty

### `ccc sessions`

Alias for default (launches TUI).

### `ccc help` / `ccc -h` / `ccc --help`

Prints usage information.

## Test Cases

- Doctor: all checks return DoctorCheck with correct OK/fail states
- Doctor: missing config → `[!!]` with "run 'ccc setup'" message
- Doctor: stale data (>30m) → `[!!]` with age warning
- Schedule: crontab entry contains binary path, interval, and marker comment
- Schedule: uninstall with no entry → prints "No schedule installed"
- Schedule: install cleans up legacy launchd plist if present
- Config: ParseRefreshInterval with valid durations
- Config: ParseRefreshInterval with empty/invalid → returns default
- Config: ParseRefreshInterval with <1m → returns default
- Notify: socket path contains PID and ends with .sock
- Notify: socket path respects CCC_STATE_DIR env var
- Notify: SendNotify with no instances returns error
- Notify: SendNotify reaches a listening socket
- Notify: stale socket files are cleaned up on failed connection
