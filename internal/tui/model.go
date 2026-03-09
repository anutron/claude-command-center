package tui

import (
	"database/sql"
	"strings"

	"github.com/anutron/claude-command-center/internal/builtin/commandcenter"
	"github.com/anutron/claude-command-center/internal/builtin/sessions"
	"github.com/anutron/claude-command-center/internal/builtin/settings"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tab int

const (
	tabNew tab = iota
	tabResume
	tabCommand
	tabThreads
)

type tabEntry struct {
	label  string
	plugin plugin.Plugin
	route  string
}

// Model is the main Bubbletea model — a thin host that dispatches to plugins.
type Model struct {
	cfg    *config.Config
	styles Styles
	grad   GradientColors

	tabs      []tabEntry
	activeTab tab
	width     int
	height    int
	frame     int

	Launch *LaunchAction

	// Direct references for cross-plugin communication
	sessionsPlugin      *sessions.Plugin
	commandCenterPlugin *commandcenter.Plugin

	// allPlugins holds every unique plugin for lifecycle management.
	allPlugins []plugin.Plugin

	db *sql.DB
}

// NewModel creates the main TUI model with plugins.
// bus and logger are owned by main.go and shared across all plugins.
// Optional extPlugins are appended as additional tabs.
func NewModel(database *sql.DB, cfg *config.Config, bus plugin.EventBus, logger plugin.Logger, extPlugins ...plugin.Plugin) Model {
	pal := config.GetPalette(cfg.Palette, cfg.Colors)
	styles := NewStyles(pal)
	grad := NewGradientColors(pal)

	sessPlug := &sessions.Plugin{}
	ccPlug := commandcenter.New()

	// Build registry with all plugins.
	registry := plugin.NewRegistry()
	registry.Register(sessPlug)
	registry.Register(ccPlug)
	for _, ep := range extPlugins {
		registry.Register(ep)
	}

	settingsPlug := settings.New(registry)
	registry.Register(settingsPlug)

	ctx := plugin.Context{
		DB:     database,
		Config: cfg,
		Styles: &styles,
		Grad:   &grad,
		Bus:    bus,
		Logger: logger,
		DBPath: config.DBPath(),
	}

	_ = sessPlug.Init(ctx)
	_ = ccPlug.Init(ctx)
	_ = settingsPlug.Init(ctx)

	tabs := []tabEntry{
		{label: "New Session", plugin: sessPlug, route: "new"},
		{label: "Resume", plugin: sessPlug, route: "resume"},
		{label: "Command Center", plugin: ccPlug, route: "commandcenter"},
		{label: "Threads", plugin: ccPlug, route: "commandcenter/threads"},
	}

	// Append tabs for external plugins.
	for _, ep := range extPlugins {
		routes := ep.Routes()
		if len(routes) > 0 {
			for _, r := range routes {
				tabs = append(tabs, tabEntry{label: r.Description, plugin: ep, route: r.Slug})
			}
		} else {
			tabs = append(tabs, tabEntry{label: ep.TabName(), plugin: ep, route: ep.Slug()})
		}
	}

	// Settings tab at the end.
	tabs = append(tabs, tabEntry{label: "Settings", plugin: settingsPlug, route: "settings"})

	// Collect all unique plugins for shutdown.
	allPlugins := []plugin.Plugin{sessPlug, ccPlug, settingsPlug}
	allPlugins = append(allPlugins, extPlugins...)

	return Model{
		cfg:                 cfg,
		styles:              styles,
		grad:                grad,
		tabs:                tabs,
		activeTab:           tabNew,
		sessionsPlugin:      sessPlug,
		commandCenterPlugin: ccPlug,
		allPlugins:          allPlugins,
		db:                  database,
	}
}

func (m Model) activePlugin() plugin.Plugin {
	return m.tabs[m.activeTab].plugin
}

// Shutdown calls Shutdown on every unique plugin.
func (m Model) Shutdown() {
	seen := map[string]bool{}
	for _, p := range m.allPlugins {
		if !seen[p.Slug()] {
			seen[p.Slug()] = true
			p.Shutdown()
		}
	}
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, tickCmd())
	cmds = append(cmds, m.sessionsPlugin.StartCmds())
	cmds = append(cmds, m.commandCenterPlugin.StartCmds())
	if m.db != nil {
		cmds = append(cmds, m.sessionsPlugin.Refresh())
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.frame++
		var cmds []tea.Cmd
		cmds = append(cmds, tickCmd())
		m.broadcastMessage(msg, &cmds)
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		var cmds []tea.Cmd
		m.broadcastMessage(msg, &cmds)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyTab:
			m.activeTab = (m.activeTab + 1) % tab(len(m.tabs))
			m.activateTab()
			return m, nil
		case tea.KeyShiftTab:
			m.activeTab = (m.activeTab + tab(len(m.tabs)) - 1) % tab(len(m.tabs))
			m.activateTab()
			return m, nil
		case tea.KeyEsc:
			// Let active plugin try esc first
			action := m.activePlugin().HandleKey(msg)
			if action.Type != "unhandled" && action.Type != "quit" {
				return m.processAction(action)
			}
			return m, tea.Quit
		}
		action := m.activePlugin().HandleKey(msg)
		return m.processAction(action)

	default:
		var cmds []tea.Cmd
		m.broadcastMessage(msg, &cmds)
		return m, tea.Batch(cmds...)
	}
}

// broadcastMessage sends a message to all unique plugins and collects cmds.
func (m *Model) broadcastMessage(msg tea.Msg, cmds *[]tea.Cmd) {
	seen := map[string]bool{}
	for _, t := range m.tabs {
		slug := t.plugin.Slug()
		if seen[slug] {
			continue
		}
		seen[slug] = true
		_, action := t.plugin.HandleMessage(msg)
		if action.TeaCmd != nil {
			*cmds = append(*cmds, action.TeaCmd)
		}
	}
}

func (m *Model) activateTab() {
	entry := m.tabs[m.activeTab]
	entry.plugin.NavigateTo(entry.route, nil)
}

func (m Model) processAction(action plugin.Action) (tea.Model, tea.Cmd) {
	switch action.Type {
	case "launch":
		la := &LaunchAction{Dir: action.Args["dir"]}
		if rid := action.Args["resume_id"]; rid != "" {
			la.Args = []string{"-r", rid}
		}
		if prompt := action.Args["initial_prompt"]; prompt != "" {
			la.InitialPrompt = prompt
		}
		m.Launch = la
		return m, tea.Quit

	case "quit":
		return m, tea.Quit

	case "navigate":
		switch action.Payload {
		case "sessions":
			if todo := m.commandCenterPlugin.PendingLaunchTodo(); todo != nil {
				m.sessionsPlugin.SetPendingLaunchTodo(todo)
			}
			m.activeTab = tabNew
			m.activateTab()
		case "command":
			m.activeTab = tabCommand
			m.activateTab()
		}
		return m, nil

	case "unhandled":
		return m, tea.Quit

	default: // "noop" and anything else
		if action.TeaCmd != nil {
			return m, action.TeaCmd
		}
		return m, nil
	}
}

func (m Model) View() string {
	topPad := "\n\n\n\n\n\n"
	banner := topPad + renderGradientBanner(&m.grad, m.cfg.Name, contentMaxWidth, m.frame)
	tabBar := m.renderTabBar()
	content := m.activePlugin().View(m.width, m.height, m.frame)

	page := lipgloss.JoinVertical(lipgloss.Left, banner, "", tabBar, "", content)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, page)
	}
	return page
}

func (m Model) renderTabBar() string {
	sep := m.styles.InactiveTab.Render(" | ")
	var parts []string
	for i, t := range m.tabs {
		if tab(i) == m.activeTab {
			parts = append(parts, m.styles.ActiveTab.Render("> "+t.label))
		} else {
			parts = append(parts, m.styles.InactiveTab.Render(t.label))
		}
		if i < len(m.tabs)-1 {
			parts = append(parts, sep)
		}
	}
	tabBar := strings.Join(parts, "")
	return lipgloss.PlaceHorizontal(contentMaxWidth, lipgloss.Center, tabBar)
}
