# SPEC: DataSource Interface

## Purpose

Extensible data source interface for the refresh pipeline. Each source owns its auth loading, enablement logic, and data fetching independently. Sources live in sub-packages under `internal/refresh/sources/`.

## Interface

### DataSource

```go
type DataSource interface {
    Name() string
    Enabled() bool
    Fetch(ctx context.Context) (*SourceResult, error)
}
```

- **Name()** — returns a stable identifier (e.g., "calendar", "gmail", "github", "slack", "granola") used for logging and warning attribution
- **Enabled()** — returns whether this source should be fetched; checked before spawning goroutine
- **Fetch(ctx)** — loads auth, fetches data, and returns results; auth errors are returned as errors (caller converts to warnings)

### SourceResult

```go
type SourceResult struct {
    Calendar *db.CalendarData
    Todos    []db.Todo
    PullRequests []db.PullRequest
    Warnings []db.Warning
}
```

Each source populates only the fields it produces. Nil/empty fields are ignored during result combination.

### PostMerger (optional)

```go
type PostMerger interface {
    PostMerge(ctx context.Context, db *sql.DB, cc *db.CommandCenter, verbose bool) error
}
```

DataSources that need to perform actions after the merge step (e.g., calendar pending action execution) implement this optional interface. The orchestrator checks each source after merge and calls PostMerge if implemented.

## Package Structure

```
internal/refresh/
  datasource.go          # DataSource, SourceResult, PostMerger, combineResults
  refresh.go             # Run() orchestrator
  merge.go               # Merge() logic
  types.go               # FreshData
  llm.go                 # generateSuggestions, CleanJSON, activeTodos, activeThreads
  auth.go                # Shared auth: GoogleTokenFile, LoadGoogleOAuth2Config, LoadEnvFile
  sources/
    calendar/
      calendar.go        # CalendarSource (implements DataSource + PostMerger)
      auth.go            # Calendar-specific auth, RunCalendarAuth, MigrateCalendarCredentials
      actions.go         # executePendingActions, findFreeSlot
      settings.go        # CalendarSettings (implements plugin.SettingsProvider)
    github/
      github.go          # GitHubSource (implements DataSource)
      settings.go        # GitHubSettings (implements plugin.SettingsProvider)
    gmail/
      gmail.go           # GmailSource (implements DataSource + PostMerger)
      client.go          # SafeGmailClient (restricted Gmail API wrapper)
      llm.go             # detectAndLabelCommitments (Gmail-specific LLM extraction)
    granola/
      granola.go         # GranolaSource (implements DataSource)
      llm.go             # extractCommitments (Granola-specific LLM extraction)
      settings.go        # GranolaSettings (implements plugin.SettingsProvider)
    slack/
      slack.go           # SlackSource (implements DataSource)
      llm.go             # extractSlackCommitments (Slack-specific LLM extraction)
```

### What stays in `internal/refresh/`

| File | Contents | Why |
|------|----------|-----|
| `datasource.go` | DataSource, SourceResult, PostMerger, combineResults() | Shared contract |
| `refresh.go` | Run() orchestrator | Source-agnostic coordination |
| `merge.go` | Merge() logic | Source-agnostic |
| `types.go` | FreshData | Shared type |
| `llm.go` | generateSuggestions(), CleanJSON(), activeTodos() | Post-merge global logic |
| `auth.go` | GoogleTokenFile, LoadGoogleOAuth2Config(), LoadEnvFile(), LoadCalendarCredsFromClaudeConfig() | Shared by calendar + gmail |

## Implementations

| Package | Name | Returns | Config | PostMerger |
|---------|------|---------|--------|------------|
| sources/calendar | "calendar" | Calendar | CalendarIDs, AutoAcceptDomains, enabled | Yes (pending actions) |
| sources/gmail | "gmail" | Todos (if todo_label set) | GmailConfig (enabled, todo_label, advanced), llm.LLM | Yes (label removal on completion) |
| sources/github | "github" | PullRequests | Repos, Username, enabled | No |
| sources/slack | "slack" | Todos (via LLM) | llm.LLM | No |
| sources/granola | "granola" | Todos (via LLM) | llm.LLM, enabled | No |

Each source struct holds its configuration (passed at construction time). Auth loading happens inside `Fetch()`, not at construction.

## Behavior

### refresh.Run() flow

1. Load env via `LoadEnvFile()`
2. Load existing state from DB
3. Iterate `opts.Sources`: for each enabled source, spawn a goroutine calling `Fetch(ctx)`
4. Collect all `SourceResult` values; combine into `FreshData` via `combineResults()`
5. Merge fresh data with existing state
6. Execute PostMerger hooks: iterate sources, call `PostMerge()` on those implementing PostMerger
7. Generate suggestions via LLM (if available)
8. Save merged result to DB

### combineResults()

Merges all `SourceResult` values into a single `FreshData`:
- Calendar: uses first non-nil `*db.CalendarData` (only CalendarSource provides it)
- Todos: concatenates all source todos
- PullRequests: concatenates all source pull requests

Note: `FreshData` does not carry warnings. Source warnings (from `SourceResult.Warnings`) and fetch-error warnings are collected separately in `Run()` after `combineResults` and set on the merged result.

### LLM extraction

Slack, Granola, and Gmail sources that need LLM perform extraction inside `Fetch()`. They receive `llm.LLM` at construction time. If LLM is a `NoopLLM`, they still fetch raw data but skip extraction (returning empty todos). LLM-specific logic lives in source-local `llm.go` files; the shared `CleanJSON()` helper is exported from the parent refresh package.

### Slack Source Behavior

#### Dual-Path Fetch Strategy

Slack uses a primary channel-based path with a search fallback:

1. **Primary path (conversations API)**: Calls `users.conversations` (paginated, up to 999 per page) to list all channels the bot is in (public, private, IM, MPIM). Requires `channels:read` scope.
2. **Fallback path (search.messages API)**: If `users.conversations` returns a `missing_scope` or `not_allowed_token_type` error, falls back to `search.messages`. Also runs as a supplement after the primary path to pick up DM/group-DM candidates not covered by channel history.
3. **Deduplication**: When both paths run, search candidates are deduplicated by permalink against channel candidates (channel candidates take precedence).

#### Channel Activity Filter

Not all channels have their history fetched. Channels pass the activity filter if:
- The channel is an IM (DM) — always included because Slack's `updated` field is unreliable for IMs (often reflects creation time, not last message)
- The channel's `updated` field (milliseconds) is >= the cutoff (15 hours ago)

#### Channel History Fetch

For each channel passing the filter, `conversations.history` is called with `oldest` set to 15 hours ago and `limit: 100`. Messages are then filtered by `hasCommitmentLanguage` (keyword match).

#### Search Query Construction

`fetchSlackCandidatesViaSearch` runs 9 separate `search.messages` API calls, one per query phrase:
- Self-commitment: `"i'll"`, `"i will"`, `"i promise"`, `"action item"`, `"follow up"`, `"let me"`
- Third-party assignment: `"aaron will"`, `"aaron is going to"`, `"aaron to follow"`

Each query is suffixed with `after:3d` to limit results to the last 3 days. Results are sorted by timestamp, limited to 50 per query. Deduplication is by `channelID:timestamp` key.

**Note**: Search results lack `ConversationContext` and `ThreadContext` — the search API returns individual matches without surrounding messages.

#### Channel-Based Context Fetching

For candidates found via the channel history path:
- **Conversation context**: Up to 15 preceding same-day messages from the same channel, with speaker labels via `userNameResolver`. Messages in the Slack API response are newest-first; preceding messages are at higher indices. Output is reversed to chronological order (oldest first). Stops at the calendar day boundary (Pacific time, UTC-7 fixed offset).
- **Thread context**: If the candidate message has a thread, `conversations.replies` fetches up to 20 replies. Each reply includes the resolved user display name.

#### User Name Resolution

`userNameResolver` caches user ID to display name lookups via `users.info` API. The authenticated user's name is pre-seeded from `auth.test`. Names are resolved as: `real_name` > `display_name` > `name` > raw user ID.

#### Incremental Processing

Candidates are filtered against the last successful sync time (from `cc_source_sync` table) with a 2-minute overlap to avoid losing messages sent during a sync cycle. Only new candidates are sent to the LLM for extraction.

#### Rate Limiting

`slackAPIGet` retries up to 3 times on HTTP 429, using the `Retry-After` header (default 5s). Additionally, `users.conversations` handles Slack-level `ratelimited` error strings with a 5-second wait.

### Gmail Source Behavior

#### Label-Based Todo Workflows

Gmail supports opt-in label synchronization via `GmailConfig.TodoLabel` and `GmailConfig.Advanced`:

- **Read-only mode** (`advanced: false`, default): Fetches emails with the configured label as todos. No write-back to Gmail.
- **Advanced mode** (`advanced: true`): Uses `gmail.modify` + `gmail.compose` scopes. Enables:
  - LLM commitment detection: analyzes sent emails and auto-labels commitments
  - Label removal on completion: PostMerge removes the todo label from completed gmail todos
- **Deduplication**: Uses Gmail message ID as `source_ref`. Each message creates at most one todo. New replies in a thread have distinct message IDs and create separate todos.
- **Safety**: Gmail API access is wrapped in `SafeGmailClient` which structurally blocks Send, Delete, and Trash operations. See CLAUDE.md "Gmail Safety Rules".

#### SafeGmailClient Methods

`SafeGmailClient` wraps `*gmail.Service` (unexported) with a restricted API surface:

- **ListMessages(ctx, query, maxResults)**: Search/list messages via `Users.Messages.List`. Used for both label queries and sent-mail scanning.
- **GetMessage(ctx, id, format, headers...)**: Fetch message in `metadata` or `full` format. When `format == "metadata"`, accepts header names to filter (e.g., Subject, From, To).
- **GetMessageBody(ctx, id)**: Fetch full message and extract plain-text body via `extractTextBody`. Walks the MIME payload tree looking for `text/plain` parts, decoding base64url content.
- **GetThread(ctx, threadID)**: Fetch a full thread by ID in `full` format. Used by the context fetcher to retrieve entire email conversations.
- **ModifyLabels(ctx, messageID, addLabelIDs, removeLabelIDs)**: Add or remove labels from a message. Returns error if not in advanced mode. Used by commitment detection (add label) and PostMerge cleanup (remove label).
- **GetLabelID(ctx, name)**: Resolve a label name to its Gmail label ID via `Users.Labels.List`. Case-insensitive name comparison.

#### Commitment Detection (Advanced Mode)

`detectAndLabelCommitments` scans recent sent emails (`in:sent newer_than:3d`, up to 20) for explicit commitments:
1. Fetches metadata (Subject, From) and body (truncated to 2000 chars) for each
2. Sends all candidates in a single LLM prompt with a high bar: must have explicit commitment, concrete action, and someone waiting
3. LLM returns a JSON array of message IDs containing real commitments
4. Applies the todo label to identified emails via `ModifyLabels`

#### PostMerge Label Cleanup

For completed Gmail todos whose message still has the todo label (tracked via `freshLabeledIDs` populated during Fetch), removes the label. Edge case: if a user re-labels the exact same message after completion, the label is removed again on the next refresh.

### Granola Source Behavior

#### Meeting Transcript Structure

1. `granolaListMeetings` calls `/v2/get-documents` to list meetings from the current week (since Sunday midnight)
2. Filters out deleted documents and those with empty titles
3. For each meeting, `granolaGetTranscript` calls `/v1/get-document-transcript` to get transcript chunks
4. Each chunk has `text` and `source` fields. The `source` field determines speaker labels:
   - `"microphone"` → `[Aaron]:` (the user's own audio)
   - `"system"` → `[Other]:` (other participants' audio)
   - Other values → `[Unknown]:`
5. Consecutive chunks from the same speaker are concatenated without repeating the label
6. Meeting summary comes from `summary` field, falling back to `notes_plain` if summary is absent

#### Attendee List

Attendees are extracted from `doc.People.Attendees` (name preferred, email as fallback). Stored on `RawMeeting.Attendees` for LLM context.

#### Incremental Processing

Like Slack, Granola filters meetings against the last successful sync time from `cc_source_sync`. Only meetings with `start_time` after the last sync are sent to the LLM.

#### Auth

Token is read from `~/Library/Application Support/Granola/stored-accounts.json` — a nested JSON structure (string-encoded accounts array containing string-encoded tokens). Checks token expiry based on `savedAt + expiresIn`.

### SettingsProvider

Calendar, GitHub, and Granola source packages also export `Settings` types implementing `plugin.SettingsProvider`. These are constructed by the Settings plugin at init time and provide source-specific settings detail views (enabled toggle, credential status, repo management, etc.).

## Test Cases

- Source with `Enabled() == false` is not fetched
- Source returning error produces a warning, not a fatal error
- Multiple sources' results are combined correctly
- Calendar field uses first non-nil value
- Todos and pull requests from multiple sources are concatenated
- Warnings from sources are preserved
- PostMerger is called after merge for sources that implement it
- matchesDomain correctly matches email domains (calendar)
- hasCommitmentLanguage detects commitment phrases (slack)
- findFreeSlot returns error for malformed event times (calendar)

### Slack-Specific

- Conversations API `missing_scope` error triggers search.messages fallback
- Search queries include `after:3d` suffix
- Search deduplication by `channelID:timestamp` prevents duplicate candidates
- Channel+search deduplication by permalink (channel candidates take precedence)
- IM channels always pass activity filter regardless of `updated` field
- Conversation context stops at calendar day boundary (Pacific time)
- Conversation context output is chronological (oldest first)
- Rate-limited API calls retry with Retry-After header (up to 3 attempts)
- Candidates older than last successful sync (minus 2-minute overlap) are filtered out

### Gmail-Specific

- Label-based todos use Gmail message ID as source_ref
- LLM title generation falls back to subject when LLM is nil or returns no title
- GetMessageBody extracts text/plain from MIME tree (base64url decoded)
- ModifyLabels returns error when advanced mode is false
- GetLabelID does case-insensitive name comparison
- PostMerge only removes label from completed todos whose message still has the label
- Commitment detection searches `in:sent newer_than:3d` (up to 20 results)

### Granola-Specific

- Transcript speaker labels: `"microphone"` → `[Aaron]:`, `"system"` → `[Other]:`
- Consecutive same-speaker chunks are concatenated without repeating label
- Deleted documents are excluded
- Meetings older than current week start (Sunday midnight) are excluded
- Meetings older than last successful sync are skipped for LLM extraction

## Key Design Decisions

- Source packages import the parent `refresh` package for shared types (GoogleTokenFile, CleanJSON, etc.) — no circular imports since `refresh` never imports source packages
- `cmd/ai-cron/main.go` constructs sources from source packages, passes them to `refresh.Run()`
- Calendar credential migration happens in `CalendarSource.Fetch()`, making it self-contained
- PostMerger pattern keeps the orchestrator source-agnostic while allowing calendar-specific post-merge actions

### GitHub PR Detail Fields

The `gh pr view --json` call includes `headRefOid` to fetch the current HEAD SHA of each PR. This is mapped to `HeadSHA` on the `db.PullRequest` struct and stored in the `head_sha` column. The HEAD SHA serves as the version signal for agent trigger detection — when the SHA changes, a new agent run is warranted.

## Data Safety

### ANSI Sanitization

All string data from external APIs (event titles, PR titles, Slack messages, etc.) is stripped of ANSI escape sequences at the refresh boundary via `internal/sanitize.StripANSI()`. This prevents terminal injection where a malicious string could manipulate terminal state via OSC or CSI sequences. Lipgloss/bubbletea do not strip raw ANSI from input strings.

### Response Size Limits

HTTP responses from Slack and Granola are read with `io.LimitReader(resp.Body, 10*1024*1024)` (10MB) to prevent memory exhaustion. This is especially important for Granola, which decompresses gzip before reading.
