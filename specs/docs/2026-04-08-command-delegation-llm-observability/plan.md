# Command Delegation & LLM Activity Observability — Implementation Plan

**Goal:** Make all CCC AI activity observable — command LLM delegates to real agents when it can't fulfill a request, and all `llm.Complete` calls are visible in the console overlay and `ccc console`.

**Design doc:** `specs/docs/2026-04-08-command-delegation-llm-observability/brainstorm.md` — Read this first. It contains the architecture, event flow, and design decisions that inform every stage below. This plan describes execution order; the design doc describes what to build and why.

**Assumptions and boundaries:**

- In scope: command delegation, LLM activity events, console rendering for both surfaces
- Out of scope: `ai-cron` LLM instrumentation (separate process, deferred), cost tracking for individual LLM calls, cancellation of LLM calls
- Relying on: existing daemon RPC infrastructure, event bus, `ObservableLLM` wrapper pattern

## Stages

### Stage 1: Update specs

Update `specs/builtin/command-center.md` to document:
- The `delegate` response field in the command LLM response format
- The delegation flow (command LLM → todo creation → agent launch)
- New decision logic entry: "requires external data → delegate"

Update `specs/builtin/console.md` to document:
- LLM activity section in the `~` overlay (below agent list, informational only)
- LLM activity entries in `ccc console` sidebar (below agents, not selectable)
- `llm.started` / `llm.finished` event types
- `ListLLMActivity` and `ReportLLMActivity` daemon RPCs
- Ring buffer behavior (100 entries, in-memory, no DB)

### Stage 2: Write failing tests

Write tests that validate the new behavior before implementation:

- **Command delegation test** (`commandcenter_test.go`): Mock LLM returns a `delegate` response → verify todo is created, `launchOrQueueAgent` is called, flash message shown
- **Delegation + todos test**: Mock LLM returns both `delegate` and `todos` → verify both are handled
- **Delegation + ask test**: Mock LLM returns both `delegate` and `ask` → verify `ask` takes priority
- **ObservableLLM test** (`internal/llm/observable_test.go`): Verify `llm.started` and `llm.finished` events are published with correct operation name, ID, duration, error status
- **Ring buffer test** (`internal/daemon/llm_activity_test.go`): Verify insert, update on finish, max capacity eviction, `ListLLMActivity` returns correct entries
- **Console overlay view test** (`model_view_test.go`): Verify LLM activity section renders when entries exist, doesn't render when empty
- **`ccc console` sidebar test** (`console_test.go`): Verify LLM activity entries appear in sidebar below agents

### Stage 3: ObservableLLM wrapper + context helpers

**Depends on:** Stage 2

New file `internal/llm/observable.go`:
- `WithOperation(ctx, name)` / `OperationFrom(ctx)` context helpers
- `ObservableLLM` struct wrapping `llm.LLM` with a publish callback
- On `Complete`: generate UUID, publish `llm.started`, call inner, publish `llm.finished` with duration and error

New file `internal/llm/observable_test.go` for unit tests.

This is a standalone vertical slice — no dependencies on daemon or UI changes.

### Stage 4: Daemon ring buffer + RPCs

**Depends on:** Stage 2

- Add `LLMActivityEvent` type to `internal/daemon/types.go`
- Add `llmActivityBuffer` to daemon server struct (in-memory, cap 100)
- Add `ReportLLMActivity` RPC handler: insert/update ring buffer, broadcast event
- Add `ListLLMActivity` RPC handler: return buffer contents
- Add client methods: `ReportLLMActivity()`, `ListLLMActivity()`

This is a standalone vertical slice — no dependencies on the LLM wrapper or UI.

### Stage 5: Command LLM delegation

**Depends on:** Stage 2

- Update `buildCommandPromptWithHistory` in `claude_exec.go`: add delegation action, decision logic, response format field
- Update `handleClaudeCommandFinished` in `cc_messages.go`: parse `delegate` field, create todo, call `launchOrQueueAgent()`
- Handle edge cases: delegate+todos, delegate+ask, empty delegate

This is a standalone vertical slice — it works independently of the observability features.

### Stage 6: Wire TUI to daemon for LLM events

**Depends on:** Stage 3, Stage 4

- In `commandcenter.go` init: wrap the plugin's LLM with `ObservableLLM`, using a publish callback that sends events to the event bus
- In `internal/tui/model.go`: handle `llm.started` / `llm.finished` events from the plugin event bus, forward to daemon via `ReportLLMActivity` RPC
- Handle daemon `llm.started` / `llm.finished` broadcast events (re-fetch LLM activity for overlay)

### Stage 7: Console rendering — overlay + `ccc console`

**Depends on:** Stage 4, Stage 6

- `internal/tui/console_overlay.go`: add `llmActivity []LLMActivityEvent` field, render "── llm activity ──" section below agent list with status icon + operation + elapsed/duration. Completed entries shown for 30 seconds then hidden.
- `cmd/ccc/console.go`: fetch `ListLLMActivity` on each poll tick, render LLM entries in sidebar below agent separator. Dimmed style, not selectable.
