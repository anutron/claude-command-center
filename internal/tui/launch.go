package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// LaunchAction describes what to execute when the user picks an item.
type LaunchAction struct {
	Dir           string   // directory to chdir into
	Args          []string // args to claude (empty = new session, ["-r", id] = resume)
	InitialPrompt string  // task context written to file for session reference
}

// RunClaude runs claude as a child process and returns when it exits.
func RunClaude(action LaunchAction) error {
	if err := os.Chdir(action.Dir); err != nil {
		return fmt.Errorf("chdir %s: %w", action.Dir, err)
	}

	if action.InitialPrompt != "" {
		stateDir := os.Getenv("CCC_STATE_DIR")
		if stateDir == "" {
			home, _ := os.UserHomeDir()
			stateDir = filepath.Join(home, ".config", "ccc", "data")
		}
		_ = os.MkdirAll(stateDir, 0o755)
		contextPath := filepath.Join(stateDir, "task-context.md")
		_ = os.WriteFile(contextPath, []byte(action.InitialPrompt), 0o644)
		fmt.Fprintf(os.Stderr, "\nTask context written to %s\n\n", contextPath)
	}

	cmd := exec.Command("claude", action.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude exited with error: %w", err)
	}
	return nil
}
