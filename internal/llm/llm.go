package llm

import (
	"context"
	"os/exec"
)

// LLM abstracts language model completions behind a single interface.
type LLM interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Available returns true if the claude CLI binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}
