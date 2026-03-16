package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/tui"
)

func runUpdateTodo(args []string) error {
	fs := flag.NewFlagSet("update-todo", flag.ContinueOnError)
	id := fs.String("id", "", "Todo ID (required)")
	sessionSummary := fs.String("session-summary", "", "Session summary (use - to read from stdin)")
	sessionStatus := fs.String("session-status", "", "Session status")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *id == "" {
		return fmt.Errorf("--id is required")
	}

	// If session-summary is "-", read from stdin.
	if *sessionSummary == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		s := strings.TrimSpace(string(data))
		sessionSummary = &s
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if *sessionSummary != "" {
		if err := db.DBUpdateTodoSessionSummary(database, *id, *sessionSummary); err != nil {
			return fmt.Errorf("update session summary: %w", err)
		}
		fmt.Printf("Updated session_summary for todo %s\n", *id)
	}

	if *sessionStatus != "" {
		if err := db.DBUpdateTodoSessionStatus(database, *id, *sessionStatus); err != nil {
			return fmt.Errorf("update session status: %w", err)
		}
		fmt.Printf("Updated session_status for todo %s\n", *id)
	}

	_ = tui.SendNotify("reload")

	return nil
}
