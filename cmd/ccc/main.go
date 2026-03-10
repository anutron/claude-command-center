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
	"github.com/anutron/claude-command-center/internal/tui"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "doctor":
			fmt.Println("Claude Command Center — Doctor")
			fmt.Println()
			if err := doctor.RunDoctor(); err != nil {
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
		case "-h", "--help", "help":
			printUsage()
			return
		case "sessions":
			// same as default, fall through
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config: %v\n", err)
		cfg = config.DefaultConfig()
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
		m := tui.NewModel(database, cfg, bus, logger, pluginInterfaces...)
		if returnedFromLaunch {
			m.SetReturnedFromLaunch()
		}
		p := tea.NewProgram(m, tea.WithAltScreen())

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

		if err := tui.RunClaude(*fm.Launch); err != nil {
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
	fmt.Fprintln(os.Stderr, "  doctor               Check system health")
	fmt.Fprintln(os.Stderr, "  install-schedule     Install launchd plist for background refresh")
	fmt.Fprintln(os.Stderr, "  uninstall-schedule   Remove background refresh schedule")
	fmt.Fprintln(os.Stderr, "  notify [event]       Notify running instances to reload (default: reload)")
	fmt.Fprintln(os.Stderr, "  sessions             Same as default")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  -h, --help           Show this help")
}
