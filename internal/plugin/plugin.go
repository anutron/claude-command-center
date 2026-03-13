package plugin

import (
	"database/sql"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// Context is provided to plugins during Init.
type Context struct {
	DB     *sql.DB
	Config *config.Config
	Styles *ui.Styles
	Grad   *ui.GradientColors
	Bus    EventBus
	Logger Logger
	DBPath string
	LLM    llm.LLM
}

// Action type constants.
const (
	ActionNoop      = "noop"
	ActionConsumed  = "consumed" // key was handled, no further action needed
	ActionOpenURL   = "open_url"
	ActionFlash     = "flash"
	ActionLaunch    = "launch"
	ActionQuit      = "quit"
	ActionNavigate  = "navigate"
	ActionUnhandled = "unhandled"
)

// Action is returned by HandleKey/HandleMessage to tell the host what to do.
type Action struct {
	Type    string            // ActionNoop, ActionOpenURL, ActionFlash, ActionLaunch, ActionQuit, ActionNavigate
	Payload string            // URL, message, slug, etc.
	Args    map[string]string // route arguments
	TeaCmd  tea.Cmd           // built-in plugins only; nil for external
}

// NoopAction returns an action that does nothing.
func NoopAction() Action {
	return Action{Type: ActionNoop}
}

// ConsumedAction signals that the plugin handled the key but needs no host
// action. Unlike NoopAction, this prevents the host from applying its own
// default behaviour for the key (e.g. Tab switching tabs).
func ConsumedAction() Action {
	return Action{Type: ActionConsumed}
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

// SettingsProvider is an optional interface for plugins that want to render
// their own settings detail view instead of the default.
type SettingsProvider interface {
	SettingsView(width, height int) string
	HandleSettingsKey(msg tea.KeyMsg) Action
	// SettingsOpenCmd is called when the user navigates to this provider's
	// settings pane. It returns a Cmd for async initialization (e.g., a live
	// credential check). Return nil if no async work is needed.
	SettingsOpenCmd() tea.Cmd
	// HandleSettingsMsg routes a tea.Msg to the provider for async result
	// handling. Returns (handled, action) — handled=true means the message
	// was consumed by this provider.
	HandleSettingsMsg(msg tea.Msg) (bool, Action)
}

// NotifyMsg is sent when an external process notifies the TUI to reload.
// Defined in the plugin package so plugins can type-assert on it without
// importing tui (which would cause a circular dependency).
type NotifyMsg struct {
	Event string
}
