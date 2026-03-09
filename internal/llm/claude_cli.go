package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ClaudeCLI implements LLM by shelling out to the claude CLI.
type ClaudeCLI struct{}

// Compile-time interface check.
var _ LLM = ClaudeCLI{}

func (c ClaudeCLI) Complete(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "claude",
		"-p", prompt,
		"--output-format", "text",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
