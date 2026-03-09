# SPEC: Refresh Package

## Purpose
Fetches data from multiple external sources (Google Calendar, Gmail, GitHub, Slack, Granola), uses LLM extraction to identify action items and commitments, merges fresh data with existing state, and saves the updated command center state to SQLite.

## Interface

- **Input**: `Options` struct configuring the refresh run
  - `Verbose bool` — enable verbose logging
  - `DryRun bool` — print JSON to stdout instead of writing
  - `DB *sql.DB` — open SQLite database connection
  - `Sources []DataSource` — list of data sources to fetch from
  - `LLM llm.LLM` — LLM for post-merge suggestion generation
- **Output**: `error` (nil on success)
- **Entry point**: `refresh.Run(opts Options) error`
- **Dependencies**: Google OAuth2 tokens (Calendar, Gmail), Slack bot token, Granola stored auth, `gh` CLI, `claude` CLI

### DataSource Interface

See `specs/core/datasource.md` for full details. Each source implements `Name()`, `Enabled()`, and `Fetch(ctx)` methods. Per-source config (CalendarIDs, GitHubRepos, etc.) lives on source structs, not on `Options`. Auth loading happens inside each source's `Fetch()`.

## Behavior

1. Load env vars from `~/.config/ccc/.env` (for launchd environments)
2. Load existing state from SQLite via `db.LoadCommandCenterFromDB(opts.DB)`
3. Migrate calendar credentials if needed (one-time)
4. **Parallel data fetch**: Iterate `opts.Sources`; for each enabled source, spawn a goroutine calling `Fetch(ctx)`. Each source loads its own auth; auth failures produce warnings, not fatal errors. LLM extraction for Slack/Granola happens inside `Fetch()`.
5. **Combine results**: Merge all `SourceResult` values into a single `FreshData` (calendar from first non-nil, todos/threads concatenated)
6. **Merge**: Combine fresh data with existing state preserving IDs, statuses, dismissed items, manual items, and pause states
7. **Execute pending actions**: Process booking requests by creating calendar events in free slots (loads calendar auth independently)
8. **Generate suggestions**: LLM-based priority ranking of todos (if `opts.LLM` is non-nil)
9. Save merged state to SQLite via `db.DBSaveRefreshResult(opts.DB, merged)` (or print to stdout if DryRun)

## Types

Types are consolidated in `internal/db/` as the single source of truth. The refresh package imports from `internal/db` rather than maintaining its own duplicated type definitions in `refresh/types.go`.

## Locking

Refresh locking is implemented in `internal/refresh/lock.go`:

- `AcquireLock(stateDir string)` — acquires a PID lockfile to prevent concurrent refresh runs
- `IsLocked(stateDir string) bool` — checks whether a refresh is currently in progress (used by TUI to skip spawning refresh if one is already running)

The lockfile lives at `~/.config/ccc/data/refresh.lock`. Stale locks are detected by checking if the PID is still alive.

## Configurable Refresh Interval

The refresh interval is configurable via `config.yaml`:

```yaml
refresh_interval: "10m"  # default: "5m", minimum: "1m"
```

`Config.ParseRefreshInterval()` parses the duration string, returning `DefaultRefreshInterval` (5m) if the string is empty, unparseable, or less than 1 minute.

The CC plugin reads this at `Init()` and uses it for:
- Background auto-refresh timer
- Stale data detection on startup

## Refresh Status Indicator

The CC footer shows refresh status:
- **Normal**: "refreshed Xm ago" (muted)
- **Refreshing**: "refreshing..." with animated dots (cyan)
- **Error**: "refresh failed: ..." (red, truncated to 60 chars)

Fields on Plugin struct: `lastRefreshAt time.Time`, `lastRefreshError string`.

## Auto-Refresh on Startup

During `Init()`, after loading CC from DB, if `GeneratedAt` is older than the configured refresh interval, an auto-refresh is triggered via `StartCmds()`. This handles the common case of launching CCC after machine sleep.

## ccc-refresh Binary

A standalone binary at `cmd/ccc-refresh/main.go` provides the CLI entrypoint for refresh.

**Flags:**
- `-v` — verbose logging
- `--dry-run` — print result to stdout instead of writing to DB
- `--no-llm` — skip LLM calls (data-only refresh)

This binary is what launchd/cron invokes on schedule, and what the TUI spawns when the user presses `r`.

## Data Sources

| Source | Auth | Data |
|--------|------|------|
| Google Calendar | OAuth2 token from `~/.config/google-calendar-mcp/` | Today/tomorrow events from configured calendar IDs |
| Gmail | OAuth2 token from `~/.gmail-mcp/work.json` | Unread emails from last 3 days |
| GitHub | `gh` CLI auth | Open PRs authored by user, with review comment counts |
| Slack | `SLACK_BOT_TOKEN` env var | Messages with commitment language + thread context |
| Granola | Token from Electron app cache | This week's meetings with transcripts |

## Merge Rules

- **Calendar**: Replaced entirely each refresh
- **Todos**: Matched by `source_ref`; dismissed = tombstone (never recreated); existing items preserve ID/status/created_at while updating title/detail/context; new items get generated IDs; manual items always preserved
- **Threads**: Matched by URL; completed/dismissed never recreated; paused state preserved; summary updated from fresh data
- **PendingActions**: Preserved from existing state

## Test Cases

- Calendar replaced entirely on merge
- Dismissed todo never recreated from fresh data
- Existing todo updated (preserves ID, status, created_at)
- New todo gets generated ID and "active" status
- Manual todos preserved across merges
- Dismissed thread not recreated
- Paused thread state preserved, summary updated
- Pending actions preserved
- Nil existing state handled gracefully

## Key Changes from AI-RON Original

- Package `refresh` (not `main`); exposes `Run(opts Options) error`
- GitHub repos come from source struct config, not hardcoded
- Calendar supports multiple IDs via CalendarSource config
- Auto-accept is configurable via CalendarSource.AutoAcceptDomains (not hardcoded to @thanx.com)
- Env file reads from `~/.config/ccc/.env` instead of `~/.airon-env`
- State stored in SQLite (via `internal/db`) instead of `command-center.json`
- DataSource interface replaces hardcoded goroutines — each source owns its auth, enablement, and fetching
- LLM extraction for Slack/Granola happens inside each source's Fetch(), not as a separate phase
- LLM prompts use generic "user" language instead of "Aaron"
