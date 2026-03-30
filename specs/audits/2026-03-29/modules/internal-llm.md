# Spec Audit: internal/llm

**Date:** 2026-03-29
**Spec:** specs/core/llm.md

---

## internal/llm/llm.go

### LLM interface

- **[COVERED]** llm.md "Interface": `Complete(ctx, prompt) (string, error)` — exact match

### Available()

- **[COVERED]** llm.md "Available() bool": "checks whether the claude CLI binary is on PATH"

---

## internal/llm/claude_cli.go

### ClaudeCLI struct fields

- **[COVERED]** llm.md "ClaudeCLI": Model, Timeout, Tools, DisableSlashCommands — all documented with exact behavior
- **[COVERED]** llm.md: "buildArgs() method extracts arg construction for testability"

### ClaudeCLI.Complete

- **[COVERED]** llm.md: "Shells out to claude -p <prompt> --output-format text"
- **[COVERED]** llm.md: "Timeout field — if >0 and incoming ctx has no deadline, wraps with context.WithTimeout"
- **[COVERED]** llm.md: "Trims whitespace from output"
- **[COVERED]** llm.md: "Wraps exec errors with exit code and stderr"

### ParseClaudeError

- **[COVERED]** llm.md "Error Parsing" section: JSON API errors, 500/529 patterns, truncation, empty fallback — all match code exactly

---

## internal/llm/noop.go

### NoopLLM

- **[COVERED]** llm.md "NoopLLM": `Returns "", nil for every call`
- **[COVERED]** llm.md: "Used when --no-llm flag is set or claude binary is not found"

---

## internal/llm/failure_log.go

### FailureEntry / LogFailure

- **[COVERED]** llm.md "Failure Logging": JSON-lines format, fields (timestamp, operation, prompt, error, todo_id), append behavior
- **[COVERED]** llm.md: "Log path: ~/.config/ccc/data/llm-failures.jsonl"

---

## Spec -> Code Direction Gaps

1. **llm.md "Integration Points"** documents cmd/ai-cron constructing two ClaudeCLI instances (haiku + sonnet) — verified in cmd/ai-cron/main.go.
2. **llm.md "cmd/ccc (TUI)"** documents sandboxed ClaudeCLI with Timeout:90s, Tools:ptr(""), DisableSlashCommands:true — verified in cmd/ccc/main.go.
3. **llm.md "cmd/ccc paths --auto-describe"** documents same sandboxed ClaudeCLI — verified in cmd/ccc/paths.go.
4. **llm.md "Two-Tier Architecture"** (haiku extraction, sonnet routing/validation) — verified in cmd/ai-cron/main.go.
5. **llm.md operations tagged** — not verifiable from llm package alone (tags are in call sites), no gap.

No spec-to-code gaps found.

---

## Summary

- **CONTRADICTS: 0**
- **UNCOVERED-BEHAVIORAL: 0**
- **COVERED: 11 behavioral paths** — This is the best-covered module in the audit. The spec and code are in tight alignment.
