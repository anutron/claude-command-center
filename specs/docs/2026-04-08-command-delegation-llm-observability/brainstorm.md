# Command Delegation & LLM Activity Observability

## Problem

CCC has two tiers of AI activity — daemon-managed agents (visible in console) and inline LLM calls via `llm.Complete` (invisible). This creates three problems:

1. **Silent failure on commands requiring external data.** The command LLM (`c` key) has no tool access. When a user asks it to "read my Granola call and extract todos," it spins for a minute trying to generate a response from CC state JSON alone, then silently produces nothing. The user has no feedback that it can't do what was asked.

2. **No observability for inline LLM calls.** There are 18 `llm.Complete` call sites across the codebase (9 in commandcenter, 1 in sessions, 8 in refresh). None emit events. The console overlay and `ccc console` only show daemon-managed agents. A user watching the console has no idea these calls are happening or costing money.

3. **Anxiety gap.** The user opens `ccc console` for peace of mind — to see everything that's spending money and confirm it stops when expected. But a significant category of activity is invisible there.

## Goals

- When the command LLM can't fulfill a request (needs external tools/data), it delegates to a real agent that appears in the console immediately
- All `llm.Complete` calls are visible in `ccc console` and the `~` overlay — start, finish, duration, operation name
- No new database tables; LLM activity is ephemeral (in-memory ring buffer on daemon)

## Non-Goals

- Cost tracking for individual LLM calls (we don't get token counts back from `llm.Complete` today)
- Controlling or cancelling LLM calls from the console
- Changing the refresh pipeline's LLM usage

## Design

### Feature 1: Command LLM Delegation

#### Prompt Change

Add a 6th allowed action to `buildCommandPromptWithHistory` in `claude_exec.go`:

> 6. **Delegate to agent** — if the instruction requires reading external data (Granola transcripts, Slack messages, emails, files, GitHub PRs), performing real work (writing code, sending messages), or anything you cannot answer from the command center state below, set `"delegate"` with a rewritten prompt for the agent.

Add to the decision logic:

> 6. If the instruction requires external data or tools you don't have -> delegate to an agent

Add to the "What you must NEVER do" section:

> - Do NOT attempt to answer questions about data you don't have (Granola calls, Slack threads, emails, files). Delegate those immediately.

#### Response Format Change

Add `"delegate"` field to the JSON response format:

```json
{
  "message": "Launching agent to read your Granola call...",
  "delegate": {
    "prompt": "Read the most recent Granola call with Zach and create CCC todos (via `ccc add-todo`) for every action item. Assign all of them to Aaron — not just the ones where he explicitly says 'I will', but every action item from the call.",
    "project_dir": ""
  },
  "todos": [],
  "complete_todo_ids": []
}
```

The `delegate.prompt` is a rewritten version of the user's instruction — expanded for clarity, with enough context for a Claude Code agent to execute it. The command LLM's job is to be a good prompt writer here: take the user's terse instruction and produce an unambiguous agent prompt.

`delegate.project_dir` defaults to empty (agent will use `$HOME`). The command LLM can set it if the instruction implies a specific project.

#### Handler Change (`cc_messages.go`)

In `handleClaudeCommandFinished`, after JSON parsing, check for `resp.Delegate` before processing todos:

1. Create a todo with title derived from the delegate prompt (first ~60 chars or a summary)
2. Set `todo.Detail` to the full delegate prompt
3. Set `todo.ProjectDir` to delegate's project_dir (or `$HOME` if empty)
4. Insert into DB
5. Call `p.launchOrQueueAgent()` with a `queuedSession` built from the todo
6. Flash message: "Delegating to agent: <truncated title>"

The todo appears in the list immediately, the agent appears in the console, and the user can observe it working.

#### Edge Cases

- **Delegation + todos in same response:** The command LLM might delegate AND create some simple todos in the same response (e.g., "read my Granola call for action items, also remind me to buy milk"). Handle both: create the simple todos, then launch the agent for the delegated work.
- **Delegation + ask:** If `delegate` and `ask` are both set, prefer `ask` (the LLM needs clarification before it can delegate).
- **Empty delegate prompt:** Ignore — treat as no delegation.
- **Daemon not connected:** Flash "Daemon not connected — cannot delegate to agent" (same as existing `launchOrQueueAgent` behavior).

### Feature 2: LLM Activity Events

#### Event Types

Two new event types on the plugin event bus:

- `llm.started` — published when an `llm.Complete` call begins
- `llm.finished` — published when an `llm.Complete` call returns (success or error)

Payload for both:

```go
map[string]interface{}{
    "id":        "<uuid>",       // unique ID for this call, links started→finished
    "operation": "command",      // operation name: command, edit, enrich, focus, date-parse, synthesize, review-address, refine, train, describe, thread-extract, todo-route, todo-synthesis, slack-summarize, gmail-summarize, granola-summarize
    "source":    "commandcenter", // plugin or subsystem name
    "todo_id":   "abc123",       // optional, if associated with a specific todo
}
```

`llm.finished` adds:

```go
map[string]interface{}{
    // ...same fields as started...
    "duration_ms": 3200,
    "error":       "",    // empty on success, error message on failure
}
```

#### Instrumentation Approach

Wrap the LLM call sites. Rather than modifying every call site, create a helper in `claude_exec.go`:

```go
func (p *Plugin) llmCompleteWithEvents(ctx context.Context, operation string, todoID string) (string, error) {
    id := uuid.New().String()
    p.publishEvent("llm.started", map[string]interface{}{
        "id": id, "operation": operation, "source": "commandcenter", "todo_id": todoID,
    })
    out, err := p.llm.Complete(ctx, prompt)
    errMsg := ""
    if err != nil { errMsg = err.Error() }
    p.publishEvent("llm.finished", map[string]interface{}{
        "id": id, "operation": operation, "source": "commandcenter", "todo_id": todoID,
        "duration_ms": ..., "error": errMsg,
    })
    return out, err
}
```

**Problem:** The `claude_exec.go` functions are free functions that return `tea.Cmd` closures. They don't have access to the plugin's event bus. Two options:

- **A) Pass the bus into each function.** Adds a parameter but keeps functions pure.
- **B) Wrap at the call site.** Each place that calls `claudeCommandCmd(...)` also publishes events before/after. Messy — the "after" happens in the message handler, not at the call site.
- **C) Wrap the LLM interface.** Create an `ObservableLLM` that wraps `llm.LLM`, publishes events on every `Complete` call. The plugin injects it at init time.

**Recommendation: C.** An `ObservableLLM` wrapper is clean, covers all call sites automatically (including refresh pipeline calls), and requires no changes to existing function signatures. It lives in `internal/llm/observable.go`.

```go
type ObservableLLM struct {
    inner    LLM
    publish  func(topic string, payload map[string]interface{})
    source   string
}

func (o *ObservableLLM) Complete(ctx context.Context, prompt string) (string, error) {
    id := generateID()
    op := inferOperation(prompt) // or passed via context
    o.publish("llm.started", ...)
    out, err := o.inner.Complete(ctx, prompt)
    o.publish("llm.finished", ...)
    return out, err
}
```

**Operation name problem:** The wrapper doesn't know the operation name ("command", "focus", "enrich"). Options:

- Pass it via `context.WithValue` — idiomatic Go, no signature changes
- Accept that the wrapper can't distinguish operations and use a generic label — loses the useful operation name

**Recommendation:** Use context. Define `llm.WithOperation(ctx, "command")` and `llm.OperationFrom(ctx)`. Each call site already has a context (or can trivially add one). This keeps the wrapper clean and gives us operation names everywhere.

**Refresh pipeline coverage:** The refresh LLM calls (`internal/refresh/llm.go`, `sources/slack/llm.go`, etc.) also go through `llm.LLM`. If the TUI passes an `ObservableLLM` to the refresh pipeline, those calls get instrumented for free. But the refresh pipeline runs both in the TUI (via `ai-cron` invoked from the plugin) and standalone. When run standalone, no event bus exists — the `ObservableLLM` would need a no-op publish function. This is fine; the wrapper gracefully degrades.

**However:** `ai-cron` runs as a separate process, not inside the TUI. It creates its own LLM instance. So the TUI's `ObservableLLM` wrapper won't cover refresh-pipeline calls. To cover those, `ai-cron` would need to either:
- Report LLM activity to the daemon via RPC (new `ReportLLMActivity` RPC), or
- Emit events to a shared channel the daemon can read

This is a bigger lift. **For v1, instrument only the TUI-side LLM calls** (the 9 in commandcenter + 1 in sessions). The refresh pipeline calls happen on a schedule and are fast — they're lower priority for observability. We can add them later by having `ai-cron` report to the daemon.

### Feature 3: Daemon LLM Activity Ring Buffer

#### New Daemon RPC: `ReportLLMActivity`

The TUI sends LLM activity events to the daemon so they're accessible to all consumers (`ccc console`, other TUI instances).

```go
type LLMActivityEvent struct {
    ID         string `json:"id"`
    Operation  string `json:"operation"`
    Source     string `json:"source"`
    TodoID     string `json:"todo_id,omitempty"`
    StartedAt  time.Time `json:"started_at"`
    FinishedAt *time.Time `json:"finished_at,omitempty"`
    DurationMs int    `json:"duration_ms,omitempty"`
    Error      string `json:"error,omitempty"`
    Status     string `json:"status"` // "running", "completed", "failed"
}
```

#### Ring Buffer

Daemon holds an in-memory ring buffer of the last 100 LLM activity events. No DB persistence — these are ephemeral heartbeats.

```go
type llmActivityBuffer struct {
    mu      sync.Mutex
    entries []LLMActivityEvent
    max     int // 100
}
```

On `llm.started`: insert a new entry with `Status: "running"`.
On `llm.finished`: find the matching entry by ID, update `FinishedAt`, `DurationMs`, `Error`, `Status`.

#### New Daemon RPC: `ListLLMActivity`

Returns the current ring buffer contents. Used by `ccc console` and the `~` overlay.

#### Broadcast

On receiving `ReportLLMActivity`, the daemon broadcasts `llm.started` / `llm.finished` events to all subscribers. This lets the `~` overlay in other TUI instances react without polling.

#### Flow

```
TUI plugin
  → llm.Complete starts
  → ObservableLLM publishes "llm.started" on event bus
  → TUI event bus handler calls daemon.ReportLLMActivity(started)
  → Daemon stores in ring buffer, broadcasts "llm.started"
  → ccc console (subscribed) sees the event, re-polls ListLLMActivity
  → ~overlay (if open, subscribed) sees the event, re-polls ListLLMActivity

TUI plugin
  → llm.Complete returns
  → ObservableLLM publishes "llm.finished" on event bus
  → TUI event bus handler calls daemon.ReportLLMActivity(finished)
  → Daemon updates ring buffer entry, broadcasts "llm.finished"
  → consumers update
```

### Feature 4: Console Rendering

#### `~` Overlay

Add an "LLM Activity" section below the agent list. Only shown when there are in-flight LLM calls or recently completed ones (last 30 seconds).

```
┌─ Agent Console ──────────────────────────────────────────┐
│                                                          │
│  ● TODO #114 — Read Granola call with Zach    2m12s $0.38│
│  ✓ PR #47 — Review auth changes               4m01s $0.52│
│                                                          │
│  ── llm activity ──                                      │
│  ⠋ command  (12s)                                        │
│  ✓ focus    (3s)                                         │
│  ✓ enrich   (2s)                                         │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

LLM activity entries show: status icon, operation name, elapsed/duration. Completed entries fade out after 30 seconds. Running entries show a spinner.

The LLM section is not navigable — it's informational only. `j`/`k` still navigate the agent list.

#### `ccc console`

Add LLM activity to the sidebar below the agent list, with a separator:

```
┌─ Sidebar ──────────┬─ Focus Pane ─────────────────────────┐
│ AGENTS             │ TODO #114 — Read Granola call         │
│                    │ ──────────────────────────────────────│
│ ● TODO #114 grano… │ ⠋ Reading Granola transcript...      │
│                    │ → Found call with Zach (2026-04-07)   │
│ ── completed ──    │ → Extracting action items...          │
│ ✓ TODO #91 refac   │                                      │
│                    │                                       │
│ ── llm ──          │                                       │
│ ⠋ command   12s    │                                       │
│ ✓ focus      3s    │                                       │
│ ✓ enrich     2s    │                                       │
└────────────────────┴───────────────────────────────────────┘
```

LLM entries in the sidebar are not selectable (no focus pane content for them). They're status indicators only. The separator and entries are dimmed to visually distinguish them from agents.

`ccc console` fetches LLM activity via the new `ListLLMActivity` RPC on each poll tick (already polls every 1 second).

## Summary of Changes by File

- `internal/llm/observable.go` — new file: `ObservableLLM` wrapper, context helpers
- `internal/llm/llm.go` — add `WithOperation`/`OperationFrom` context helpers
- `internal/builtin/commandcenter/claude_exec.go` — update command prompt with delegation action; wrap LLM calls with operation context
- `internal/builtin/commandcenter/cc_messages.go` — handle `delegate` response; wire event bus → daemon for LLM events
- `internal/builtin/commandcenter/commandcenter.go` — init `ObservableLLM` wrapper
- `internal/daemon/daemon.go` — add `ReportLLMActivity` handler, ring buffer, broadcast
- `internal/daemon/client.go` — add `ReportLLMActivity`, `ListLLMActivity` client methods
- `internal/daemon/types.go` — add `LLMActivityEvent` type
- `internal/tui/console_overlay.go` — add LLM activity section rendering
- `internal/tui/model.go` — handle `llm.started`/`llm.finished` daemon events for overlay; forward plugin LLM events to daemon
- `cmd/ccc/console.go` — fetch and render LLM activity in sidebar
