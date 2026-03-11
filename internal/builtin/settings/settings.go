// Package settings implements the Settings plugin for CCC.
// It provides a sidebar-based UI for appearance, plugins, data sources,
// system actions, and logs.
package settings

import (
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/refresh/sources/calendar"
	ghsettings "github.com/anutron/claude-command-center/internal/refresh/sources/github"
	"github.com/anutron/claude-command-center/internal/refresh/sources/granola"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Plugin implements the plugin.Plugin interface for Settings.
type Plugin struct {
	cfg      *config.Config
	logger   plugin.Logger
	registry *plugin.Registry
	bus      plugin.EventBus
	styles   settingsStyles

	// SettingsProvider implementations for data sources
	providers map[string]plugin.SettingsProvider

	// Sidebar nav state
	navCategories []Category
	navCursor     int
	focusZone     FocusZone

	// Banner editing state (used by content_appearance)
	bannerNameInput     textinput.Model
	bannerSubtitleInput textinput.Model
	bannerField         int  // 0=name, 1=subtitle, 2=show/hide
	bannerEditing       bool // true when a text field is focused

	// Palette state (used by content_appearance)
	paletteCursor int

	// Logs state (used by content_logs)
	logOffset int

	// System content pane cursor positions (keyed by slug)
	systemCursors map[string]int

	// Flash message
	flashMessage   string
	flashMessageAt time.Time

	width, height int
}

// New creates a new Settings plugin. The registry is used to enumerate all plugins.
func New(registry *plugin.Registry) *Plugin {
	return &Plugin{
		registry: registry,
	}
}

func (p *Plugin) Slug() string    { return "settings" }
func (p *Plugin) TabName() string { return "Settings" }

func (p *Plugin) Migrations() []plugin.Migration { return nil }

func (p *Plugin) Routes() []plugin.Route {
	return []plugin.Route{
		{Slug: "settings", Description: "Settings"},
	}
}

func (p *Plugin) NavigateTo(route string, args map[string]string) {
	// The sidebar layout handles all navigation internally.
	// External navigation just activates the settings tab.
}

func (p *Plugin) RefreshInterval() time.Duration { return 0 }
func (p *Plugin) Refresh() tea.Cmd               { return nil }
func (p *Plugin) Shutdown()                       {}

func (p *Plugin) Init(ctx plugin.Context) error {
	p.cfg = ctx.Config
	p.logger = ctx.Logger
	p.bus = ctx.Bus

	pal := config.GetPalette(p.cfg.Palette, p.cfg.Colors)
	p.styles = newSettingsStyles(pal)

	// Set palette cursor to current palette
	for i, name := range config.PaletteNames() {
		if name == p.cfg.Palette {
			p.paletteCursor = i
			break
		}
	}

	// Initialize providers map and register data source settings providers.
	if p.providers == nil {
		p.providers = make(map[string]plugin.SettingsProvider)
	}
	p.providers["calendar"] = calendar.NewSettings(p.cfg, pal)
	p.providers["github"] = ghsettings.NewSettings(p.cfg, pal)
	p.providers["granola"] = granola.NewSettings(p.cfg, pal)

	// Banner text inputs
	ni := textinput.New()
	ni.Placeholder = "Claude Command"
	ni.CharLimit = 20
	ni.SetValue(p.cfg.Name)
	p.bannerNameInput = ni

	si := textinput.New()
	si.Placeholder = "Center"
	si.CharLimit = 30
	si.SetValue(p.cfg.Subtitle)
	p.bannerSubtitleInput = si

	// Build sidebar navigation
	p.rebuildNav()

	// Subscribe to todo events for logging
	if p.bus != nil {
		todoTopics := []string{"todo.completed", "todo.created", "todo.dismissed", "todo.deferred", "todo.promoted", "todo.edited"}
		for _, topic := range todoTopics {
			t := topic // capture
			p.bus.Subscribe(t, func(e plugin.Event) {
				if p.logger != nil {
					if m, ok := e.Payload.(map[string]interface{}); ok {
						title, _ := m["title"].(string)
						p.logger.Info("settings", fmt.Sprintf("event %s: %s", t, title))
					}
				}
			})
		}
	}

	return nil
}

// RegisterProvider adds a SettingsProvider for a given slug.
// This allows data source packages to provide their own settings UI.
func (p *Plugin) RegisterProvider(slug string, sp plugin.SettingsProvider) {
	if p.providers == nil {
		p.providers = make(map[string]plugin.SettingsProvider)
	}
	p.providers[slug] = sp
}

// StartCmds returns initial commands (none needed for settings).
func (p *Plugin) StartCmds() tea.Cmd { return nil }

func (p *Plugin) KeyBindings() []plugin.KeyBinding {
	return []plugin.KeyBinding{
		{Key: "up/down", Description: "Navigate", Promoted: true},
		{Key: "enter/right", Description: "Open content pane", Promoted: true},
		{Key: "esc/left", Description: "Back to sidebar", Promoted: true},
		{Key: "space", Description: "Toggle enable/disable", Promoted: true},
	}
}

func (p *Plugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	switch p.focusZone {
	case FocusNav:
		return p.handleNavKey(msg)
	case FocusContent, FocusEditing:
		return p.handleContentKey(msg)
	}
	return plugin.NoopAction()
}

func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return false, plugin.NoopAction()
	case systemActionResult:
		p.handleSystemActionResult(msg)
		return true, plugin.NoopAction()
	case plugin.TabLeaveMsg:
		// Cancel any active banner editing when leaving the tab
		if p.bannerEditing {
			p.bannerEditing = false
			p.bannerNameInput.SetValue(p.cfg.Name)
			p.bannerNameInput.Blur()
			p.bannerSubtitleInput.SetValue(p.cfg.Subtitle)
			p.bannerSubtitleInput.Blur()
		}
		return true, plugin.NoopAction()
	}

	// Clear flash after 10 seconds
	if p.flashMessage != "" && time.Since(p.flashMessageAt) > 10*time.Second {
		p.flashMessage = ""
	}

	return false, plugin.NoopAction()
}

func (p *Plugin) View(width, height, frame int) string {
	p.syncNavFromConfig()
	p.width = width
	p.height = height

	viewWidth := ui.ContentMaxWidth
	if width > 0 && width < viewWidth {
		viewWidth = width
	}

	// Sidebar + content split
	sidebarWidth := 28
	contentWidth := viewWidth - sidebarWidth - 4 // account for borders
	if contentWidth < 20 {
		contentWidth = 20
	}
	panelHeight := height - 4 // leave room for help line + flash
	if panelHeight < 10 {
		panelHeight = 10
	}

	sidebar := p.viewSidebar(sidebarWidth, panelHeight, p.focusZone)
	content := p.viewContent(contentWidth, panelHeight)

	layout := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)

	// Flash message
	var flash string
	if p.flashMessage != "" {
		flash = p.styles.flash.Render("  > " + p.flashMessage)
	}

	// Help line
	var help string
	switch p.focusZone {
	case FocusNav:
		help = p.styles.muted.Render("  up/down navigate  space toggle  enter/right open  esc back")
	case FocusContent:
		help = p.styles.muted.Render("  esc/left sidebar  up/down navigate  enter select  space toggle")
	case FocusEditing:
		help = p.styles.muted.Render("  enter save  esc cancel")
	}

	parts := []string{layout}
	if flash != "" {
		parts = append(parts, flash)
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// syncNavFromConfig updates nav item enabled states from the live config.
func (p *Plugin) syncNavFromConfig() {
	for i := range p.navCategories {
		for j := range p.navCategories[i].Items {
			item := &p.navCategories[i].Items[j]
			if item.Enabled == nil {
				continue
			}
			switch item.Kind {
			case "plugin":
				if len(item.Slug) > 9 && item.Slug[:9] == "external-" {
					// External plugins — find matching config entry
					for k, ep := range p.cfg.ExternalPlugins {
						if item.Slug == fmt.Sprintf("external-%d", k) {
							*item.Enabled = ep.Enabled
							break
						}
					}
				} else {
					*item.Enabled = p.cfg.PluginEnabled(item.Slug)
				}
			case "datasource":
				switch item.Slug {
				case "calendar":
					*item.Enabled = p.cfg.Calendar.Enabled
				case "github":
					*item.Enabled = p.cfg.GitHub.Enabled
				case "granola":
					*item.Enabled = p.cfg.Granola.Enabled
				case "slack":
					*item.Enabled = p.cfg.Slack.Enabled
				case "gmail":
					*item.Enabled = p.cfg.Gmail.Enabled
				case "todos":
					*item.Enabled = p.cfg.Todos.Enabled
				case "threads":
					*item.Enabled = p.cfg.Threads.Enabled
				}
			}
		}
	}
}

// publishConfigSaved publishes a config.saved event via the bus.
func (p *Plugin) publishConfigSaved(keysChanged string) {
	if p.bus != nil {
		p.bus.Publish(plugin.Event{
			Source: "settings",
			Topic:  "config.saved",
			Payload: map[string]interface{}{
				"keys_changed": keysChanged,
			},
		})
	}
}

func (p *Plugin) validateDataSource(slug string) error {
	switch slug {
	case "calendar":
		return config.ValidateCalendar()
	case "github":
		return config.ValidateGitHub()
	case "granola":
		return config.ValidateGranola()
	case "slack":
		return config.ValidateSlack()
	case "gmail":
		return config.ValidateGmail()
	}
	return nil
}

func renderSwatches(pal config.Palette) string {
	colors := []string{pal.Cyan, pal.Yellow, pal.Purple, pal.Green, pal.White}
	var parts []string
	for _, c := range colors {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("██"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// handleBannerKey processes key events for the banner editing content pane.
func (p *Plugin) handleBannerKey(msg tea.KeyMsg) plugin.Action {
	// When editing a text field, route to the textinput.
	if p.bannerEditing {
		switch msg.Type {
		case tea.KeyEnter:
			// Save the value and exit editing.
			p.bannerEditing = false
			p.focusZone = FocusContent
			if p.bannerField == 0 {
				p.bannerNameInput.Blur()
				p.cfg.Name = p.bannerNameInput.Value()
			} else {
				p.bannerSubtitleInput.Blur()
				p.cfg.Subtitle = p.bannerSubtitleInput.Value()
			}
			if err := config.Save(p.cfg); err == nil {
				p.flashMessage = "Banner saved"
				p.publishConfigSaved("banner")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
			p.flashMessageAt = time.Now()
			return plugin.NoopAction()
		case tea.KeyEsc:
			// Cancel editing, restore original value.
			p.bannerEditing = false
			p.focusZone = FocusContent
			if p.bannerField == 0 {
				p.bannerNameInput.SetValue(p.cfg.Name)
				p.bannerNameInput.Blur()
			} else {
				p.bannerSubtitleInput.SetValue(p.cfg.Subtitle)
				p.bannerSubtitleInput.Blur()
			}
			return plugin.NoopAction()
		default:
			if p.bannerField == 0 {
				p.bannerNameInput, _ = p.bannerNameInput.Update(msg)
			} else {
				p.bannerSubtitleInput, _ = p.bannerSubtitleInput.Update(msg)
			}
			return plugin.NoopAction()
		}
	}

	// Not editing — navigation mode within the banner content pane.
	switch msg.String() {
	case "up", "k":
		if p.bannerField > 0 {
			p.bannerField--
		}
	case "down", "j":
		if p.bannerField < 2 {
			p.bannerField++
		}
	case "enter":
		if p.bannerField <= 1 {
			// Start editing text field.
			p.bannerEditing = true
			p.focusZone = FocusEditing
			if p.bannerField == 0 {
				p.bannerNameInput.Focus()
			} else {
				p.bannerSubtitleInput.Focus()
			}
		}
	case " ":
		if p.bannerField == 2 {
			// Toggle show/hide.
			p.cfg.SetShowBanner(!p.cfg.BannerVisible())
			if err := config.Save(p.cfg); err == nil {
				if p.cfg.BannerVisible() {
					p.flashMessage = "Banner shown"
				} else {
					p.flashMessage = "Banner hidden"
				}
				p.publishConfigSaved("banner")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
			p.flashMessageAt = time.Now()
		}
	}
	return plugin.NoopAction()
}
