package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/anutron/claude-command-center/internal/worktree"
)

// LaunchAction describes what to execute when the user picks an item.
type LaunchAction struct {
	Dir           string   // directory to chdir into
	Args          []string // args to claude (empty = new session, ["-r", id] = resume)
	InitialPrompt string  // task context written to file for session reference
	Worktree      bool     // if true, create a git worktree for isolation
}

// RunClaude runs claude as a child process and returns when it exits.
// It returns the resolved launch directory (which may be a worktree path).
func RunClaude(action LaunchAction) (resolvedDir string, err error) {
	dir := action.Dir

	if action.Worktree {
		wtDir, branch, wtErr := worktree.PrepareWorktree(dir)
		if wtErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: worktree failed: %v, launching directly\n", wtErr)
		} else {
			dir = wtDir
			fmt.Fprintf(os.Stderr, "Worktree: %s (branch %s)\n", wtDir, branch)
		}
	}

	if err := os.Chdir(dir); err != nil {
		return dir, fmt.Errorf("chdir %s: %w", dir, err)
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
		return dir, fmt.Errorf("claude exited with error: %w", err)
	}
	return dir, nil
}
