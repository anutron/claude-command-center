# SPEC: Todo Source Context

## Purpose

Give todo-executing agents access to the raw source material that spawned each todo. Today, todos have an LLM-summarized `detail` field but no verbatim excerpt from the originating meeting transcript, Slack thread, GitHub PR, or email. Agents work with lossy summaries and have no easy way to get back to the original context.

This feature adds:
- A `source_context` field that stores a raw excerpt from the source
- A CLI command to fetch/refresh that context on demand
- Automatic population at todo creation time
- TTL-based refresh for sources whose content changes over time

## Schema Changes

Two new columns on `cc_todos`:

```sql
source_context TEXT DEFAULT ''    -- raw excerpt from the originating source
source_context_at TEXT DEFAULT '' -- ISO8601 timestamp of last fetch
```

Added via a new migration in the plugin migration list.

- `source_context` holds verbatim content (transcript, messages, PR body + comments, email thread)
- `source_context_at` tracks when the context was last fetched, used for TTL logic

## Source Interface Extension

### ContextFetcher Interface

Each source that can provide context implements a new interface:

```go
type ContextFetcher interface {
    FetchContext(sourceRef string) (string, error)
    ContextTTL() time.Duration // 0 means immutable, no refresh needed
}
```

The core code never contains source-specific logic. It resolves the source name to a `ContextFetcher` via a registry and calls it polymorphically. Sources that don't support context fetching (e.g., `cli`) simply don't register a fetcher.

### Registry

The `ContextFetcher` registry lives in `internal/refresh/context.go` alongside the existing refresh orchestration code. Each source package registers its fetcher during initialization. The registry is a `map[string]ContextFetcher` keyed by source name, built by the refresh package and passed to the CLI command when needed.

```go
fetcher, ok := registry[todo.Source]
if !ok {
    return "", nil // no fetcher for this source
}
return fetcher.FetchContext(todo.SourceRef)
```

Adding a new source with context support requires only: implement `ContextFetcher`, register it. No core code changes.

### Error Handling

Context fetching is best-effort. If a source API is unreachable or returns an error:
- During refresh: log a warning, leave `source_context` empty, continue with todo creation
- During CLI `--fetch-context`: print the error to stderr, exit non-zero
- Never block todo creation on a context fetch failure

### Per-Source Fetch Strategies

| Source | What We Fetch | TTL | SourceRef Format |
|--------|--------------|-----|-----------------|
| **granola** | Full meeting transcript | None (immutable) | `{meeting_id}-{title_hash}` |
| **slack** | Channel messages within +/-24h of source message, plus all thread replies on those messages | 24h | Slack message permalink |
| **github** | PR/issue body + all comments | 24h | PR/issue URL |
| **gmail** | Full email thread | 24h | Email message ID |
| **cli/manual** | Nothing (no source to fetch) | N/A | User-provided or empty |

### Slack Fetch Constraints

Slack channels can have years of history. The fetch window is bounded:

1. Parse channel ID and message timestamp from the permalink in `source_ref`
2. Fetch all channel messages from 24h before to 24h after the source message timestamp
3. For each of those messages, also fetch their thread replies
4. Return the combined content as the context excerpt

This produces a reasonable bounded result (typically a few dozen messages) while capturing the surrounding conversation.

## CLI Command

```bash
ccc todo --fetch-context <display_id>
```

### Behavior

1. Load the todo by `display_id`
2. Check cache validity:
   - If `source_context` is populated AND source has no TTL -> print cached content, done
   - If `source_context` is populated AND `source_context_at` is within the source's TTL -> print cached content, done
   - Otherwise -> fetch fresh
3. Look up `ContextFetcher` for the todo's `source` in the registry
4. If no fetcher registered -> print empty, done
5. Call `fetcher.FetchContext(todo.SourceRef)`
6. Save result to `source_context`, set `source_context_at` to current time
7. Print the raw content to stdout (plain text, not JSON — intended for piping or agent consumption)

### TTL Logic

```go
func shouldRefresh(todo db.Todo, fetcher ContextFetcher) bool {
    if todo.SourceContext == "" {
        return true
    }
    ttl := fetcher.ContextTTL()
    if ttl == 0 {
        return false // immutable source
    }
    fetchedAt, err := time.Parse(time.RFC3339, todo.SourceContextAt)
    if err != nil {
        return true
    }
    return time.Since(fetchedAt) > ttl
}
```

## Refresh Agent Integration

When the refresh agent creates a new todo, it fetches source context immediately after insertion:

1. LLM extracts commitments from source data (existing behavior)
2. Todo inserted into DB with `source` and `source_ref` (existing behavior)
3. **New:** Call `ContextFetcher.FetchContext()` for the todo's source
4. **New:** Save result to `source_context` and `source_context_at`

New todos arrive fully populated with source context.

## Prompt Template Integration

The todo-agent and todo-trainer prompt templates append source context at the bottom of the proposed prompt:

```
## Source Reference

**Source:** {source} ({source_ref})
**Fetched:** {source_context_at}

<source_context>
{source_context}
</source_context>

If this context is insufficient, run `ccc todo --fetch-context {display_id}`
to refresh it, or access the source directly.
```

This gives the executing agent:
- The raw source material inline (no extra fetch needed in most cases)
- A fallback command to refresh if the context is stale or insufficient
- Clear labeling of what's LLM-summarized (`detail`) vs. verbatim (`source_context`)

## Backfilling Existing Todos

Existing active todos can be backfilled by running `--fetch-context` on each:

```bash
ccc todo --fetch-context 40
ccc todo --fetch-context 43
# ... etc for each active todo
```

Todos with `source=cli` or missing `source_ref` are skipped gracefully (no fetcher registered).

## Test Cases

- **Happy path:** Fetch context for a Granola-sourced todo, verify `source_context` and `source_context_at` are populated
- **TTL respected:** Fetch GitHub todo context, fetch again within 24h -> returns cached, fetch after 24h -> re-fetches
- **Immutable source:** Fetch Granola todo context twice -> second call returns cached (no API call)
- **Unknown source:** Todo with `source=cli` -> returns empty, no error
- **Missing source_ref:** Todo with empty `source_ref` -> returns empty, no error
- **Slack window:** Verify Slack fetch only retrieves messages within +/-24h of source message
- **CLI output:** `--fetch-context` prints content to stdout and persists to DB
- **Refresh agent:** New todo created by refresh has `source_context` populated on first insert
