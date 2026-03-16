package tui

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/worktree"
)

// LaunchAction describes what to execute when the user picks an item.
type LaunchAction struct {
	Dir             string   // directory to chdir into
	Args            []string // args to claude (empty = new session, ["-r", id] = resume)
	InitialPrompt   string   // task context written to file for session reference
	Worktree        bool     // if true, create a git worktree for isolation
	ReturnToTodoID  string   // todo ID to return to after session exits
	WasResumeJoin   bool     // true if this was a join/resume of an existing session
}

// resolveSessionDir finds the correct project directory for a Claude session ID.
// Claude stores sessions under ~/.claude/projects/<project-path>/<session-id>.jsonl,
// where <project-path> encodes the working directory by replacing "/" with "-".
// When resuming, we need to chdir to the directory that maps to the correct project
// path, otherwise --resume won't find the session.
// Returns the original dir if the session can't be found (fall through to Claude's error).
func resolveSessionDir(sessionID, fallbackDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return fallbackDir
	}
	projectsDir := filepath.Join(home, ".claude", "projects")

	// First, check if the session exists in the fallback dir's project.
	if fallbackDir != "" {
		encoded := strings.ReplaceAll(filepath.Clean(fallbackDir), "/", "-")
		candidate := filepath.Join(projectsDir, encoded, sessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return fallbackDir // Session is where we expect it.
		}
	}

	// Session not in expected project — scan all project directories.
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return fallbackDir
	}

	sessionFile := sessionID + ".jsonl"
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, e.Name(), sessionFile)
		if _, err := os.Stat(candidate); err == nil {
			// Found the session file. Reverse the project-path encoding:
			// "-Users-aaron-Personal-proj" → "/Users/aaron/Personal/proj"
			projectPath := strings.ReplaceAll(e.Name(), "-", "/")
			// The encoding replaces "/" with "-", so the first char becomes "/"
			// because the leading "-" maps to a leading "/".
			if _, err := os.Stat(projectPath); err == nil {
				return projectPath
			}
		}
	}
	return fallbackDir
}

// RunClaude runs claude as a child process and returns when it exits.
// It returns the resolved launch directory (which may be a worktree path).
func RunClaude(action LaunchAction) (resolvedDir string, err error) {
	dir := action.Dir

	// When resuming a session, verify the session exists in the expected project
	// directory. If not, search across all Claude project directories to find
	// the correct one. This handles cases where the session was created in a
	// different directory (e.g., via --worktree or project dir mismatch).
	if action.WasResumeJoin && len(action.Args) >= 2 && action.Args[0] == "--resume" {
		sessionID := action.Args[1]
		dir = resolveSessionDir(sessionID, dir)
	}

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

	args := append([]string{}, action.Args...)
	if action.InitialPrompt != "" {
		// Write context to file for reference, and inject into Claude via --append-system-prompt
		// so the session has task context available.
		stateDir := os.Getenv("CCC_STATE_DIR")
		if stateDir == "" {
			home, _ := os.UserHomeDir()
			stateDir = filepath.Join(home, ".config", "ccc", "data")
		}
		_ = os.MkdirAll(stateDir, 0o755)
		contextPath := filepath.Join(stateDir, "task-context.md")
		_ = os.WriteFile(contextPath, []byte(action.InitialPrompt), 0o644)
		fmt.Fprintf(os.Stderr, "\nTask context written to %s\n\n", contextPath)
		args = append(args, "--append-system-prompt", action.InitialPrompt)
	}

	cmd := exec.Command("claude", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return dir, fmt.Errorf("claude exited with error: %w", err)
	}
	return dir, nil
}

// validateLaunchDir checks that dir is one of the Sessions learned paths or a
// subdirectory of one. Returns nil if allowed, an error otherwise. An empty
// dir (meaning "use cwd") is always allowed.
func validateLaunchDir(database *sql.DB, dir string) error {
	if dir == "" {
		return nil
	}

	// Normalize the requested dir.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("invalid launch dir: %w", err)
	}
	absDir = filepath.Clean(absDir)

	// Resolve symlinks so traversal tricks can't bypass the check.
	resolved, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		// If the path doesn't exist yet, fall back to the cleaned abs path.
		resolved = absDir
	}

	paths, err := db.DBLoadPaths(database)
	if err != nil || len(paths) == 0 {
		return fmt.Errorf("launch dir %q rejected: no learned paths available", dir)
	}

	for _, allowed := range paths {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		allowedAbs = filepath.Clean(allowedAbs)
		allowedResolved, err := filepath.EvalSymlinks(allowedAbs)
		if err != nil {
			allowedResolved = allowedAbs
		}

		// Exact match or subdirectory check.
		if resolved == allowedResolved {
			return nil
		}
		// Ensure trailing separator for prefix check to avoid
		// "/home/user/project2" matching "/home/user/project".
		prefix := allowedResolved + string(filepath.Separator)
		if strings.HasPrefix(resolved, prefix) {
			return nil
		}
	}

	return fmt.Errorf("launch dir %q is not within any learned path", dir)
}
