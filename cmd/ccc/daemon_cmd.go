package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/automation"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/refresh"
	calendarsrc "github.com/anutron/claude-command-center/internal/refresh/sources/calendar"
	githubsrc "github.com/anutron/claude-command-center/internal/refresh/sources/github"
	gmailsrc "github.com/anutron/claude-command-center/internal/refresh/sources/gmail"
	granolasrc "github.com/anutron/claude-command-center/internal/refresh/sources/granola"
	slacksrc "github.com/anutron/claude-command-center/internal/refresh/sources/slack"
)

// socketPath returns the deterministic daemon socket path.
func socketPath() string {
	return filepath.Join(config.ConfigDir(), "daemon.sock")
}

// pidPath returns the daemon PID file path.
func pidPath() string {
	return filepath.Join(config.ConfigDir(), "daemon.pid")
}

// runDaemon dispatches daemon start|stop|status subcommands.
func runDaemon(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ccc daemon start|stop|status")
	}
	switch args[0] {
	case "start":
		return runDaemonStart()
	case "stop":
		return runDaemonStop()
	case "status":
		return runDaemonStatus()
	default:
		return fmt.Errorf("unknown daemon command: %s (expected start|stop|status)", args[0])
	}
}

// runDaemonStart spawns a detached daemon process.
func runDaemonStart() error {
	// Check if already running.
	if pid, alive := readPID(); alive {
		return fmt.Errorf("daemon already running (PID: %d)", pid)
	}

	// Re-exec ourselves with the internal flag to run the daemon loop.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	if err := os.MkdirAll(config.DataDir(), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	logPath := filepath.Join(config.DataDir(), "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}

	cmd := exec.Command(exe, "--daemon-internal")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session — survives parent exit.
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}
	logFile.Close()

	// Write PID file.
	pid := cmd.Process.Pid
	if err := os.MkdirAll(config.ConfigDir(), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(pidPath(), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}

	// Release the process so it's fully detached from us.
	_ = cmd.Process.Release()

	fmt.Printf("Daemon started (PID: %d)\n", pid)
	fmt.Printf("  Socket: %s\n", socketPath())
	fmt.Printf("  Log:    %s\n", logPath)
	return nil
}

// runDaemonStop sends SIGTERM to the running daemon.
func runDaemonStop() error {
	pid, alive := readPID()
	if !alive {
		// Clean up stale PID file if it exists.
		os.Remove(pidPath())
		fmt.Println("Daemon is not running")
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM to %d: %w", pid, err)
	}

	os.Remove(pidPath())
	fmt.Printf("Daemon stopped (PID: %d)\n", pid)
	return nil
}

// runDaemonStatus prints whether the daemon is running.
func runDaemonStatus() error {
	pid, alive := readPID()

	if !alive {
		fmt.Println("Status: stopped")
		if pid > 0 {
			fmt.Printf("  Stale PID file references PID %d (not running)\n", pid)
			os.Remove(pidPath())
		}
		return nil
	}

	fmt.Printf("Status: running (PID: %d)\n", pid)

	// Try to ping via socket.
	client, err := daemon.NewClient(socketPath())
	if err != nil {
		fmt.Printf("  Socket: unreachable (%v)\n", err)
		return nil
	}
	defer client.Close()

	if err := client.Ping(); err != nil {
		fmt.Printf("  Socket: connected but ping failed (%v)\n", err)
	} else {
		fmt.Printf("  Socket: %s (healthy)\n", socketPath())
	}
	return nil
}

// readPID reads the PID file and checks if the process is alive.
// Returns (pid, alive). If no PID file exists, returns (0, false).
func readPID() (int, bool) {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		return 0, false
	}
	// Check if process is alive by sending signal 0.
	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return pid, false
	}
	return pid, true
}

// runDaemonInternal is the actual daemon loop, called via --daemon-internal.
// This runs in a detached process and blocks until shutdown.
func runDaemonInternal() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	// Parse refresh interval from daemon config.
	interval := 5 * time.Minute
	if cfg.Daemon.RefreshInterval != "" {
		if d, err := time.ParseDuration(cfg.Daemon.RefreshInterval); err == nil && d >= 1*time.Minute {
			interval = d
		}
	}

	refreshFunc := func() error {
		// Reload config each refresh to pick up changes.
		freshCfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("reload config: %w", err)
		}
		return runRefresh(freshCfg, database)
	}

	// Create agent runner wrapped with budget/rate-limit governance.
	maxConcurrent := cfg.Agent.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	innerRunner := agent.NewRunner(maxConcurrent)
	governedRunner := agent.NewGovernedRunner(innerRunner, database, &cfg.Agent)

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:      socketPath(),
		DB:              database,
		RefreshFunc:     refreshFunc,
		RefreshInterval: interval,
		AgentRunner:     governedRunner,
		GovernedRunner:  governedRunner,
	})

	// Handle shutdown signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		srv.Shutdown()
		os.Remove(pidPath())
	}()

	fmt.Printf("Daemon serving on %s (interval: %s)\n", socketPath(), interval)
	return srv.Serve()
}

// runRefresh builds data sources, runs a refresh cycle, and then executes
// any configured automations (same logic as ai-cron but running inside the daemon).
func runRefresh(cfg *config.Config, database *sql.DB) error {
	var l llm.LLM
	if llm.Available() {
		l = llm.ClaudeCLI{Model: "haiku"}
	} else {
		l = llm.NoopLLM{}
	}

	var calendarIDs []string
	for _, cal := range cfg.Calendar.Calendars {
		if cal.IsEnabled() {
			calendarIDs = append(calendarIDs, cal.ID)
		}
	}

	sources := []refresh.DataSource{
		calendarsrc.New(cfg.Calendar.Enabled, calendarIDs, nil),
		gmailsrc.New(cfg.Gmail, l),
		githubsrc.New(cfg.GitHub.Enabled, cfg.GitHub.Repos, cfg.GitHub.Username, cfg.GitHub.IsTrackMyPRs()),
		slacksrc.New(cfg.Slack.Enabled, cfg.Slack.EffectiveToken(), l, database),
		granolasrc.New(cfg.Granola.Enabled, l, database),
	}

	if err := refresh.Run(refresh.Options{
		DB:      database,
		Sources: sources,
		LLM:     l,
	}); err != nil {
		return err
	}

	// Run automations after successful data refresh.
	if len(cfg.Automations) > 0 {
		logPath := filepath.Join(config.DataDir(), "automation.log")
		logger, err := plugin.NewFileLogger(logPath)
		if err != nil {
			log.Printf("Warning: could not create automation logger: %v", err)
			return nil // Don't fail the refresh for automation logger issues.
		}
		defer logger.Close()

		autoRunner := automation.Runner{
			Automations: cfg.Automations,
			Config:      cfg,
			DBPath:      config.DBPath(),
			Logger:      logger,
			LogPath:     logPath,
		}
		results := autoRunner.RunAll(context.Background(), "daemon-refresh")
		for _, r := range results {
			if r.Status == "error" {
				log.Printf("automation %s: %s (%s)", r.Name, r.Message, r.Elapsed)
			}
		}
	}

	return nil
}
