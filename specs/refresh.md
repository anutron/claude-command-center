# SPEC: Refresh Package

## Purpose
Fetches data from multiple external sources (Google Calendar, Gmail, GitHub, Slack, Granola), uses LLM extraction to identify action items and commitments, merges fresh data with existing state, and saves the updated command center JSON.

## Interface

- **Input**: `Options` struct configuring the refresh run
  - `Verbose bool` — enable verbose logging
  - `NoLLM bool` — skip LLM calls (data-only refresh)
  - `DryRun bool` — print JSON to stdout instead of writing
  - `GitHubRepos []string` — repos to check for open PRs
  - `GitHubUsername string` — GitHub username (reserved)
  - `CalendarIDs []string` — Google Calendar IDs to fetch (defaults to `["primary"]`)
  - `AutoAcceptDomains []string` — email domains to auto-accept calendar events from
  - `StateDir string` — directory for command-center.json (defaults to `~/.config/ccc/data`)
- **Output**: `error` (nil on success)
- **Entry point**: `refresh.Run(opts Options) error`
- **Dependencies**: Google OAuth2 tokens (Calendar, Gmail), Slack bot token, Granola stored auth, `gh` CLI, `claude` CLI

## Behavior

1. Load env vars from `~/.config/ccc/.env` (for launchd environments)
2. Load existing command-center.json from StateDir
3. Migrate calendar credentials if needed (one-time)
4. Load auth for all services (Calendar, Gmail, Slack, Granola); missing auth produces a warning, not a fatal error
5. Check for `claude` CLI availability; disable LLM features if missing
6. **Parallel data fetch**: Calendar events (today + tomorrow from all CalendarIDs), Gmail (unread last 3 days), GitHub PRs (from configured repos via `gh` CLI), Slack candidates (messages with commitment language), Granola meetings (this week's with transcripts)
7. **Auto-accept**: If AutoAcceptDomains configured, accept pending calendar events from those domains
8. **LLM extraction** (parallel): Extract commitments from Granola transcripts and Slack candidates using `claude` CLI
9. **Merge**: Combine fresh data with existing state preserving IDs, statuses, dismissed items, manual items, and pause states
10. **Execute pending actions**: Process booking requests by creating calendar events in free slots
11. **Generate suggestions**: LLM-based priority ranking of todos
12. Write merged state to command-center.json (or stdout if DryRun)

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
- GitHub repos come from Options, not hardcoded
- Calendar supports multiple IDs from Options
- Auto-accept is configurable via `AutoAcceptDomains` (not hardcoded to @thanx.com)
- Env file reads from `~/.config/ccc/.env` instead of `~/.airon-env`
- State dir defaults to `~/.config/ccc/data` instead of `~/.claude/airon`
- LLM prompts use generic "user" language instead of "Aaron"
