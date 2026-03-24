package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
)

// runRegister handles: ccc register --session-id <id> --pid <pid> --project <dir> [--worktree-path <path>]
func runRegister(args []string) error {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	sessionID := fs.String("session-id", "", "Session ID (required)")
	pid := fs.Int("pid", 0, "Session PID (required)")
	project := fs.String("project", "", "Project directory (required)")
	worktreePath := fs.String("worktree-path", "", "Worktree path (optional)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sessionID == "" || *pid == 0 || *project == "" {
		return fmt.Errorf("--session-id, --pid, and --project are required")
	}

	params := daemon.RegisterSessionParams{
		SessionID:    *sessionID,
		PID:          *pid,
		Project:      *project,
		WorktreePath: *worktreePath,
	}

	// Try daemon first.
	client, err := daemon.NewClient(socketPath())
	if err == nil {
		defer client.Close()
		if err := client.RegisterSession(params); err != nil {
			return fmt.Errorf("register via daemon: %w", err)
		}
		fmt.Printf("Session %s registered via daemon\n", *sessionID)
		return nil
	}

	// Fallback: direct DB write.
	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	record := db.SessionRecord{
		SessionID:    *sessionID,
		PID:          *pid,
		Project:      *project,
		WorktreePath: *worktreePath,
		State:        "active",
		RegisteredAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := db.DBInsertSession(database, record); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	fmt.Printf("Session %s registered via direct DB write (daemon not running)\n", *sessionID)
	return nil
}

// runUpdateSession handles: ccc update-session --session-id <id> --topic <topic>
func runUpdateSession(args []string) error {
	fs := flag.NewFlagSet("update-session", flag.ContinueOnError)
	sessionID := fs.String("session-id", "", "Session ID (required)")
	topic := fs.String("topic", "", "Session topic")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sessionID == "" {
		return fmt.Errorf("--session-id is required")
	}

	params := daemon.UpdateSessionParams{
		SessionID: *sessionID,
		Topic:     *topic,
	}

	client, err := daemon.NewClient(socketPath())
	if err != nil {
		return fmt.Errorf("daemon not running: %w", err)
	}
	defer client.Close()

	if err := client.UpdateSession(params); err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	fmt.Printf("Session %s updated\n", *sessionID)
	return nil
}

// runRefreshCmd handles: ccc refresh
func runRefreshCmd() error {
	client, err := daemon.NewClient(socketPath())
	if err != nil {
		return fmt.Errorf("daemon not running: %w", err)
	}
	defer client.Close()

	if err := client.Refresh(); err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	fmt.Println("Refresh triggered")
	return nil
}

// runStopAll handles: ccc stop-all
func runStopAll() error {
	client, err := daemon.NewClient(socketPath())
	if err != nil {
		return fmt.Errorf("daemon not running (is it started?): %w", err)
	}
	defer client.Close()

	result, err := client.StopAllAgents()
	if err != nil {
		return fmt.Errorf("stop-all: %w", err)
	}
	fmt.Printf("Emergency stop: all agents stopped. (%d killed)\n", result.Stopped)
	return nil
}
