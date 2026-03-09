package plugin

import (
	"database/sql"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

// Context is provided to plugins during Init.
type Context struct {
	DB     *sql.DB
	Config *config.Config
	Styles interface{} // *tui.Styles — interface to avoid circular import
	Grad   interface{} // *tui.GradientColors
	Bus    EventBus
	Logger Logger
	DBPath string
	LLM    interface{} // llm.LLM — interface to avoid circular import
}

// Action is returned by HandleKey/HandleMessage to tell the host what to do.
type Action struct {
	Type    string            // "noop", "open_url", "flash", "launch", "quit", "navigate"
	Payload string            // URL, message, slug, etc.
	Args    map[string]string // route arguments
	TeaCmd  tea.Cmd           // built-in plugins only; nil for external
}

// NoopAction returns an action that does nothing.
func NoopAction() Action {
	return Action{Type: "noop"}
}

// Route declares a navigable sub-route within a plugin.
type Route struct {
	Slug        string
	Description string
	ArgKeys     []string
}

// Migration is a versioned SQL migration for plugin-specific tables.
type Migration struct {
	Version int
	SQL     string
}

// KeyBinding declares a keybinding for a plugin.
type KeyBinding struct {
	Key         string
	Description string
	Mode        string // "" = normal mode
	Promoted    bool   // true = shown in tab footer hints; false = only in ? help
}

// Plugin is the interface all plugins must implement.
type Plugin interface {
	// Identity
	Slug() string
	TabName() string

	// Lifecycle
	Init(ctx Context) error
	Shutdown()

	// Database
	Migrations() []Migration

	// Display
	View(width, height, frame int) string
	KeyBindings() []KeyBinding

	// Input
	HandleKey(msg tea.KeyMsg) Action
	HandleMessage(msg tea.Msg) (handled bool, action Action)

	// Routing
	Routes() []Route
	NavigateTo(route string, args map[string]string)

	// Scheduling
	RefreshInterval() time.Duration
	Refresh() tea.Cmd
}

// Starter is optionally implemented by plugins that need to run initial
// tea.Cmds (e.g., spinner ticks). The host collects these during Init().
type Starter interface {
	StartCmds() tea.Cmd
}

// SetupFlow is optionally implemented by plugins that need onboarding.
type SetupFlow interface {
	RunSetup() (map[string]interface{}, error)
}

// NotifyMsg is sent when an external process notifies the TUI to reload.
// Defined in the plugin package so plugins can type-assert on it without
// importing tui (which would cause a circular dependency).
type NotifyMsg struct {
	Event string
}
