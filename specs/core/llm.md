# SPEC: LLM Abstraction Layer

## Purpose

Abstract LLM calls behind an interface so the codebase has a single integration point for language model completions. This enables:
- Swapping providers without changing call sites
- Disabling LLM features cleanly (--no-llm flag, missing binary)
- Testing without real LLM calls

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
- Trims whitespace from output
- Wraps exec errors with exit code and stderr

### NoopLLM

- Returns `""`, `nil` for every call
- Used when `--no-llm` flag is set or `claude` binary is not found

## Integration Points

### plugin.Context

`LLM` field added to `plugin.Context` (typed as `interface{}` to avoid circular imports, same pattern as `Styles` and `Grad`).

### refresh.Options

`LLM` field added to `refresh.Options` (typed as `llm.LLM`). The refresh functions `extractCommitments`, `extractSlackCommitments`, and `generateSuggestions` accept `llm.LLM` as a parameter instead of calling `callClaude` directly.

### cmd/ccc-refresh

Constructs two `ClaudeCLI` instances: `{Model: "haiku"}` for extraction/suggestions and `{Model: "sonnet"}` for routing/validation. Falls back to `NoopLLM` if `--no-llm` or `claude` not available. Passes `LLM` (haiku) and `RoutingLLM` (sonnet) into `refresh.Options`.

### cmd/ccc

Constructs `ClaudeCLI` or `NoopLLM` based on `Available()`, passes into `plugin.Context`.

### commandcenter plugin

The `claudeEditCmd`, `claudeEnrichCmd`, `claudeCommandCmd`, and `claudeFocusCmd` functions accept `llm.LLM` and use it instead of shelling out to `claude` directly.

## Behavior

1. On startup, check `Available()` (or `--no-llm` flag)
2. Construct `ClaudeCLI{Model: "haiku"}` for extraction, `ClaudeCLI{Model: "sonnet"}` for routing (or `NoopLLM{}` for both)
3. Pass the chosen implementations through Options/Context
4. All LLM call sites use the interface — never shell out directly

## Two-Tier Architecture

The refresh pipeline uses two LLM tiers:

- **Haiku** (cheap, wide net): Extracts candidate todos from meetings/Slack/Gmail. May produce false positives.
- **Sonnet** (accurate, quality gate): Routes todos to projects and validates ownership. Can REJECT todos that aren't Aaron's, auto-dismissing them. Also writes the actionable prompt.

## Test Cases

- `NoopLLM.Complete()` returns `""`, `nil`
- `Available()` returns `true` when `claude` is on PATH, `false` otherwise
- `ClaudeCLI` implements the `LLM` interface (compile-time check)
