package tui

import (
	"database/sql"
	"strings"

	"github.com/anutron/claude-command-center/internal/builtin/commandcenter"
	"github.com/anutron/claude-command-center/internal/builtin/sessions"
	"github.com/anutron/claude-command-center/internal/builtin/settings"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Verify built-in plugins implement Starter at compile time.
var _ plugin.Starter = (*sessions.Plugin)(nil)
var _ plugin.Starter = (*commandcenter.Plugin)(nil)

type tab int

const (
	tabNew tab = iota
	tabResume
	tabCommand
	tabThreads
)

type tabEntry struct {
	label     string
	plugin    plugin.Plugin
	route     string
	ownerSlug string // plugin slug that owns this tab, for filtering
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

	// allTabs is the full unfiltered tab list; tabs is the current visible subset.
	allTabs []tabEntry
	// allPlugins holds every unique plugin for lifecycle management.
	allPlugins []plugin.Plugin

	// returnedFromLaunch is set when the TUI restarts after a Claude session.
	returnedFromLaunch bool

	// Onboarding flow state.
	onboarding      bool
	onboardingState *onboardingState

	db *sql.DB
}

// NewModel creates the main TUI model with plugins.
// bus and logger are owned by main.go and shared across all plugins.
// Optional extPlugins are appended as additional tabs.
func NewModel(database *sql.DB, cfg *config.Config, bus plugin.EventBus, logger plugin.Logger, l llm.LLM, extPlugins ...plugin.Plugin) Model {
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
		LLM:    l,
	}

	_ = sessPlug.Init(ctx)
	_ = ccPlug.Init(ctx)
	_ = settingsPlug.Init(ctx)

	// Build the full tab list (allTabs); rebuildTabs filters to visible.
	var allTabs []tabEntry
	allTabs = append(allTabs,
		tabEntry{label: "New Session", plugin: sessPlug, route: "new", ownerSlug: "sessions"},
		tabEntry{label: "Resume", plugin: sessPlug, route: "resume", ownerSlug: "sessions"},
		tabEntry{label: "Command Center", plugin: ccPlug, route: "commandcenter", ownerSlug: "commandcenter"},
		tabEntry{label: "Threads", plugin: ccPlug, route: "commandcenter/threads", ownerSlug: "commandcenter"},
	)
	for _, ep := range extPlugins {
		routes := ep.Routes()
		if len(routes) > 0 {
			for _, r := range routes {
				allTabs = append(allTabs, tabEntry{label: r.Description, plugin: ep, route: r.Slug, ownerSlug: ep.Slug()})
			}
		} else {
			allTabs = append(allTabs, tabEntry{label: ep.TabName(), plugin: ep, route: ep.Slug(), ownerSlug: ep.Slug()})
		}
	}
	allTabs = append(allTabs, tabEntry{label: "Settings", plugin: settingsPlug, route: "settings", ownerSlug: "settings"})

	// Collect all unique plugins for shutdown.
	allPlugins := []plugin.Plugin{sessPlug, ccPlug, settingsPlug}
	allPlugins = append(allPlugins, extPlugins...)

	m := Model{
		cfg:        cfg,
		styles:     styles,
		grad:       grad,
		allTabs:    allTabs,
		activeTab:  0,
		allPlugins: allPlugins,
		db:         database,
	}
	m.rebuildTabs()
	return m
}

// rebuildTabs filters allTabs to only enabled plugins, preserving the active tab if possible.
func (m *Model) rebuildTabs() {
	currentRoute := ""
	if int(m.activeTab) < len(m.tabs) {
		currentRoute = m.tabs[m.activeTab].route
	}

	var filtered []tabEntry
	for _, t := range m.allTabs {
		if !m.cfg.PluginEnabled(t.ownerSlug) {
			continue
		}
		if t.route == "commandcenter/threads" && !m.cfg.Threads.Enabled {
			continue
		}
		filtered = append(filtered, t)
	}
	m.tabs = filtered

	// Try to stay on the same route
	if currentRoute != "" {
		for i, t := range m.tabs {
			if t.route == currentRoute {
				m.activeTab = tab(i)
				return
			}
		}
	}
	// Fallback: clamp to valid range
	if int(m.activeTab) >= len(m.tabs) {
		m.activeTab = tab(len(m.tabs) - 1)
		if m.activeTab < 0 {
			m.activeTab = 0
		}
	}
}

// findTabByRoute returns the index of a tab with the given route, or -1 if not found.
func (m *Model) findTabByRoute(route string) int {
	for i, t := range m.tabs {
		if t.route == route {
			return i
		}
	}
	return -1
}

// SetReturnedFromLaunch marks that this TUI instance is returning from a Claude session.
// Must be called before the program is run.
func (m *Model) SetReturnedFromLaunch() {
	m.returnedFromLaunch = true
}

// SetOnboarding enables the onboarding flow. Must be called before the program is run.
func (m *Model) SetOnboarding() {
	m.onboarding = true
	m.onboardingState = newOnboardingState(m.cfg)
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
	// During onboarding, only run the tick — defer plugin init until onboarding completes.
	if m.onboarding {
		return ui.TickCmd()
	}

	var cmds []tea.Cmd
	cmds = append(cmds, ui.TickCmd())

	// Collect StartCmds from all plugins that implement Starter.
	seen := map[string]bool{}
	for _, p := range m.allPlugins {
		if seen[p.Slug()] {
			continue
		}
		seen[p.Slug()] = true
		if starter, ok := p.(plugin.Starter); ok {
			if cmd := starter.StartCmds(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	// Initial data load for plugins that need it.
	if m.db != nil {
		for _, p := range m.allPlugins {
			if p.RefreshInterval() == 0 && p.Slug() == "sessions" {
				cmds = append(cmds, p.Refresh())
			}
		}
	}

	if m.returnedFromLaunch {
		cmds = append(cmds, func() tea.Msg { return plugin.ReturnMsg{} })
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.onboarding {
		return m.updateOnboarding(msg)
	}

	switch msg := msg.(type) {
	case ui.TickMsg:
		m.frame++
		var cmds []tea.Cmd
		cmds = append(cmds, ui.TickCmd())
		m.broadcastMessage(msg, &cmds)
		return m, tea.Batch(cmds...)

	case tea.FocusMsg:
		// Terminal regained focus — force a full screen repaint to clear ghost artifacts.
		var cmds []tea.Cmd
		cmds = append(cmds, tea.ClearScreen)
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
			prev := m.activeTab
			m.activeTab = (m.activeTab + 1) % tab(len(m.tabs))
			cmd := m.activateTab(prev)
			return m, cmd
		case tea.KeyShiftTab:
			prev := m.activeTab
			m.activeTab = (m.activeTab + tab(len(m.tabs)) - 1) % tab(len(m.tabs))
			cmd := m.activateTab(prev)
			return m, cmd
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

	case plugin.NotifyMsg:
		// External notification — reload all plugins from DB
		var cmds []tea.Cmd
		m.broadcastMessage(msg, &cmds)
		return m, tea.Batch(cmds...)

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

func (m *Model) activateTab(prevTab tab) tea.Cmd {
	var cmds []tea.Cmd

	// Send TabLeaveMsg to the previous plugin.
	prevEntry := m.tabs[prevTab]
	_, leaveAction := prevEntry.plugin.HandleMessage(plugin.TabLeaveMsg{Route: prevEntry.route})
	if leaveAction.TeaCmd != nil {
		cmds = append(cmds, leaveAction.TeaCmd)
	}

	// Navigate the new plugin to its route.
	newEntry := m.tabs[m.activeTab]
	newEntry.plugin.NavigateTo(newEntry.route, nil)

	// Send TabViewMsg to the new plugin.
	_, viewAction := newEntry.plugin.HandleMessage(plugin.TabViewMsg{Route: newEntry.route})
	if viewAction.TeaCmd != nil {
		cmds = append(cmds, viewAction.TeaCmd)
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

func (m Model) processAction(action plugin.Action) (tea.Model, tea.Cmd) {
	// Rebuild tabs in case a plugin toggle changed visibility.
	m.rebuildTabs()

	switch action.Type {
	case plugin.ActionLaunch:
		la := &LaunchAction{Dir: action.Args["dir"]}
		if rid := action.Args["resume_id"]; rid != "" {
			la.Args = []string{"-r", rid}
		}
		if prompt := action.Args["initial_prompt"]; prompt != "" {
			la.InitialPrompt = prompt
		}
		if action.Args["worktree"] == "true" {
			la.Worktree = true
		}
		m.Launch = la
		// Broadcast LaunchMsg to all plugins before quitting.
		var cmds []tea.Cmd
		m.broadcastMessage(plugin.LaunchMsg{
			Dir:      action.Args["dir"],
			ResumeID: action.Args["resume_id"],
		}, &cmds)
		cmds = append(cmds, tea.Quit)
		return m, tea.Batch(cmds...)

	case plugin.ActionQuit:
		return m, tea.Quit

	case plugin.ActionNavigate:
		var cmd tea.Cmd
		switch action.Payload {
		case "sessions":
			if idx := m.findTabByRoute("new"); idx >= 0 {
				prev := m.activeTab
				m.activeTab = tab(idx)
				cmd = m.activateTab(prev)
			}
		case "command":
			if idx := m.findTabByRoute("commandcenter"); idx >= 0 {
				prev := m.activeTab
				m.activeTab = tab(idx)
				cmd = m.activateTab(prev)
			}
		}
		return m, cmd

	case plugin.ActionUnhandled:
		return m, tea.Quit

	default: // ActionNoop and anything else
		if action.TeaCmd != nil {
			return m, action.TeaCmd
		}
		return m, nil
	}
}

func (m Model) View() string {
	topPad := "\n\n\n\n\n\n"

	if m.onboarding {
		// During onboarding, always show banner for preview (unless user toggled it off).
		var banner string
		if m.cfg.BannerVisible() {
			banner = topPad + renderGradientBanner(&m.grad, m.cfg.Name, m.cfg.Subtitle, ui.ContentMaxWidth, m.frame)
		} else {
			banner = topPad
		}
		content := m.onboardingState.view(m.width, m.height, &m.styles, &m.grad, m.cfg, m.frame)
		page := lipgloss.JoinVertical(lipgloss.Left, banner, "", content)
		if m.width > 0 && m.height > 0 {
			return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, page)
		}
		return page
	}

	var sections []string
	if m.cfg.BannerVisible() {
		sections = append(sections, topPad+renderGradientBanner(&m.grad, m.cfg.Name, m.cfg.Subtitle, ui.ContentMaxWidth, m.frame))
	}

	tabBar := m.renderTabBar()

	// Compute overhead height so plugins know the available content area.
	// Build the header sections, count their lines, and pass the remainder.
	headerParts := make([]string, len(sections))
	copy(headerParts, sections)
	headerParts = append(headerParts, "", tabBar, "")
	header := lipgloss.JoinVertical(lipgloss.Left, headerParts...)
	contentHeight := m.height - strings.Count(header, "\n") - 1
	if contentHeight < 10 {
		contentHeight = 10
	}

	content := m.activePlugin().View(m.width, contentHeight, m.frame)

	sections = append(sections, "", tabBar, "", content)
	page := lipgloss.JoinVertical(lipgloss.Left, sections...)

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
	return lipgloss.PlaceHorizontal(ui.ContentMaxWidth, lipgloss.Center, tabBar)
}
