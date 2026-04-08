# SPEC: LLM Abstraction Layer

## Purpose

Abstract LLM calls behind an interface so the codebase has a single integration point for language model completions. This enables:
- Swapping providers without changing call sites
- Disabling LLM features cleanly (--no-llm flag, missing binary)
- Testing without real LLM calls
- Timeout safety nets and sandboxing for TUI calls
- Structured failure logging for debugging and manual retry

## Interface

### `LLM`

```go
type LLM interface {
    Complete(ctx context.Context, prompt string) (string, error)
}
```

- **Inputs**: context for cancellation/timeout, prompt string
- **Outputs**: trimmed response text, error
- **Dependencies**: none (interface only)

### `Available() bool`

Package-level function that checks whether the `claude` CLI binary is on PATH.

## Implementations

### ClaudeCLI

- Shells out to `claude -p <prompt> --output-format text`
- Optional `Model` field (e.g., `"haiku"`, `"sonnet"`) — passed via `--model` flag when set
- `Timeout` field — if >0 and incoming ctx has no deadline, wraps with `context.WithTimeout`
- `Tools` field — `*string`; nil = don't pass flag; `ptr("")` = pass `--tools ""`
- `DisableSlashCommands` field — when true, adds `--disable-slash-commands`
- `buildArgs()` method extracts arg construction for testability
- Trims whitespace from output
- Wraps exec errors with exit code and stderr

### NoopLLM

- Returns `""`, `nil` for every call
- Used when `--no-llm` flag is set or `claude` binary is not found

### ObservableLLM

- Wraps any `LLM` and publishes observability events via a `PublishFunc` callback
- Constructor: `NewObservableLLM(inner LLM, publish PublishFunc, source string) *ObservableLLM`
- On each `Complete` call:
  1. Generates a UUID (v4, crypto/rand, no external deps)
  2. Reads operation name from context via `OperationFrom(ctx)`, defaults to `"unknown"`
  3. Publishes `"llm.started"` with `{id, operation, source}`
  4. Delegates to inner LLM
  5. Publishes `"llm.finished"` with `{id, operation, source, duration_ms, status, error?}`
  6. `status` is `"completed"` on success, `"failed"` on error
- Context helpers:
  - `WithOperation(ctx, op) context.Context` — attaches operation name to context
  - `OperationFrom(ctx) string` — extracts operation name, empty string if not set
- Types:
  - `EventPayload = map[string]interface{}`
  - `PublishFunc = func(topic string, payload EventPayload)`

## Error Parsing

### `ParseClaudeError(stderr string) string`

Extracts a human-readable message from Claude CLI error output:
- JSON API errors → `"Claude API error: <type>"` (e.g., `overloaded_error`)
- Patterns with 500/529 → `"Claude API error (500)"` / `"Claude API overloaded (529)"`
- Long messages → truncated to ~80 chars
- Empty → `"unknown error"`

## Failure Logging

### `FailureEntry` struct + `LogFailure(logPath, entry)`

- Appends one JSON object per line (JSON-lines format, `.jsonl`) to the given path
- Fields: `timestamp`, `operation`, `prompt`, `error`, `todo_id` (optional)
- Log path: `~/.config/ccc/data/llm-failures.jsonl`
- All TUI LLM call sites log on error before returning the message

Operations tagged: `edit`, `enrich`, `command`, `date-parse`, `review-address`, `refine`, `train`, `focus`

## Integration Points

### plugin.Context

`LLM` field added to `plugin.Context` (typed as `interface{}` to avoid circular imports, same pattern as `Styles` and `Grad`).

### refresh.Options

`LLM` field added to `refresh.Options` (typed as `llm.LLM`). The refresh functions `extractCommitments`, `extractSlackCommitments`, and `generateSuggestions` accept `llm.LLM` as a parameter instead of calling `callClaude` directly.

### cmd/ai-cron

Constructs two `ClaudeCLI` instances: `{Model: "haiku"}` for extraction/suggestions and `{Model: "sonnet"}` for routing/validation. Falls back to `NoopLLM` if `--no-llm` or `claude` not available. Passes `LLM` (haiku) and `RoutingLLM` (sonnet) into `refresh.Options`. **No sandboxing** — refresh is a background process with its own error handling.

### cmd/ccc (TUI)

Constructs sandboxed `ClaudeCLI` with:
- `Timeout: 90s` — safety net for hung API calls
- `Tools: ptr("")` — disables all tool use (pure text completion)
- `DisableSlashCommands: true` — prevents slash command interpretation

### cmd/ccc paths --auto-describe

Same sandboxed `ClaudeCLI` as TUI (also pure text completion).

### commandcenter plugin

The `claudeEditCmd`, `claudeEnrichCmd`, `claudeCommandCmd`, `claudeFocusCmd`, `claudeTrainCmd`, `claudeDateParseCmd`, `claudeRefinePromptCmd`, `claudeReviewAddressCmd` functions accept `llm.LLM` and use it instead of shelling out to `claude` directly.

All handlers show flash messages on error:
- `handleClaudeEditFinished` → `"Edit failed: <parsed error>"` or `"Edit returned invalid JSON"`
- `handleClaudeEnrichFinished` → `"Enrich failed: <parsed error>"`
- `handleClaudeFocusFinished` → `"Focus failed: <parsed error>"`
- `handleClaudeCommandFinished` → `"Command failed: <error>"` (already existed)
- `handleClaudeTrainFinished` → `"Training failed: <error>"` (already existed)

### Elapsed time on loading spinner

All loading indicators show `"<message> (Xs)"` with a live ticking timer so users know the TUI hasn't frozen. The `claudeLoadingAt` field is set alongside `claudeLoading = true` at all call sites.

## Behavior

1. On startup, check `Available()` (or `--no-llm` flag)
2. Construct `ClaudeCLI{Model: "haiku"}` for extraction, `ClaudeCLI{Model: "sonnet"}` for routing (or `NoopLLM{}` for both)
3. TUI constructs sandboxed `ClaudeCLI` with timeout + `--tools ""` + `--disable-slash-commands`
4. Pass the chosen implementations through Options/Context
5. All LLM call sites use the interface — never shell out directly
6. On error, log to `llm-failures.jsonl` and show flash message to user

## Two-Tier Architecture

The refresh pipeline uses two LLM tiers:

- **Haiku** (cheap, wide net): Extracts candidate todos from meetings/Slack/Gmail. May produce false positives.
- **Sonnet** (accurate, quality gate): Routes todos to projects and validates ownership. Can REJECT todos that aren't Aaron's, auto-dismissing them. Also writes the actionable prompt.

## Test Cases

- `NoopLLM.Complete()` returns `""`, `nil`
- `Available()` returns `true` when `claude` is on PATH, `false` otherwise
- `ClaudeCLI` implements the `LLM` interface (compile-time check)
- `buildArgs()` with no fields → basic args only
- `buildArgs()` with timeout → no change to args (wraps context)
- `buildArgs()` with sandbox flags → includes `--tools ""` and `--disable-slash-commands`
- `buildArgs()` with model + sandbox → all flags present
- `ParseClaudeError` with JSON API error → extracts error type
- `ParseClaudeError` with plain text 500 → `"Claude API error (500)"`
- `ParseClaudeError` with empty string → `"unknown error"`
- `ParseClaudeError` with long message → truncated to ~80 chars
- `LogFailure` writes valid JSON-lines, all fields present, appends correctly
- `WithOperation` / `OperationFrom` round-trip stores and retrieves operation name
- `OperationFrom` on bare context returns empty string
- `ObservableLLM.Complete` publishes `llm.started` then `llm.finished` with matching IDs
- `ObservableLLM.Complete` on error sets status `"failed"` with error message
- `ObservableLLM.Complete` defaults operation to `"unknown"` when context has no operation
- `ObservableLLM` generates unique IDs across multiple calls
- `ObservableLLM` implements the `LLM` interface (compile-time check)
