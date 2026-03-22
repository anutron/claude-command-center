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
    Threads  []db.Thread
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
| `llm.go` | generateSuggestions(), CleanJSON(), activeTodos(), activeThreads() | Post-merge global logic |
| `auth.go` | GoogleTokenFile, LoadGoogleOAuth2Config(), LoadEnvFile(), LoadCalendarCredsFromClaudeConfig() | Shared by calendar + gmail |

## Implementations

| Package | Name | Returns | Config | PostMerger |
|---------|------|---------|--------|------------|
| sources/calendar | "calendar" | Calendar | CalendarIDs, AutoAcceptDomains, enabled | Yes (pending actions) |
| sources/gmail | "gmail" | Threads + Todos (if todo_label set) | GmailConfig (enabled, todo_label, advanced), llm.LLM | Yes (label removal on completion) |
| sources/github | "github" | Threads, PullRequests | Repos, Username, enabled | No |
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
- Threads: concatenates all source threads

Note: `FreshData` does not carry warnings. Source warnings (from `SourceResult.Warnings`) and fetch-error warnings are collected separately in `Run()` after `combineResults` and set on the merged result.

### LLM extraction

Slack, Granola, and Gmail sources that need LLM perform extraction inside `Fetch()`. They receive `llm.LLM` at construction time. If LLM is a `NoopLLM`, they still fetch raw data but skip extraction (returning empty todos). LLM-specific logic lives in source-local `llm.go` files; the shared `CleanJSON()` helper is exported from the parent refresh package.

### Gmail label-based todos

Gmail supports opt-in label synchronization via `GmailConfig.TodoLabel` and `GmailConfig.Advanced`:

- **Read-only mode** (`advanced: false`, default): Fetches emails with the configured label as todos. No write-back to Gmail.
- **Advanced mode** (`advanced: true`): Uses `gmail.modify` + `gmail.compose` scopes. Enables:
  - LLM commitment detection: analyzes sent emails and auto-labels commitments
  - Label removal on completion: PostMerge removes the todo label from completed gmail todos
- **Deduplication**: Uses Gmail message ID as `source_ref`. Each message creates at most one todo. New replies in a thread have distinct message IDs and create separate todos.
- **Safety**: Gmail API access is wrapped in `SafeGmailClient` which structurally blocks Send, Delete, and Trash operations. See CLAUDE.md "Gmail Safety Rules".

### SettingsProvider

Calendar, GitHub, and Granola source packages also export `Settings` types implementing `plugin.SettingsProvider`. These are constructed by the Settings plugin at init time and provide source-specific settings detail views (enabled toggle, credential status, repo management, etc.).

## Test Cases

- Source with `Enabled() == false` is not fetched
- Source returning error produces a warning, not a fatal error
- Multiple sources' results are combined correctly
- Calendar field uses first non-nil value
- Todos and threads from multiple sources are concatenated
- Warnings from sources are preserved
- PostMerger is called after merge for sources that implement it
- matchesDomain correctly matches email domains (calendar)
- hasCommitmentLanguage detects commitment phrases (slack)
- findFreeSlot returns error for malformed event times (calendar)

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
