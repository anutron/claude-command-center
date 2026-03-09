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

Constructs `ClaudeCLI` or `NoopLLM` based on `--no-llm` flag and `Available()`, passes into `refresh.Options`.

### cmd/ccc

Constructs `ClaudeCLI` or `NoopLLM` based on `Available()`, passes into `plugin.Context`.

### commandcenter plugin

The `claudeEditCmd`, `claudeEnrichCmd`, `claudeCommandCmd`, and `claudeFocusCmd` functions accept `llm.LLM` and use it instead of shelling out to `claude` directly.

## Behavior

1. On startup, check `Available()` (or `--no-llm` flag)
2. Construct `ClaudeCLI{}` if available, `NoopLLM{}` otherwise
3. Pass the chosen implementation through Options/Context
4. All LLM call sites use the interface — never shell out directly

## Test Cases

- `NoopLLM.Complete()` returns `""`, `nil`
- `Available()` returns `true` when `claude` is on PATH, `false` otherwise
- `ClaudeCLI` implements the `LLM` interface (compile-time check)
