package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/tui"
)

func runAddBookmark(args []string) error {
	fs := flag.NewFlagSet("add-bookmark", flag.ContinueOnError)
	sessionID := fs.String("session-id", "", "Claude session UUID (required)")
	project := fs.String("project", "", "Project path (required)")
	repo := fs.String("repo", "", "Repo name (required)")
	branch := fs.String("branch", "", "Branch name (required)")
	summary := fs.String("summary", "", "Summary (required)")
	label := fs.String("label", "", "Label (optional)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *sessionID == "" || *project == "" || *repo == "" || *branch == "" || *summary == "" {
		return fmt.Errorf("--session-id, --project, --repo, --branch, and --summary are required")
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	sess := db.Session{
		SessionID: *sessionID,
		Project:   *project,
		Repo:      *repo,
		Branch:    *branch,
		Summary:   *summary,
		Created:   time.Now(),
	}

	if err := db.DBInsertBookmark(database, sess, *label); err != nil {
		return fmt.Errorf("insert bookmark: %w", err)
	}

	_ = tui.SendNotify("reload")

	fmt.Printf("Bookmark saved: %s\n", *summary)
	return nil
}
