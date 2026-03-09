# SPEC: DataSource Interface

## Purpose

Extensible data source interface for the refresh pipeline. Replaces hardcoded fetcher goroutines in `refresh.Run()` with a uniform `DataSource` interface, enabling each source to own its auth loading, enablement logic, and data fetching independently.

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

## Implementations

| Struct | Name | Returns | Config |
|--------|------|---------|--------|
| CalendarSource | "calendar" | Calendar | CalendarIDs, AutoAcceptDomains, enabled flag |
| GmailSource | "gmail" | Threads | (auth only) |
| GitHubSource | "github" | Threads | Repos |
| SlackSource | "slack" | Todos (via LLM) | llm.LLM |
| GranolaSource | "granola" | Todos (via LLM) | llm.LLM |

Each source struct holds its configuration (passed at construction time). Auth loading happens inside `Fetch()`, not at construction.

## Behavior

### refresh.Run() flow

1. Load env, existing state, migrate credentials (unchanged)
2. Iterate `opts.Sources`: for each enabled source, spawn a goroutine calling `Fetch(ctx)`
3. Collect all `SourceResult` values; combine into `FreshData`
4. Merge, execute pending actions, generate suggestions, save (unchanged)

### combineResults()

Merges all `SourceResult` values into a single `FreshData`:
- Calendar: uses first non-nil `*db.CalendarData` (only CalendarSource provides it)
- Todos: concatenates all source todos
- Threads: concatenates all source threads
- Warnings: concatenates all source warnings

### LLM extraction

Slack and Granola sources that need LLM perform extraction inside `Fetch()`. They receive `llm.LLM` at construction time. If LLM is a `NoopLLM`, they still fetch raw data but skip extraction (returning empty todos).

## Test Cases

- Source with `Enabled() == false` is not fetched
- Source returning error produces a warning, not a fatal error
- Multiple sources' results are combined correctly
- Calendar field uses first non-nil value
- Todos and threads from multiple sources are concatenated
- Warnings from sources are preserved

## Key Changes from Previous Design

- `Options` no longer carries per-source config (CalendarIDs, GitHubRepos, etc.) — those live on source structs
- `Options` carries `Sources []DataSource` instead of enable flags and config fields
- Auth loading moves from `Run()` into each source's `Fetch()`
- LLM extraction for Slack/Granola happens inside `Fetch()` rather than as a separate post-fetch phase
