package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ClaudeCLI implements LLM by shelling out to the claude CLI.
// If Model is set (e.g., "haiku", "sonnet"), it is passed via --model.
type ClaudeCLI struct {
	Model string

	// Timeout wraps the context with a deadline if >0 and the incoming ctx has none.
	Timeout time.Duration

	// Tools controls the --tools flag. nil = don't pass; ptr("") = pass --tools "".
	Tools *string

	// DisableSlashCommands adds --disable-slash-commands when true.
	DisableSlashCommands bool
}

// Compile-time interface check.
var _ LLM = ClaudeCLI{}

func (c ClaudeCLI) buildArgs() []string {
	args := []string{"-p", "-", "--output-format", "text"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.Tools != nil {
		args = append(args, "--tools", *c.Tools)
	}
	if c.DisableSlashCommands {
		args = append(args, "--disable-slash-commands")
	}
	return args
}

func (c ClaudeCLI) Complete(ctx context.Context, prompt string) (string, error) {
	// Apply timeout if configured and ctx has no deadline.
	if c.Timeout > 0 {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.Timeout)
			defer cancel()
		}
	}

	args := c.buildArgs()
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ParseClaudeError extracts a human-readable message from Claude CLI error output.
// Returns e.g. "Claude API error (500)" for overloaded case, or truncates to ~80 chars.
func ParseClaudeError(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return "unknown error"
	}

	// Try to parse as JSON error from Claude API
	var apiErr struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(stderr), &apiErr) == nil && apiErr.Error.Type != "" {
		return fmt.Sprintf("Claude API error: %s", apiErr.Error.Type)
	}

	// Look for common patterns like "API Error: 500"
	if strings.Contains(stderr, "500") || strings.Contains(stderr, "overloaded") {
		return "Claude API error (500)"
	}
	if strings.Contains(stderr, "529") {
		return "Claude API overloaded (529)"
	}

	// Truncate long messages
	if len(stderr) > 80 {
		return stderr[:77] + "..."
	}
	return stderr
}
