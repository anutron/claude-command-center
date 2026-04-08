package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/anutron/claude-command-center/internal/automation"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/lockfile"
	"github.com/anutron/claude-command-center/internal/plugin"
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
		fmt.Fprintf(os.Stderr, "Error: could not load config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Fix the config file at %s, or remove it to start fresh.\n", config.ConfigPath())
		os.Exit(1)
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

	// Connect to daemon for LLM activity reporting (best-effort, non-fatal).
	sockPath := filepath.Join(config.ConfigDir(), "daemon.sock")
	daemonClient, _ := daemon.NewClient(sockPath)
	if daemonClient != nil {
		defer daemonClient.Close()
	}

	// Construct LLM implementations.
	// Haiku for extraction (cheap, wide net), sonnet for routing (validates + writes prompts).
	var l llm.LLM
	var routingLLM llm.LLM
	if !*noLLM && llm.Available() {
		l = llm.ClaudeCLI{Model: "haiku"}
		routingLLM = llm.ClaudeCLI{Model: "sonnet"}
	} else {
		l = llm.NoopLLM{}
	}

	// Wrap LLMs with observability — reports activity to daemon for console visibility.
	if daemonClient != nil {
		report := func(topic string, payload llm.EventPayload) {
			id, _ := payload["id"].(string)
			op, _ := payload["operation"].(string)
			src, _ := payload["source"].(string)
			if topic == "llm.started" {
				go daemonClient.ReportLLMActivity(daemon.LLMActivityEvent{
					ID: id, Operation: op, Source: src,
					StartedAt: time.Now(), Status: "running",
				})
			} else {
				now := time.Now()
				durationMs, _ := payload["duration_ms"].(int64)
				errMsg, _ := payload["error"].(string)
				status, _ := payload["status"].(string)
				go daemonClient.ReportLLMActivity(daemon.LLMActivityEvent{
					ID: id, Operation: op, Source: src,
					FinishedAt: &now, DurationMs: int(durationMs),
					Error: errMsg, Status: status,
				})
			}
		}
		l = llm.NewObservableLLM(l, report, "ai-cron")
		if routingLLM != nil {
			routingLLM = llm.NewObservableLLM(routingLLM, report, "ai-cron-routing")
		}
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
		gmailsrc.New(cfg.Gmail, l),
		githubsrc.New(cfg.GitHub.Enabled, cfg.GitHub.Repos, cfg.GitHub.Username, cfg.GitHub.IsTrackMyPRs()),
		slacksrc.New(cfg.Slack.Enabled, cfg.Slack.EffectiveToken(), l, database),
		granolasrc.New(cfg.Granola.Enabled, l, database),
	}

	// Build context registry for source context fetching
	contextRegistry := refresh.NewContextRegistry()
	contextRegistry.Register("granola", granolasrc.NewContextFetcher())
	contextRegistry.Register("github", githubsrc.NewContextFetcher())
	if token := cfg.Slack.EffectiveToken(); token != "" {
		contextRegistry.Register("slack", slacksrc.NewContextFetcher(token))
	}
	contextRegistry.Register("gmail", gmailsrc.NewContextFetcher(cfg.Gmail))

	opts := refresh.Options{
		Verbose:         *verbose,
		DryRun:          *dryRun,
		DB:              database,
		Sources:         sources,
		LLM:             l,
		RoutingLLM:      routingLLM,
		ContextRegistry: contextRegistry,
	}

	if err := refresh.Run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Run automations after successful refresh.
	if len(cfg.Automations) > 0 {
		logPath := filepath.Join(config.DataDir(), "automation.log")
		logger, err := plugin.NewFileLogger(logPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create automation logger: %v\n", err)
		} else {
			defer logger.Close()

			autoRunner := automation.Runner{
				Automations: cfg.Automations,
				Config:      cfg,
				DBPath:      dbPath,
				Logger:      logger,
				Verbose:     *verbose,
				LogPath:     logPath,
			}
			results := autoRunner.RunAll(context.Background(), "refresh")
			for _, r := range results {
				if r.Status == "error" || *verbose {
					log.Printf("automation %s: %s (%s)", r.Name, r.Message, r.Elapsed)
				}
			}
		}
	}
}
