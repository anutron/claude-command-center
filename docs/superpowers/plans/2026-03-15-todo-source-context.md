# Todo Source Context Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store raw source excerpts (transcripts, Slack threads, PR comments, email threads) on todos so executing agents have full original context.

**Architecture:** New `ContextFetcher` interface per source, `source_context` + `source_context_at` columns on `cc_todos`, CLI `--fetch-context` command, and automatic population during refresh. Each source owns its fetch logic and TTL; the core orchestrates without source-specific knowledge.

**Tech Stack:** Go, SQLite, existing source API clients (Granola REST, Slack Web API, GitHub `gh` CLI, Gmail OAuth)

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/db/types.go` | Modify | Add `SourceContext` and `SourceContextAt` fields to `Todo` struct |
| `internal/db/schema.go` | Modify | Add migration for two new columns |
| `internal/db/read.go` | Modify | Add new columns to SELECT and Scan in `dbLoadTodos` and `DBLoadTodoByDisplayID` |
| `internal/db/write.go` | Modify | Add new columns to INSERT/UPDATE in `DBInsertTodo`, `DBUpdateTodo`, `DBSaveRefreshResult` |
| `internal/refresh/context.go` | Create | `ContextFetcher` interface, registry, `FetchAndSave` orchestrator |
| `internal/refresh/context_test.go` | Create | Tests for registry, TTL logic, FetchAndSave |
| `internal/refresh/sources/granola/context.go` | Create | `ContextFetcher` for Granola (fetch transcript by meeting ID) |
| `internal/refresh/sources/slack/context.go` | Create | `ContextFetcher` for Slack (+/-24h window + thread replies) |
| `internal/refresh/sources/github/context.go` | Create | `ContextFetcher` for GitHub (PR/issue body + comments via `gh`) |
| `internal/refresh/sources/gmail/context.go` | Create | `ContextFetcher` for Gmail (full email thread) |
| `cmd/ccc/todo.go` | Modify | Add `--fetch-context` flag |
| `internal/refresh/refresh.go` | Modify | Call context fetch after prompt generation |

---

## Chunk 1: Schema and DB Layer

### Task 1: Add fields to Todo struct

**Files:**
- Modify: `internal/db/types.go:96-117`

- [ ] **Step 1: Add SourceContext and SourceContextAt fields**

Add after `SessionSummary` (line 113):

```go
SourceContext   string `json:"source_context,omitempty"`
SourceContextAt string `json:"source_context_at,omitempty"`
```

- [ ] **Step 2: Run tests to verify no breakage**

Run: `go test ./internal/db/...`

- [ ] **Step 3: Commit**

```bash
git add internal/db/types.go
git commit -m "Add SourceContext and SourceContextAt fields to Todo struct"
```

### Task 2: Add schema migration

**Files:**
- Modify: `internal/db/schema.go:189` (after `launch_mode` migration)

- [ ] **Step 1: Add ALTER TABLE statements**

Add after the `launch_mode` migration (line 189):

```go
// Add source_context columns to todos for raw source excerpts
_, _ = db.Exec(`ALTER TABLE cc_todos ADD COLUMN source_context TEXT`)
_, _ = db.Exec(`ALTER TABLE cc_todos ADD COLUMN source_context_at TEXT`)
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/db/...`

- [ ] **Step 3: Commit**

```bash
git add internal/db/schema.go
git commit -m "Add source_context columns to cc_todos schema"
```

### Task 3: Update read functions

**Files:**
- Modify: `internal/db/read.go:57-104` (`dbLoadTodos`)
- Modify: `internal/db/read.go:419-439` (`DBLoadTodoByDisplayID`)

- [ ] **Step 1: Update dbLoadTodos SELECT and Scan**

In `dbLoadTodos` (line 58), add `source_context, source_context_at` to the SELECT:

```sql
SELECT id, COALESCE(display_id, 0), title, status, source, source_ref, context, detail,
    who_waiting, project_dir, launch_mode, due, effort, session_id, proposed_prompt, session_status,
    session_summary, source_context, source_context_at, COALESCE(triage_status, 'accepted'), created_at, completed_at
    FROM cc_todos ORDER BY sort_order ASC
```

Add `sourceContext, sourceContextAt sql.NullString` to the variable declarations (line 72).

Add to Scan (line 75-78):

```go
err := rows.Scan(&t.ID, &t.DisplayID, &t.Title, &t.Status, &t.Source,
    &sourceRef, &ctx, &detail, &who, &projDir, &launchMode, &due, &effort, &sessionID,
    &proposedPrompt, &sessionStatus, &sessionSummary, &sourceContext, &sourceContextAt, &triageStatus,
    &createdStr, &completedStr)
```

Add field assignments (after line 94):

```go
t.SourceContext = sourceContext.String
t.SourceContextAt = sourceContextAt.String
```

- [ ] **Step 2: Update DBLoadTodoByDisplayID SELECT and Scan**

Same pattern — add `source_context, source_context_at` to SELECT (line 426), add variables, Scan, and assignments. Mirror the same changes from Step 1.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/db/...`

- [ ] **Step 4: Commit**

```bash
git add internal/db/read.go
git commit -m "Read source_context columns from cc_todos"
```

### Task 4: Update write functions

**Files:**
- Modify: `internal/db/write.go:68-98` (`DBInsertTodo`)
- Modify: `internal/db/write.go:100-126` (`DBUpdateTodo`)
- Modify: `internal/db/write.go:457-468` (`DBSaveRefreshResult` todo insert loop)

- [ ] **Step 1: Update DBInsertTodo**

Add `source_context, source_context_at` to the INSERT column list and VALUES (line 83-90). Use `NULLIF(?, '')` pattern. Add `t.SourceContext, t.SourceContextAt` to the args.

- [ ] **Step 2: Update DBUpdateTodo**

Add `source_context = NULLIF(?, ''), source_context_at = NULLIF(?, '')` to the SET clause (line 111-117). Add `t.SourceContext, t.SourceContextAt` to the args.

- [ ] **Step 3: Update DBSaveRefreshResult**

Add `source_context, source_context_at` to the INSERT in the todo loop (line 457-464). Add `NULLIF(?, ''), NULLIF(?, '')` to VALUES. Add `t.SourceContext, t.SourceContextAt` to the args.

- [ ] **Step 4: Add DBUpdateTodoSourceContext helper**

Add a focused update function following the pattern of `DBUpdateTodoSessionStatus`:

```go
func DBUpdateTodoSourceContext(db *sql.DB, id, sourceContext, sourceContextAt string) error {
    now := FormatTime(time.Now())
    _, err := db.Exec(`UPDATE cc_todos SET source_context = NULLIF(?, ''), source_context_at = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
        sourceContext, sourceContextAt, now, id)
    if err != nil {
        return fmt.Errorf("update todo source_context %s: %w", id, err)
    }
    return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/db/...`

- [ ] **Step 6: Run full test suite**

Run: `go test ./...`

- [ ] **Step 7: Commit**

```bash
git add internal/db/write.go
git commit -m "Write source_context columns in todo insert/update/refresh"
```

---

## Chunk 2: ContextFetcher Interface and Registry

### Task 5: Create ContextFetcher interface and registry

**Files:**
- Create: `internal/refresh/context.go`
- Create: `internal/refresh/context_test.go`

- [ ] **Step 1: Write tests for registry and TTL logic**

```go
// internal/refresh/context_test.go
package refresh

import (
    "context"
    "testing"
    "time"

    "github.com/anutron/claude-command-center/internal/db"
)

type mockFetcher struct {
    content string
    err     error
    ttl     time.Duration
}

func (m *mockFetcher) FetchContext(sourceRef string) (string, error) {
    return m.content, m.err
}
func (m *mockFetcher) ContextTTL() time.Duration { return m.ttl }

func TestShouldRefresh_EmptyContext(t *testing.T) {
    todo := db.Todo{SourceContext: ""}
    f := &mockFetcher{ttl: 24 * time.Hour}
    if !shouldRefresh(todo, f) {
        t.Error("expected refresh for empty source_context")
    }
}

func TestShouldRefresh_ImmutableSource(t *testing.T) {
    todo := db.Todo{SourceContext: "content", SourceContextAt: db.FormatTime(time.Now())}
    f := &mockFetcher{ttl: 0}
    if shouldRefresh(todo, f) {
        t.Error("expected no refresh for immutable source with content")
    }
}

func TestShouldRefresh_FreshTTL(t *testing.T) {
    todo := db.Todo{SourceContext: "content", SourceContextAt: db.FormatTime(time.Now())}
    f := &mockFetcher{ttl: 24 * time.Hour}
    if shouldRefresh(todo, f) {
        t.Error("expected no refresh for fresh TTL source")
    }
}

func TestShouldRefresh_StaleTTL(t *testing.T) {
    staleTime := time.Now().Add(-25 * time.Hour)
    todo := db.Todo{SourceContext: "content", SourceContextAt: db.FormatTime(staleTime)}
    f := &mockFetcher{ttl: 24 * time.Hour}
    if !shouldRefresh(todo, f) {
        t.Error("expected refresh for stale TTL source")
    }
}

func TestRegistryLookup(t *testing.T) {
    reg := NewContextRegistry()
    f := &mockFetcher{content: "test"}
    reg.Register("granola", f)

    got, ok := reg.Get("granola")
    if !ok || got != f {
        t.Error("expected to find registered fetcher")
    }

    _, ok = reg.Get("unknown")
    if ok {
        t.Error("expected not found for unregistered source")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/refresh/ -run TestShouldRefresh -v`
Expected: FAIL (functions don't exist yet)

- [ ] **Step 3: Implement context.go**

```go
// internal/refresh/context.go
package refresh

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    "time"

    "github.com/anutron/claude-command-center/internal/db"
)

// ContextFetcher is implemented by sources that can retrieve raw context
// for a todo's source_ref. Each source owns its fetch logic and TTL.
type ContextFetcher interface {
    FetchContext(sourceRef string) (string, error)
    ContextTTL() time.Duration // 0 means immutable, no refresh needed
}

// ContextRegistry maps source names to their ContextFetcher implementations.
type ContextRegistry struct {
    fetchers map[string]ContextFetcher
}

func NewContextRegistry() *ContextRegistry {
    return &ContextRegistry{fetchers: make(map[string]ContextFetcher)}
}

func (r *ContextRegistry) Register(source string, f ContextFetcher) {
    r.fetchers[source] = f
}

func (r *ContextRegistry) Get(source string) (ContextFetcher, bool) {
    f, ok := r.fetchers[source]
    return f, ok
}

// shouldRefresh returns true if the todo's source_context needs to be fetched or refreshed.
func shouldRefresh(todo db.Todo, fetcher ContextFetcher) bool {
    if todo.SourceContext == "" {
        return true
    }
    ttl := fetcher.ContextTTL()
    if ttl == 0 {
        return false // immutable source
    }
    fetchedAt := db.ParseTime(todo.SourceContextAt)
    if fetchedAt.IsZero() {
        return true
    }
    return time.Since(fetchedAt) > ttl
}

// FetchAndSave fetches source context for a todo if needed, saves it to the DB,
// and returns the content. Returns empty string with no error if no fetcher is
// registered or no refresh is needed.
func FetchAndSave(ctx context.Context, database *sql.DB, registry *ContextRegistry, todo *db.Todo) (string, error) {
    fetcher, ok := registry.Get(todo.Source)
    if !ok {
        return "", nil
    }

    if !shouldRefresh(*todo, fetcher) {
        return todo.SourceContext, nil
    }

    content, err := fetcher.FetchContext(todo.SourceRef)
    if err != nil {
        return "", fmt.Errorf("fetch context for %s/%s: %w", todo.Source, todo.SourceRef, err)
    }

    now := db.FormatTime(time.Now())
    if err := db.DBUpdateTodoSourceContext(database, todo.ID, content, now); err != nil {
        return content, fmt.Errorf("save source context: %w", err)
    }

    todo.SourceContext = content
    todo.SourceContextAt = now
    return content, nil
}

// FetchContextBestEffort fetches and saves source context, logging errors
// instead of propagating them. Used during refresh where context fetch
// should not block todo creation.
func FetchContextBestEffort(ctx context.Context, database *sql.DB, registry *ContextRegistry, todo *db.Todo) {
    _, err := FetchAndSave(ctx, database, registry, todo)
    if err != nil {
        log.Printf("source context for %s #%d: %v", todo.Source, todo.DisplayID, err)
    }
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/refresh/ -run "TestShouldRefresh|TestRegistry" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/refresh/context.go internal/refresh/context_test.go
git commit -m "Add ContextFetcher interface and registry with TTL logic"
```

---

## Chunk 3: Source ContextFetcher Implementations

### Task 6: Granola ContextFetcher

**Files:**
- Create: `internal/refresh/sources/granola/context.go`

- [ ] **Step 1: Implement FetchContext**

The source_ref format is `{meeting_id}-{title_hash}`. Extract the meeting ID (everything before the last `-`), fetch the transcript using existing `granolaGetTranscript` and meeting summary using `granolaListMeetings`.

```go
// internal/refresh/sources/granola/context.go
package granola

import (
    "context"
    "fmt"
    "strings"
    "time"
)

// ContextFetcherImpl fetches meeting transcripts from Granola.
type ContextFetcherImpl struct{}

func NewContextFetcher() *ContextFetcherImpl {
    return &ContextFetcherImpl{}
}

func (f *ContextFetcherImpl) ContextTTL() time.Duration { return 0 } // immutable

func (f *ContextFetcherImpl) FetchContext(sourceRef string) (string, error) {
    // source_ref format: {meeting_id}-{title_hash}
    // Extract meeting ID: everything before the last dash
    lastDash := strings.LastIndex(sourceRef, "-")
    if lastDash < 0 {
        return "", fmt.Errorf("invalid granola source_ref: %s", sourceRef)
    }
    meetingID := sourceRef[:lastDash]

    token, err := loadGranolaAuth()
    if err != nil {
        return "", fmt.Errorf("granola auth: %w", err)
    }

    ctx := context.Background()
    transcript, err := granolaGetTranscript(ctx, token, meetingID)
    if err != nil {
        return "", fmt.Errorf("fetch transcript: %w", err)
    }

    return transcript, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/refresh/sources/granola/`

- [ ] **Step 3: Commit**

```bash
git add internal/refresh/sources/granola/context.go
git commit -m "Add Granola ContextFetcher — fetches meeting transcript"
```

### Task 7: Slack ContextFetcher

**Files:**
- Create: `internal/refresh/sources/slack/context.go`

- [ ] **Step 1: Implement FetchContext**

Parse channel ID and message timestamp from the Slack permalink. Fetch channel messages in +/-24h window using `conversations.history` with `oldest` and `latest` params. For each message in the window, fetch thread replies. Combine into formatted output.

```go
// internal/refresh/sources/slack/context.go
package slack

import (
    "context"
    "fmt"
    "net/url"
    "strconv"
    "strings"
    "time"
)

// ContextFetcherImpl fetches Slack conversation context around a message.
type ContextFetcherImpl struct {
    BotToken string
}

func NewContextFetcher(botToken string) *ContextFetcherImpl {
    return &ContextFetcherImpl{BotToken: botToken}
}

func (f *ContextFetcherImpl) ContextTTL() time.Duration { return 24 * time.Hour }

func (f *ContextFetcherImpl) FetchContext(sourceRef string) (string, error) {
    channelID, ts, err := parseSlackPermalink(sourceRef)
    if err != nil {
        return "", err
    }

    ctx := context.Background()
    token := strings.TrimSpace(f.BotToken)
    if token == "" {
        return "", fmt.Errorf("slack bot token not configured")
    }

    // Parse the source message timestamp to compute the +/-24h window
    tsFloat, err := strconv.ParseFloat(ts, 64)
    if err != nil {
        return "", fmt.Errorf("invalid slack timestamp %q: %w", ts, err)
    }
    msgTime := time.Unix(int64(tsFloat), 0)
    oldest := strconv.FormatInt(msgTime.Add(-24*time.Hour).Unix(), 10)
    latest := strconv.FormatInt(msgTime.Add(24*time.Hour).Unix(), 10)

    // Fetch channel messages in the window
    messages, err := fetchChannelHistoryWindow(ctx, token, channelID, oldest, latest)
    if err != nil {
        return "", fmt.Errorf("fetch channel history: %w", err)
    }

    // Build output with thread replies
    var sb strings.Builder
    for _, msg := range messages {
        if msg.Text == "" {
            continue
        }
        sb.WriteString(msg.Text)
        sb.WriteString("\n")

        // Fetch thread replies for each message
        replies, err := fetchThreadContext(ctx, token, channelID, msg.TS)
        if err == nil && len(replies) > 1 {
            for _, reply := range replies[1:] { // skip parent (already included)
                sb.WriteString("  > ")
                sb.WriteString(reply.Text)
                sb.WriteString("\n")
            }
        }
        sb.WriteString("\n")
    }

    return strings.TrimSpace(sb.String()), nil
}

// parseSlackPermalink extracts channel ID and timestamp from a Slack permalink.
// Format: https://app.slack.com/archives/{channelID}/p{ts_without_dot}
func parseSlackPermalink(permalink string) (channelID, ts string, err error) {
    u, err := url.Parse(permalink)
    if err != nil {
        return "", "", fmt.Errorf("invalid permalink: %w", err)
    }
    parts := strings.Split(strings.Trim(u.Path, "/"), "/")
    // Expected: ["archives", channelID, "pTIMESTAMP"]
    if len(parts) < 3 || parts[0] != "archives" {
        return "", "", fmt.Errorf("unexpected permalink format: %s", permalink)
    }
    channelID = parts[1]
    tsRaw := strings.TrimPrefix(parts[2], "p")
    // Re-insert the dot: "1710000000000100" -> "1710000000.000100"
    if len(tsRaw) > 10 {
        ts = tsRaw[:10] + "." + tsRaw[10:]
    } else {
        ts = tsRaw
    }
    return channelID, ts, nil
}

// fetchChannelHistoryWindow retrieves messages in a time window.
func fetchChannelHistoryWindow(ctx context.Context, token, channelID, oldest, latest string) ([]slackHistoryEntry, error) {
    params := url.Values{
        "channel": {channelID},
        "oldest":  {oldest},
        "latest":  {latest},
        "limit":   {"200"},
    }

    var result slackHistoryResponse
    if err := slackAPIGet(ctx, token, "conversations.history", params, &result); err != nil {
        return nil, err
    }
    if !result.OK {
        return nil, fmt.Errorf("conversations.history error: %s", result.Error)
    }

    return result.Messages, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/refresh/sources/slack/`

- [ ] **Step 3: Commit**

```bash
git add internal/refresh/sources/slack/context.go
git commit -m "Add Slack ContextFetcher — fetches +/-24h window with thread replies"
```

### Task 8: GitHub ContextFetcher

**Files:**
- Create: `internal/refresh/sources/github/context.go`

- [ ] **Step 1: Implement FetchContext**

GitHub source_ref is a PR/issue URL. Use `gh` CLI to fetch the PR body and comments.

```go
// internal/refresh/sources/github/context.go
package github

import (
    "encoding/json"
    "fmt"
    "os/exec"
    "strings"
    "time"
)

// ContextFetcherImpl fetches PR/issue body and comments from GitHub.
type ContextFetcherImpl struct{}

func NewContextFetcher() *ContextFetcherImpl {
    return &ContextFetcherImpl{}
}

func (f *ContextFetcherImpl) ContextTTL() time.Duration { return 24 * time.Hour }

func (f *ContextFetcherImpl) FetchContext(sourceRef string) (string, error) {
    // sourceRef is a GitHub URL like https://github.com/org/repo/pull/123
    // Use gh CLI to fetch details
    out, err := exec.Command("gh", "pr", "view", sourceRef, "--json",
        "title,body,comments,reviews", "--jq",
        `"# " + .title + "\n\n" + .body + "\n\n## Comments\n" + ([.comments[] | "**" + .author.login + ":** " + .body] | join("\n\n")) + "\n\n## Reviews\n" + ([.reviews[] | "**" + .author.login + " (" + .state + "):** " + .body] | join("\n\n"))`).Output()
    if err != nil {
        // Try as issue if PR view fails
        out, err = exec.Command("gh", "issue", "view", sourceRef, "--json",
            "title,body,comments", "--jq",
            `"# " + .title + "\n\n" + .body + "\n\n## Comments\n" + ([.comments[] | "**" + .author.login + ":** " + .body] | join("\n\n"))`).Output()
        if err != nil {
            return "", fmt.Errorf("gh view %s: %w", sourceRef, err)
        }
    }

    return strings.TrimSpace(string(out)), nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/refresh/sources/github/`

- [ ] **Step 3: Commit**

```bash
git add internal/refresh/sources/github/context.go
git commit -m "Add GitHub ContextFetcher — fetches PR/issue body and comments"
```

### Task 9: Gmail ContextFetcher

**Files:**
- Create: `internal/refresh/sources/gmail/context.go`

- [ ] **Step 1: Implement FetchContext**

Gmail source_ref is a message ID. Fetch the full thread containing that message.

```go
// internal/refresh/sources/gmail/context.go
package gmail

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/anutron/claude-command-center/internal/config"
    gm "google.golang.org/api/gmail/v1"
)

// ContextFetcherImpl fetches email thread content from Gmail.
type ContextFetcherImpl struct {
    Cfg config.GmailConfig
}

func NewContextFetcher(cfg config.GmailConfig) *ContextFetcherImpl {
    return &ContextFetcherImpl{Cfg: cfg}
}

func (f *ContextFetcherImpl) ContextTTL() time.Duration { return 24 * time.Hour }

func (f *ContextFetcherImpl) FetchContext(sourceRef string) (string, error) {
    ts, err := loadGmailAuth(f.Cfg.Advanced)
    if err != nil {
        return "", fmt.Errorf("gmail auth: %w", err)
    }

    ctx := context.Background()
    client, err := NewSafeClient(ctx, ts, f.Cfg.Advanced)
    if err != nil {
        return "", fmt.Errorf("gmail client: %w", err)
    }

    // Get the message to find its thread ID
    msg, err := client.GetMessage(ctx, sourceRef, "metadata", "Subject", "From", "Date")
    if err != nil {
        return "", fmt.Errorf("get message %s: %w", sourceRef, err)
    }

    // Fetch the full thread using GetThread (read-only, safe)
    thread, err := client.GetThread(ctx, msg.ThreadId)
    if err != nil {
        // Fall back to single message body
        body, _ := client.GetMessageBody(ctx, sourceRef)
        return formatGmailMessage(msg, body), nil
    }

    var sb strings.Builder
    for _, m := range thread.Messages {
        body := extractTextBody(m.Payload)
        sb.WriteString(formatGmailMessage(m, body))
        sb.WriteString("\n---\n")
    }

    return strings.TrimSpace(sb.String()), nil
}

// formatGmailMessage extracts a readable representation from a Gmail message.
func formatGmailMessage(msg *gm.Message, body string) string {
    var subject, from, date string
    if msg.Payload != nil {
        for _, h := range msg.Payload.Headers {
            switch h.Name {
            case "Subject":
                subject = h.Value
            case "From":
                from = h.Value
            case "Date":
                date = h.Value
            }
        }
    }
    return fmt.Sprintf("From: %s\nDate: %s\nSubject: %s\n\n%s", from, date, subject, body)
}
```

**Prerequisite:** Add `GetThread` method to `SafeGmailClient` in `internal/refresh/sources/gmail/client.go`. This is a read-only operation and follows the existing safety pattern:

```go
// GetThread fetches all messages in a thread (read-only).
func (c *SafeGmailClient) GetThread(ctx context.Context, threadID string) (*gmail.Thread, error) {
    return c.svc.Users.Threads.Get("me", threadID).
        Format("full").
        Context(ctx).
        Do()
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/refresh/sources/gmail/`

- [ ] **Step 3: Commit**

```bash
git add internal/refresh/sources/gmail/context.go
git commit -m "Add Gmail ContextFetcher — fetches email thread content"
```

---

## Chunk 4: CLI Command and Refresh Integration

### Task 10: Add --fetch-context CLI flag

**Files:**
- Modify: `cmd/ccc/todo.go`

- [ ] **Step 1: Extend runTodo with --fetch-context flag**

```go
func runTodo(args []string) error {
    fs := flag.NewFlagSet("todo", flag.ContinueOnError)
    getID := fs.String("get", "", "Get todo by display_id (integer)")
    fetchContextID := fs.String("fetch-context", "", "Fetch and cache source context for todo by display_id")

    if err := fs.Parse(args); err != nil {
        return err
    }

    if *getID == "" && *fetchContextID == "" {
        return fmt.Errorf("usage: ccc todo --get <display_id> | --fetch-context <display_id>")
    }

    if *getID != "" {
        // ... existing --get logic unchanged ...
    }

    if *fetchContextID != "" {
        return runFetchContext(*fetchContextID)
    }

    return nil
}

func runFetchContext(idStr string) error {
    displayID, err := strconv.Atoi(idStr)
    if err != nil {
        return fmt.Errorf("invalid display_id %q: must be an integer", idStr)
    }

    database, err := db.OpenDB(config.DBPath())
    if err != nil {
        return fmt.Errorf("open database: %w", err)
    }
    defer database.Close()

    todo, err := db.DBLoadTodoByDisplayID(database, displayID)
    if err != nil {
        return fmt.Errorf("query todo: %w", err)
    }
    if todo == nil {
        return fmt.Errorf("no todo with display_id %d", displayID)
    }

    // Build context registry with all source fetchers
    registry := buildContextRegistry()

    ctx := context.Background()
    content, err := refresh.FetchAndSave(ctx, database, registry, todo)
    if err != nil {
        return fmt.Errorf("fetch context: %w", err)
    }

    fmt.Print(content)
    return nil
}
```

The `buildContextRegistry()` function creates a `ContextRegistry` and registers all source fetchers. It needs to load auth configs for sources that need them (Slack bot token, Gmail config). This follows the same pattern as `cmd/ccc-refresh/main.go` for initializing sources.

- [ ] **Step 2: Add buildContextRegistry function**

```go
func buildContextRegistry() *refresh.ContextRegistry {
    reg := refresh.NewContextRegistry()

    // Granola: no auth needed at registration time
    reg.Register("granola", granola.NewContextFetcher())

    // Slack: needs bot token
    cfg := config.Load()
    if cfg.Slack.BotToken != "" {
        reg.Register("slack", slack.NewContextFetcher(cfg.Slack.BotToken))
    }

    // GitHub: no auth needed (uses gh CLI)
    reg.Register("github", github.NewContextFetcher())

    // Gmail: needs config
    if cfg.Gmail.Enabled {
        reg.Register("gmail", gmail.NewContextFetcher(cfg.Gmail))
    }

    return reg
}
```

- [ ] **Step 3: Add required imports**

Add imports for `context`, `refresh`, and the source packages.

- [ ] **Step 4: Verify it compiles**

Run: `go build ./cmd/ccc/`

- [ ] **Step 5: Test manually**

Run: `ccc todo --fetch-context 40`
Expected: Prints the Granola transcript for todo #40

- [ ] **Step 6: Commit**

```bash
git add cmd/ccc/todo.go
git commit -m "Add --fetch-context CLI flag to ccc todo command"
```

### Task 11: Integrate context fetch into refresh pipeline

**Files:**
- Modify: `internal/refresh/refresh.go:129-132`

- [ ] **Step 1: Add context registry parameter to Options**

Add to the `Options` struct:

```go
ContextRegistry *ContextRegistry // for fetching source context on new todos
```

- [ ] **Step 2: Add context fetch after prompt generation**

After the prompt generation block (line 132), add:

```go
// Fetch source context for todos that need it
if opts.ContextRegistry != nil && opts.DB != nil {
    for i := range merged.Todos {
        FetchContextBestEffort(ctx, opts.DB, opts.ContextRegistry, &merged.Todos[i])
    }
}
```

- [ ] **Step 3: Wire up registry in ccc-refresh main**

In `cmd/ccc-refresh/main.go`, build the context registry and pass it in `Options.ContextRegistry`. Follow the same pattern as `buildContextRegistry()` from Task 10.

- [ ] **Step 4: Run full test suite**

Run: `go test ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/refresh/refresh.go cmd/ccc-refresh/main.go
git commit -m "Fetch source context during refresh for new todos"
```

### Task 12: Backfill existing active todos

- [ ] **Step 1: List active todos**

Run: `ccc todo --get <id>` for each active todo to identify which ones need context.

- [ ] **Step 2: Fetch context for each**

```bash
ccc todo --fetch-context 40
ccc todo --fetch-context 43
# ... repeat for all active todos with source_ref
```

- [ ] **Step 3: Verify by checking a todo**

Run: `ccc todo --get 40`
Verify `source_context` and `source_context_at` fields are populated in the JSON output.

---

## Chunk 5: Prompt Template Integration

### Task 13: Add source context to routing prompt

**Files:**
- Modify: `internal/refresh/todo_agent.go:57-143`

- [ ] **Step 1: Add source context to buildRoutingPrompt**

After the existing todo fields section (around line 81), add:

```go
if todo.SourceContext != "" {
    b.WriteString("\n## Source Context\n")
    fmt.Fprintf(&b, "Source: %s (ref: %s)\n", todo.Source, todo.SourceRef)
    if todo.SourceContextAt != "" {
        fmt.Fprintf(&b, "Fetched: %s\n", todo.SourceContextAt)
    }
    b.WriteString("\n<source_context>\n")
    b.WriteString(todo.SourceContext)
    b.WriteString("\n</source_context>\n")
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/refresh/...`

- [ ] **Step 3: Commit**

```bash
git add internal/refresh/todo_agent.go
git commit -m "Include source context in todo routing prompt"
```

### Task 14: Build and install

- [ ] **Step 1: Build both binaries**

Run: `make build`

- [ ] **Step 2: Install**

Run: `make install`

- [ ] **Step 3: Final verification**

Run: `ccc todo --fetch-context 40` and verify output.
Run: `ccc todo --get 40 | jq '.source_context' | head -5` to verify it's stored.
