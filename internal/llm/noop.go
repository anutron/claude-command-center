package llm

import "context"

// NoopLLM returns empty strings for all completions.
// Used when --no-llm flag is set or claude binary is not found.
type NoopLLM struct{}

// Compile-time interface check.
var _ LLM = NoopLLM{}

func (n NoopLLM) Complete(ctx context.Context, prompt string) (string, error) {
	return "", nil
}
