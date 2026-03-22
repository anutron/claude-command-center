# SPEC: Refresh Package

## Purpose
Fetches data from multiple external sources (Google Calendar, Gmail, GitHub, Slack, Granola), uses LLM extraction to identify action items and commitments, merges fresh data with existing state, and saves the updated command center state to SQLite.

## Interface

- **Input**: `Options` struct configuring the refresh run
  - `Verbose bool` — enable verbose logging
  - `DryRun bool` — print JSON to stdout instead of writing
  - `DB *sql.DB` — open SQLite database connection
  - `Sources []DataSource` — list of data sources to fetch from
  - `LLM llm.LLM` — LLM for extraction and suggestions (haiku)
  - `RoutingLLM llm.LLM` — LLM for routing/validation (sonnet); falls back to `LLM` if nil
  - `ContextRegistry *ContextRegistry` — for fetching source context on todos
- **Output**: `error` (nil on success)
- **Entry point**: `refresh.Run(opts Options) error`
- **Dependencies**: Google OAuth2 tokens (Calendar, Gmail), Slack bot token, Granola stored auth, `gh` CLI, `claude` CLI

### DataSource Interface

See `specs/core/datasource.md` for full details. Each source implements `Name()`, `Enabled()`, and `Fetch(ctx)` methods. Per-source config (CalendarIDs, GitHubRepos, etc.) lives on source structs, not on `Options`. Auth loading happens inside each source's `Fetch()`.

## Behavior

1. Load env vars from `~/.config/ccc/.env` (for cron/non-interactive environments)
2. Load existing state from SQLite via `db.LoadCommandCenterFromDB(opts.DB)`
3. Migrate calendar credentials if needed (one-time)
4. **Parallel data fetch**: Iterate `opts.Sources`; for each enabled source, spawn a goroutine calling `Fetch(ctx)`. Each source loads its own auth; auth failures produce warnings, not fatal errors. LLM extraction for Slack/Granola happens inside `Fetch()`. See `specs/core/todo-extraction.md` for extraction rules.
5. **Combine results**: Merge all `SourceResult` values into a single `FreshData` (calendar from first non-nil, todos/threads concatenated)
6. **Merge**: Combine fresh data with existing state preserving IDs, statuses, dismissed items, manual items, and pause states
7. **Execute pending actions**: Process booking requests by creating calendar events in free slots (loads calendar auth independently)
8. **Generate suggestions**: LLM-based priority ranking of todos (if `opts.LLM` is non-nil)
9. **Generate proposed prompts**: Route eligible todos (active, has source, no prompt yet) using `RoutingLLM` (sonnet). The routing step validates ownership — a task is Aaron's if he committed to it OR if someone else assigned it to him by name (see `specs/core/todo-extraction.md`). If the LLM returns `project_dir: "REJECT"`, the todo is auto-dismissed. Otherwise, it assigns a project directory and generates an actionable prompt.
10. **Fetch source context**: For todos with a `source_ref`, fetch raw source content (transcripts, threads, PR comments) via `ContextRegistry` and cache in `source_context`/`source_context_at` columns.
11. Save merged state to SQLite via `db.DBSaveRefreshResult(opts.DB, merged)` (or print to stdout if DryRun)

## Types

Types are consolidated in `internal/db/` as the single source of truth. The refresh package imports from `internal/db` rather than maintaining its own duplicated type definitions in `refresh/types.go`.

## Locking

Refresh locking is implemented in `internal/lockfile/lockfile.go`:

- `AcquireLock(stateDir string)` — acquires an advisory file lock via `syscall.Flock()` to prevent concurrent refresh runs. Returns a release function on success, or `ErrAlreadyLocked` if another process holds the lock.
- `IsLocked(stateDir string) bool` — checks whether a refresh is currently in progress (used by TUI to skip spawning refresh if one is already running)

The lockfile lives at `~/.config/ccc/data/refresh.lock` with `0o600` permissions. The flock-based approach is atomic and eliminates the TOCTOU race condition of the previous PID-based implementation.

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

## ai-cron Binary

A standalone binary at `cmd/ai-cron/main.go` provides the CLI entrypoint for refresh.

**Flags:**
- `-v` — verbose logging
- `--dry-run` — print result to stdout instead of writing to DB
- `--no-llm` — skip LLM calls (data-only refresh)

This binary is what crontab invokes on schedule, and what the TUI spawns when the user presses `r`.

## Background Scheduling (crontab)

Background refresh uses crontab instead of launchd. macOS BTM (Background Task Management) tracks `executableModifiedDate` for launch agent binaries — every `make build` recompiles `ai-cron`, changes its mtime, and re-triggers the "Background Items Added" notification. Crontab bypasses BTM entirely.

The schedule is managed via `ccc install-schedule` and `ccc uninstall-schedule` (see `specs/core/cli.md`). Implementation lives in `internal/config/schedule.go`.

**Schedule entry format:**

```
*/N * * * * [ -f ~/.config/ccc/.env ] && . ~/.config/ccc/.env; /path/to/ai-cron >> ~/.config/ccc/data/refresh.log 2>&1 # ai-cron schedule
```

- **Interval**: Derived from `config.refresh_interval`, converted to whole minutes (`*/N`), minimum 1 minute
- **Env sourcing**: The `.env` file is sourced inline since cron does not inherit shell environment variables (needed for `SLACK_BOT_TOKEN`, etc.)
- **Marker comment**: `# ai-cron schedule` identifies CCC entries for idempotent install/uninstall
- **Log file**: Output appended to `~/.config/ccc/data/refresh.log`
- **Legacy cleanup**: Install and uninstall both remove the old launchd plist (`~/Library/LaunchAgents/com.ccc.refresh.plist`) if present

## Security

### Data Sanitization

All external API data is stripped of ANSI escape sequences at the refresh boundary before entering the system. This prevents terminal injection attacks where a malicious calendar event title, PR title, or Slack message could inject OSC sequences or manipulate terminal state. Sanitization uses `internal/sanitize.StripANSI()` (wrapping `ansi.Strip()`).

### API Response Size Limits

HTTP responses from Slack and Granola APIs are read with `io.LimitReader(resp.Body, 10*1024*1024)` (10MB cap) to prevent memory exhaustion from malicious or corrupted responses. Granola additionally decompresses gzip before the limit is applied.

### OAuth Hardening

- **PKCE (S256)**: All OAuth2 flows use Proof Key for Code Exchange (`internal/auth/pkce.go`). The code verifier is generated per flow and included in both the authorization URL and token exchange.
- **Random state parameter**: OAuth state is a 16-byte crypto/rand hex string, validated on callback. Prevents CSRF attacks that could associate an attacker's Google account with the user's CCC.
- **Loopback binding**: The OAuth callback server binds to `127.0.0.1` only (not all interfaces), preventing LAN-based callback interception.

### Lock File

Refresh locking uses `syscall.Flock()` for atomic advisory file locking (`internal/lockfile/lockfile.go`), eliminating the TOCTOU race condition in the previous PID-based approach. The lock file is written with `0o600` permissions.

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
- **PullRequests**: Merge-based upsert (was replace-all). Each fresh PR is upserted by ID — GitHub-sourced fields are updated while agent tracking columns (`agent_session_id`, `agent_status`, `agent_category`, `agent_head_sha`, `agent_summary`) are preserved. PRs missing from the fresh batch are archived (`state = "archived"`), not deleted. Archived PRs reappearing are reactivated (`state = "open"`).
- **PendingActions**: Preserved from existing state

## Test Cases

- ANSI escape sequences stripped from external API data (sanitize.StripANSI)
- API responses capped at 10MB via io.LimitReader
- OAuth state parameter is random and validated on callback
- PKCE code verifier/challenge generated per flow and round-trips correctly
- Lock file acquired atomically via flock (concurrent acquisition returns ErrAlreadyLocked)
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
- Auto-accept is configurable via CalendarSource.AutoAcceptDomains (not hardcoded to @example.com)
- Env file reads from `~/.config/ccc/.env` instead of `~/.airon-env`
- State stored in SQLite (via `internal/db`) instead of `command-center.json`
- DataSource interface replaces hardcoded goroutines — each source owns its auth, enablement, and fetching
- LLM extraction for Slack/Granola happens inside each source's Fetch(), not as a separate phase
- LLM extraction prompts reference speaker labels ([Aaron] / [Other]) for ownership validation
- Two-tier LLM: haiku for cheap extraction, sonnet for routing/validation with rejection capability

## Source Context

Raw source excerpts (transcripts, Slack threads, PR comments, email threads) are cached on todos for use in routing prompts and agent execution.

### ContextFetcher Interface

```go
type ContextFetcher interface {
    FetchContext(sourceRef string) (string, error)
    ContextTTL() time.Duration // 0 = immutable
}
```

### ContextRegistry

Maps source names to `ContextFetcher` implementations. Registered at startup in `ai-cron`.

| Source | TTL | Fetch Strategy |
|--------|-----|---------------|
| Granola | 0 (immutable) | Meeting transcript via `/v1/get-document-transcript` with speaker labels |
| Slack | 24h | +/-24h message window around source message + thread replies |
| GitHub | 24h | PR/issue body + comments via `gh` CLI |
| Gmail | 24h | Full email thread via Gmail API |

### Speaker Attribution (Granola)

Granola transcript chunks include a `source` field: `"microphone"` = Aaron, `"system"` = other participants. Transcripts are formatted with `[Aaron]:` and `[Other]:` labels, enabling the LLM to determine who made each commitment.

### Refresh Integration

After prompt generation, `FetchContextBestEffort` is called for each todo. Context is stored in `source_context` and `source_context_at` columns. The routing prompt includes source context in `<source_context>` tags.

### CLI

`ccc todo --fetch-context <display_id>` — manually fetch and cache source context for a specific todo.
