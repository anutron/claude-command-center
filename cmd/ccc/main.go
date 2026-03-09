package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/external"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/tui"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			if err := config.RunSetup(); err != nil {
				fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
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

	// Open database
	dbPath := config.DBPath()
	database, err := db.OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open database: %v\n", err)
	}
	if database != nil {
		defer database.Close()
	}

	// Load external plugins (persist across TUI loop iterations).
	bus := plugin.NewBus()
	logPath := filepath.Join(config.DataDir(), "ccc.log")
	logger, err := plugin.NewFileLogger(logPath)
	if err != nil {
		logger = plugin.NewMemoryLogger()
	}
	defer logger.Close()

	extCtx := plugin.Context{
		DB:     database,
		Config: cfg,
		Bus:    bus,
		Logger: logger,
		DBPath: config.DBPath(),
	}
	extPlugins, err := external.LoadExternalPlugins(cfg, extCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load external plugins: %v\n", err)
	}
	var pluginInterfaces []plugin.Plugin
	for _, ep := range extPlugins {
		pluginInterfaces = append(pluginInterfaces, ep)
	}
	defer func() {
		for _, ep := range pluginInterfaces {
			ep.Shutdown()
		}
	}()

	// TUI loop: launch TUI, optionally exec claude, return to TUI
	for {
		m := tui.NewModel(database, cfg, bus, logger, pluginInterfaces...)
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
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
			fmt.Fprintf(os.Stderr, "Failed to launch claude: %v\n", err)
			os.Exit(1)
		}
		// Claude exited — loop back to TUI
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Claude Command Center — Session Launcher & Dashboard")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Usage: ccc [command]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  (default)    Launch session picker")
	fmt.Fprintln(os.Stderr, "  setup        Run interactive setup wizard")
	fmt.Fprintln(os.Stderr, "  sessions     Same as default")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  -h, --help   Show this help")
}
