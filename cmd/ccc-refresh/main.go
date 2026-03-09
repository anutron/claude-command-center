package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/refresh"
)

func main() {
	verbose := flag.Bool("v", false, "verbose output")
	dryRun := flag.Bool("dry-run", false, "print result to stdout instead of saving")
	noLLM := flag.Bool("no-llm", false, "skip LLM-based extraction and suggestions")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	dbPath := config.DBPath()
	database, err := db.OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Acquire refresh lock
	stateDir := config.DataDir()
	release, err := refresh.AcquireLock(stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer release()

	// Build calendar IDs from config
	var calendarIDs []string
	for _, cal := range cfg.Calendar.Calendars {
		calendarIDs = append(calendarIDs, cal.ID)
	}

	opts := refresh.Options{
		Verbose:         *verbose,
		NoLLM:           *noLLM,
		DryRun:          *dryRun,
		DB:              database,
		CalendarEnabled: cfg.Calendar.Enabled,
		GitHubEnabled:   cfg.GitHub.Enabled,
		GranolaEnabled:  cfg.Granola.Enabled,
		GitHubRepos:     cfg.GitHub.Repos,
		GitHubUsername:  cfg.GitHub.Username,
		CalendarIDs:     calendarIDs,
	}

	if err := refresh.Run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
