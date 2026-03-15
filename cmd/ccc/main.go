package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/doctor"
	"github.com/anutron/claude-command-center/internal/external"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/refresh/sources/calendar"
	"github.com/anutron/claude-command-center/internal/refresh/sources/github"
	"github.com/anutron/claude-command-center/internal/refresh/sources/gmail"
	"github.com/anutron/claude-command-center/internal/refresh/sources/granola"
	"github.com/anutron/claude-command-center/internal/tui"
)

func main() {
	forceSetup := false

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "doctor":
			fmt.Println("Claude Command Center — Doctor")
			fmt.Println()
			live := false
			for _, a := range os.Args[2:] {
				if a == "--live" {
					live = true
				}
			}
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: could not load config: %v\n", err)
				os.Exit(1)
			}
			pal := config.GetPalette(cfg.Palette, cfg.Colors)
			providers := []plugin.DoctorProvider{
				calendar.NewSettings(cfg, pal, nil),
				gmail.NewDoctor(cfg.Gmail),
				github.NewSettings(cfg, pal, nil),
				granola.NewSettings(cfg, pal),
			}
			if err := doctor.RunDoctor(providers, live); err != nil {
				os.Exit(1)
			}
			return
		case "install-schedule":
			if err := config.InstallSchedule(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "uninstall-schedule":
			if err := config.UninstallSchedule(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "notify":
			event := "reload"
			if len(os.Args) > 2 {
				event = os.Args[2]
			}
			if err := tui.SendNotify(event); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "add-todo":
			if err := runAddTodo(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "todo":
			if err := runTodo(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "add-bookmark":
			if err := runAddBookmark(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "paths":
			if err := runPaths(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "worktrees":
			if err := runWorktrees(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "-h", "--help", "help":
			printUsage()
			return
		case "setup":
			forceSetup = true
			// Fall through to normal TUI launch with onboarding.
		case "sessions":
			// same as default, fall through
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

	// Detect first run: config file doesn't exist yet.
	_, statErr := os.Stat(config.ConfigPath())
	isFirstRun := os.IsNotExist(statErr)

	// Load config — exit on error to prevent defaults from overwriting the user's file.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not load config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Fix the config file at %s, or remove it to start fresh.\n", config.ConfigPath())
		if bakPath := config.ConfigPath() + ".bak"; fileExists(bakPath) {
			fmt.Fprintf(os.Stderr, "A backup exists at %s\n", bakPath)
		}
		os.Exit(1)
	}

	// Open database (required — TUI is useless without it)
	dbPath := config.DBPath()
	database, err := db.OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not open database at %s: %v\n", dbPath, err)
		fmt.Fprintf(os.Stderr, "Run 'ccc setup' to initialize, or check permissions.\n")
		os.Exit(1)
	}
	defer database.Close()

	// Load external plugins (persist across TUI loop iterations).
	bus := plugin.NewBus()
	logPath := filepath.Join(config.DataDir(), "ccc.log")
	logger, err := plugin.NewFileLogger(logPath)
	if err != nil {
		logger = plugin.NewMemoryLogger()
	}
	defer logger.Close()

	// Construct LLM implementation
	var l llm.LLM
	if llm.Available() {
		l = llm.ClaudeCLI{}
	} else {
		l = llm.NoopLLM{}
	}

	extCtx := plugin.Context{
		DB:     database,
		Config: cfg,
		Bus:    bus,
		Logger: logger,
		DBPath: config.DBPath(),
		LLM:    l,
	}
	extPlugins, err := external.LoadExternalPlugins(cfg, extCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load external plugins: %v\n", err)
	}
	var pluginInterfaces []plugin.Plugin
	for _, ep := range extPlugins {
		pluginInterfaces = append(pluginInterfaces, ep)
	}
	// Graceful shutdown on SIGINT/SIGTERM to clean up plugin subprocesses
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		for _, ep := range pluginInterfaces {
			ep.Shutdown()
		}
		os.Exit(0)
	}()
	defer func() {
		signal.Stop(sigCh)
		for _, ep := range pluginInterfaces {
			ep.Shutdown()
		}
	}()

	// TUI loop: launch TUI, optionally exec claude, return to TUI
	returnedFromLaunch := false
	for {
		m := tui.NewModel(database, cfg, bus, logger, l, pluginInterfaces...)
		if returnedFromLaunch {
			m.SetReturnedFromLaunch()
		}
		if (isFirstRun || forceSetup) && !returnedFromLaunch {
			m.SetOnboarding()
		}
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())

		// Start unix socket listener for cross-instance notifications
		cleanupNotify := tui.StartNotifyListener(p)

		finalModel, err := p.Run()
		cleanupNotify()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fm := finalModel.(tui.Model)
		if fm.Launch == nil {
			// User pressed Esc — exit
			break
		}

		resolvedDir, err := tui.RunClaude(*fm.Launch)
		// Write the resolved launch directory (may be worktree) so the shell hook can cd to it after exit.
		_ = os.WriteFile(filepath.Join(config.DataDir(), "last-dir"), []byte(resolvedDir), 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Claude error: %v\n", err)
		}
		// Claude exited — loop back to TUI with returnedFromLaunch flag
		returnedFromLaunch = true
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Claude Command Center — Session Launcher & Dashboard")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Usage: ccc [command]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  (default)            Launch session picker")
	fmt.Fprintln(os.Stderr, "  setup                Run interactive setup wizard")
	fmt.Fprintln(os.Stderr, "  doctor [--live]       Check system health (--live hits network endpoints)")
	fmt.Fprintln(os.Stderr, "  install-schedule     Install launchd plist for background refresh")
	fmt.Fprintln(os.Stderr, "  uninstall-schedule   Remove background refresh schedule")
	fmt.Fprintln(os.Stderr, "  notify [event]       Notify running instances to reload (default: reload)")
	fmt.Fprintln(os.Stderr, "  add-todo             Add a todo to the Command Center")
	fmt.Fprintln(os.Stderr, "  todo --get <id>      Get a todo by display ID (JSON output)")
	fmt.Fprintln(os.Stderr, "  add-bookmark         Save a session bookmark")
	fmt.Fprintln(os.Stderr, "  paths                List learned project paths (--json, --auto-describe, --add-rule)")
	fmt.Fprintln(os.Stderr, "  worktrees            List CCC-managed git worktrees")
	fmt.Fprintln(os.Stderr, "  worktrees prune      Remove all CCC worktrees (or prune [path] for one repo)")
	fmt.Fprintln(os.Stderr, "  sessions             Same as default")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  -h, --help           Show this help")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
