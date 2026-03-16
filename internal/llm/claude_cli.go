package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ClaudeCLI implements LLM by shelling out to the claude CLI.
// If Model is set (e.g., "haiku", "sonnet"), it is passed via --model.
type ClaudeCLI struct {
	Model string
}

// Compile-time interface check.
var _ LLM = ClaudeCLI{}

func (c ClaudeCLI) Complete(ctx context.Context, prompt string) (string, error) {
	args := []string{"-p", prompt, "--output-format", "text"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
