// Package settings implements the Settings plugin for CCC.
// It provides a sidebar-based UI for appearance, plugins, data sources,
// system actions, and logs.
package settings

import (
	"fmt"
	"os"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/refresh/sources/calendar"
	ghsettings "github.com/anutron/claude-command-center/internal/refresh/sources/github"
	"github.com/anutron/claude-command-center/internal/refresh/sources/gmail"
	"github.com/anutron/claude-command-center/internal/refresh/sources/granola"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
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

	// Active huh form (nil when no form is displayed)
	activeForm *huh.Form

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
	case FocusForm:
		return p.handleFormKey(msg)
	}
	return plugin.NoopAction()
}

// handleFormKey processes key events when a huh form is focused.
func (p *Plugin) handleFormKey(msg tea.KeyMsg) plugin.Action {
	if p.activeForm == nil {
		p.focusZone = FocusContent
		return plugin.NoopAction()
	}

	// Allow escape to cancel the form
	if msg.String() == "esc" {
		p.activeForm = nil
		p.focusZone = FocusContent
		return plugin.NoopAction()
	}

	// Forward key to the form
	form, cmd := p.activeForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		p.activeForm = f
	}

	// Check if form completed
	if p.activeForm.State == huh.StateCompleted {
		// Leave form active for the caller to read values, but return to content
		p.focusZone = FocusContent
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}

	if cmd != nil {
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}
	return plugin.NoopAction()
}

func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return false, plugin.NoopAction()
	case datasourceRecheckResult:
		p.applyRecheckResult(msg)
		return true, plugin.NoopAction()
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
		// Cancel any active form
		if p.activeForm != nil {
			p.activeForm = nil
			p.focusZone = FocusContent
		}
		return true, plugin.NoopAction()
	}

	// Route non-key messages to the active form when in FocusForm.
	if p.focusZone == FocusForm && p.activeForm != nil {
		form, cmd := p.activeForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			p.activeForm = f
		}
		if p.activeForm.State == huh.StateCompleted {
			p.focusZone = FocusContent
		}
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	}

	// Route to active provider's HandleSettingsMsg when content is focused.
	if p.focusZone == FocusContent || p.focusZone == FocusEditing {
		if sp := p.activeProvider(); sp != nil {
			if handled, action := sp.HandleSettingsMsg(msg); handled {
				if action.Type == plugin.ActionFlash {
					p.flashMessage = action.Payload
					p.flashMessageAt = time.Now()
					return true, plugin.NoopAction()
				}
				return true, action
			}
		}
	}

	// Clear flash after 10 seconds
	if p.flashMessage != "" && time.Since(p.flashMessageAt) > 10*time.Second {
		p.flashMessage = ""
	}

	return false, plugin.NoopAction()
}

// activeProvider returns the SettingsProvider for the currently selected nav item,
// or nil if there is none.
func (p *Plugin) activeProvider() plugin.SettingsProvider {
	item := p.selectedNavItem()
	if item == nil {
		return nil
	}
	// Data source providers
	if sp, ok := p.providers[item.Slug]; ok {
		return sp
	}
	// Plugin providers (from registry)
	if item.Kind == "plugin" {
		if plug, ok := p.registry.BySlug(item.Slug); ok {
			if sp, ok := plug.(plugin.SettingsProvider); ok {
				return sp
			}
		}
	}
	return nil
}

// activeProviderOpenCmd returns the SettingsOpenCmd for the currently selected
// nav item's provider, or nil if there is none.
func (p *Plugin) activeProviderOpenCmd() tea.Cmd {
	if sp := p.activeProvider(); sp != nil {
		return sp.SettingsOpenCmd()
	}
	return nil
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
		item := p.selectedNavItem()
		if item != nil && item.Kind == "datasource" {
			help = p.styles.muted.Render("  esc/left sidebar  up/down navigate  enter select  space toggle  r re-check")
		} else {
			help = p.styles.muted.Render("  esc/left sidebar  up/down navigate  enter select  space toggle")
		}
	case FocusEditing:
		help = p.styles.muted.Render("  enter save  esc cancel")
	case FocusForm:
		help = p.styles.muted.Render("  tab next field  enter submit  esc cancel")
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
				if item.Slug == "threads" {
					*item.Enabled = p.cfg.Threads.Enabled
				} else if len(item.Slug) > 9 && item.Slug[:9] == "external-" {
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

// validateDataSourceResult returns a tiered ValidationResult for a data source.
// It prefers DoctorProvider checks from the provider, falling back to the
// top-level ValidateXResult functions for calendar/gmail, and then to the
// legacy error-based validateDataSource for others.
func (p *Plugin) validateDataSourceResult(slug string) plugin.ValidationResult {
	// Try DoctorProvider on the SettingsProvider first
	if sp, ok := p.providers[slug]; ok {
		if dp, ok := sp.(plugin.DoctorProvider); ok {
			checks := dp.DoctorChecks(plugin.DoctorOpts{})
			if len(checks) > 0 {
				return checks[0].Result
			}
		}
	}

	// Fallback to standalone ValidateXResult functions
	switch slug {
	case "calendar":
		return calendar.ValidateCalendarResult()
	case "gmail":
		return gmail.ValidateGmailResult()
	case "slack":
		return validateSlackResult()
	}

	// Final fallback: legacy error-based check
	if err := p.validateDataSource(slug); err != nil {
		return plugin.ValidationResult{
			Status:  "missing",
			Message: err.Error(),
			Hint:    err.Error(),
		}
	}
	return plugin.ValidationResult{
		Status:  "ok",
		Message: slug + " credentials configured",
	}
}

// applyRecheckResult updates the NavItem for a data source after a re-check.
func (p *Plugin) applyRecheckResult(msg datasourceRecheckResult) {
	for i := range p.navCategories {
		for j := range p.navCategories[i].Items {
			item := &p.navCategories[i].Items[j]
			if item.Slug == msg.Slug && item.Kind == "datasource" {
				item.ValidationStatus = msg.Result.Status
				item.ValidationMsg = msg.Result.Message
				item.ValidHint = msg.Result.Hint
				if msg.Result.Status == "ok" {
					v := true
					item.Valid = &v
				} else if msg.Result.Status != "" {
					v := false
					item.Valid = &v
				}
				p.flashMessage = msg.Result.Message
				p.flashMessageAt = time.Now()
				return
			}
		}
	}
}

// validateSlackResult performs a structural check on Slack credentials.
func validateSlackResult() plugin.ValidationResult {
	if os.Getenv("SLACK_BOT_TOKEN") == "" {
		return plugin.ValidationResult{
			Status:  "missing",
			Message: "SLACK_BOT_TOKEN not set",
			Hint:    "Export SLACK_BOT_TOKEN in your shell profile",
		}
	}
	return plugin.ValidationResult{
		Status:  "ok",
		Message: "Slack token configured",
	}
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
