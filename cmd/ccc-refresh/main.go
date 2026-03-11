package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/lockfile"
	"github.com/anutron/claude-command-center/internal/refresh"
	calendarsrc "github.com/anutron/claude-command-center/internal/refresh/sources/calendar"
	githubsrc "github.com/anutron/claude-command-center/internal/refresh/sources/github"
	gmailsrc "github.com/anutron/claude-command-center/internal/refresh/sources/gmail"
	granolasrc "github.com/anutron/claude-command-center/internal/refresh/sources/granola"
	slacksrc "github.com/anutron/claude-command-center/internal/refresh/sources/slack"
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
	release, err := lockfile.AcquireLock(stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer release()

	// Construct LLM implementation
	var l llm.LLM
	if !*noLLM && llm.Available() {
		l = llm.ClaudeCLI{}
	} else {
		l = llm.NoopLLM{}
	}

	// Build calendar IDs from config (only enabled calendars)
	var calendarIDs []string
	for _, cal := range cfg.Calendar.Calendars {
		if cal.IsEnabled() {
			calendarIDs = append(calendarIDs, cal.ID)
		}
	}

	// Build DataSources from config
	sources := []refresh.DataSource{
		calendarsrc.New(cfg.Calendar.Enabled, calendarIDs, nil),
		gmailsrc.New(cfg.Gmail.Enabled),
		githubsrc.New(cfg.GitHub.Enabled, cfg.GitHub.Repos, cfg.GitHub.Username),
		slacksrc.New(l),
		granolasrc.New(cfg.Granola.Enabled, l),
	}

	opts := refresh.Options{
		Verbose: *verbose,
		DryRun:  *dryRun,
		DB:      database,
		Sources: sources,
		LLM:     l,
	}

	if err := refresh.Run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
