package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/tui"
)

func runAddTodo(args []string) error {
	fs := flag.NewFlagSet("add-todo", flag.ContinueOnError)
	title := fs.String("title", "", "Todo title (required)")
	source := fs.String("source", "cli", "Source identifier")
	sourceRef := fs.String("source-ref", "", "Source reference")
	ctx := fs.String("context", "", "Context text")
	detail := fs.String("detail", "", "Detail text")
	whoWaiting := fs.String("who-waiting", "", "Who is waiting")
	projectDir := fs.String("project-dir", "", "Project directory")
	sessionID := fs.String("session-id", "", "Claude session ID")
	due := fs.String("due", "", "Due date (YYYY-MM-DD)")
	effort := fs.String("effort", "", "Effort estimate")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *title == "" {
		return fmt.Errorf("--title is required")
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	t := db.Todo{
		ID:         db.GenID(),
		Title:      *title,
		Status:     "active",
		Source:     *source,
		SourceRef:  *sourceRef,
		Context:    *ctx,
		Detail:     *detail,
		WhoWaiting: *whoWaiting,
		ProjectDir: *projectDir,
		SessionID:  *sessionID,
		Due:        *due,
		Effort:     *effort,
		CreatedAt:  time.Now(),
	}

	if err := db.DBInsertTodo(database, t); err != nil {
		return fmt.Errorf("insert todo: %w", err)
	}

	_ = tui.SendNotify("reload")

	fmt.Printf("Todo created: %s (id: %s)\n", t.Title, t.ID)
	return nil
}
