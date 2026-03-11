// Package settings implements the Settings plugin for CCC.
// It provides a UI for viewing plugins, toggling data sources,
// viewing logs, and picking color palettes.
package settings

import (
	"fmt"
	"os/exec"
	"strings"
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


// settingsItem represents a plugin or data source in the settings list.
type settingsItem struct {
	name       string
	slug       string
	kind       string // "builtin-plugin", "external-plugin", "datasource"
	enabled    bool
	toggleable bool
	valid      *bool // nil = not checked, true/false = credential check result
	validHint  string
}

// Plugin implements the plugin.Plugin interface for Settings.
type Plugin struct {
	cfg      *config.Config
	logger   plugin.Logger
	registry *plugin.Registry
	bus      plugin.EventBus
	styles   settingsStyles

	// SettingsProvider implementations for data sources
	providers map[string]plugin.SettingsProvider

	subView       string // "plugins", "logs", "palette", "banner"
	cursor        int
	logOffset     int
	paletteCursor int
	items         []settingsItem

	// Banner editing state
	bannerNameInput     textinput.Model
	bannerSubtitleInput textinput.Model
	bannerField         int  // 0=name, 1=subtitle, 2=show/hide
	bannerEditing       bool // true when a text field is focused

	// Detail view state
	detailView   bool
	detailIdx    int
	detailCursor int // cursor within detail view fields

	flashMessage   string
	flashMessageAt time.Time

	width, height int
}

// New creates a new Settings plugin. The registry is used to enumerate all plugins.
func New(registry *plugin.Registry) *Plugin {
	return &Plugin{
		subView:  "plugins",
		registry: registry,
	}
}

func (p *Plugin) Slug() string    { return "settings" }
func (p *Plugin) TabName() string { return "Settings" }

func (p *Plugin) Migrations() []plugin.Migration { return nil }

func (p *Plugin) Routes() []plugin.Route {
	return []plugin.Route{
		{Slug: "settings", Description: "Settings"},
		{Slug: "settings/logs", Description: "Logs"},
		{Slug: "settings/palette", Description: "Palette"},
		{Slug: "settings/banner", Description: "Banner"},
	}
}

func (p *Plugin) NavigateTo(route string, args map[string]string) {
	switch route {
	case "settings/logs":
		p.subView = "logs"
	case "settings/palette":
		p.subView = "palette"
	case "settings/banner":
		p.subView = "banner"
	default:
		p.subView = "plugins"
	}
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

	p.rebuildItems()
	p.validateAllSources()

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

	// Initialize providers map and register data source settings providers.
	if p.providers == nil {
		p.providers = make(map[string]plugin.SettingsProvider)
	}
	p.providers["calendar"] = calendar.NewSettings(p.cfg, pal)
	p.providers["github"] = ghsettings.NewSettings(p.cfg, pal)
	p.providers["granola"] = granola.NewSettings(p.cfg, pal)

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

// rebuildItems populates the items list from registry and config.
func (p *Plugin) rebuildItems() {
	p.items = nil

	// Built-in plugins from registry
	if p.registry != nil {
		for _, plug := range p.registry.All() {
			slug := plug.Slug()
			// Settings itself is not toggleable
			toggleable := slug != "settings"
			p.items = append(p.items, settingsItem{
				name:       plug.TabName(),
				slug:       slug,
				kind:       "builtin-plugin",
				enabled:    p.cfg.PluginEnabled(slug),
				toggleable: toggleable,
			})
		}
	}

	// External plugins from config
	for i, ep := range p.cfg.ExternalPlugins {
		p.items = append(p.items, settingsItem{
			name:       ep.Name,
			slug:       fmt.Sprintf("external-%d", i),
			kind:       "external-plugin",
			enabled:    ep.Enabled,
			toggleable: true,
		})
	}

	// Data sources
	p.items = append(p.items, settingsItem{
		name: "Todos", slug: "todos", kind: "datasource",
		enabled: p.cfg.Todos.Enabled, toggleable: false,
	})
	p.items = append(p.items, settingsItem{
		name: "Threads", slug: "threads", kind: "datasource",
		enabled: p.cfg.Threads.Enabled, toggleable: true,
	})
	p.items = append(p.items, settingsItem{
		name: "Calendar", slug: "calendar", kind: "datasource",
		enabled: p.cfg.Calendar.Enabled, toggleable: true,
	})
	p.items = append(p.items, settingsItem{
		name: "GitHub", slug: "github", kind: "datasource",
		enabled: p.cfg.GitHub.Enabled, toggleable: true,
	})
	p.items = append(p.items, settingsItem{
		name: "Granola", slug: "granola", kind: "datasource",
		enabled: p.cfg.Granola.Enabled, toggleable: true,
	})
	p.items = append(p.items, settingsItem{
		name: "Slack", slug: "slack", kind: "datasource",
		enabled: p.cfg.Slack.Enabled, toggleable: true,
	})
}

// validateAllSources runs credential checks on all data sources and updates valid status.
func (p *Plugin) validateAllSources() {
	for i := range p.items {
		if p.items[i].kind != "datasource" {
			continue
		}
		err := p.validateDataSource(p.items[i].slug)
		if err != nil {
			v := false
			p.items[i].valid = &v
			p.items[i].validHint = err.Error()
		} else {
			v := true
			p.items[i].valid = &v
			p.items[i].validHint = ""
		}
	}
}

func (p *Plugin) KeyBindings() []plugin.KeyBinding {
	return []plugin.KeyBinding{
		{Key: "up/down", Description: "Navigate list", Promoted: true},
		{Key: "enter/space", Description: "Toggle enable/disable", Mode: "plugins", Promoted: true},
		{Key: "l", Description: "View logs", Promoted: true},
		{Key: "p", Description: "Pick palette", Promoted: true},
		{Key: "b", Description: "Edit banner", Promoted: true},
		{Key: "s", Description: "Plugin list", Promoted: true},
		{Key: "left/right", Description: "Cycle palettes", Mode: "palette", Promoted: true},
		{Key: "enter", Description: "Apply palette", Mode: "palette", Promoted: true},
	}
}

func (p *Plugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	// Detail view takes precedence
	if p.detailView {
		return p.handleDetailKey(msg)
	}

	// Sub-view switching
	switch msg.String() {
	case "l":
		if p.subView != "logs" {
			p.subView = "logs"
			p.logOffset = 0
			return plugin.NoopAction()
		}
	case "s":
		if p.subView != "plugins" {
			p.subView = "plugins"
			return plugin.NoopAction()
		}
	case "p":
		if p.subView != "palette" {
			p.subView = "palette"
			return plugin.NoopAction()
		}
	case "b":
		if p.subView != "banner" && !p.bannerEditing {
			p.subView = "banner"
			p.bannerField = 0
			p.bannerEditing = false
			return plugin.NoopAction()
		}
	}

	switch p.subView {
	case "plugins":
		return p.handlePluginsKey(msg)
	case "logs":
		return p.handleLogsKey(msg)
	case "palette":
		return p.handlePaletteKey(msg)
	case "banner":
		return p.handleBannerKey(msg)
	}
	return plugin.NoopAction()
}

func (p *Plugin) handlePluginsKey(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "j":
		if p.cursor < len(p.items)-1 {
			p.cursor++
		}
	case " ":
		// Space toggles directly
		if p.cursor < len(p.items) {
			item := &p.items[p.cursor]
			if !item.toggleable {
				return plugin.NoopAction()
			}
			item.enabled = !item.enabled
			p.applyToggle(*item)
		}
	case "enter":
		// Enter opens detail view
		if p.cursor < len(p.items) {
			p.openDetailView(p.cursor)
		}
	case "t":
		// Re-validate all data source credentials
		p.validateAllSources()
		p.flashMessage = "Credentials re-checked"
		p.flashMessageAt = time.Now()
	case "esc":
		return plugin.Action{Type: plugin.ActionUnhandled}
	}
	return plugin.NoopAction()
}

func (p *Plugin) handleLogsKey(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "up", "k":
		if p.logOffset > 0 {
			p.logOffset--
		}
	case "down", "j":
		p.logOffset++
	case "esc":
		return plugin.Action{Type: plugin.ActionUnhandled}
	}
	return plugin.NoopAction()
}

func (p *Plugin) handlePaletteKey(msg tea.KeyMsg) plugin.Action {
	names := config.PaletteNames()
	switch msg.String() {
	case "left", "h":
		if p.paletteCursor > 0 {
			p.paletteCursor--
		}
	case "right", "l":
		if p.paletteCursor < len(names)-1 {
			p.paletteCursor++
		}
	case "enter":
		selected := names[p.paletteCursor]
		previous := p.cfg.Palette
		p.cfg.Palette = selected
		if err := config.Save(p.cfg); err == nil {
			p.flashMessage = "Palette saved: " + selected
			p.publishConfigSaved("palette")
			if p.bus != nil {
				p.bus.Publish(plugin.Event{
					Source: "settings",
					Topic:  "palette.changed",
					Payload: map[string]interface{}{
						"previous": previous,
						"new":      selected,
					},
				})
			}
		} else {
			p.flashMessage = "Failed to save palette: " + err.Error()
		}
		p.flashMessageAt = time.Now()
	case "esc":
		return plugin.Action{Type: plugin.ActionUnhandled}
	}
	return plugin.NoopAction()
}

// applyToggle persists a toggle change to the config file.
// For data sources, validates credentials when enabling.
func (p *Plugin) applyToggle(item settingsItem) {
	switch item.kind {
	case "builtin-plugin":
		p.cfg.SetPluginEnabled(item.slug, item.enabled)
		if err := config.Save(p.cfg); err == nil {
			if item.enabled {
				p.flashMessage = item.name + " enabled"
			} else {
				p.flashMessage = item.name + " disabled"
			}
			p.publishConfigSaved("disabled_plugins")
		} else {
			p.flashMessage = "Failed to save: " + err.Error()
		}
		p.flashMessageAt = time.Now()

	case "external-plugin":
		// Find the matching external plugin config by slug index
		epIdx := -1
		for i := range p.cfg.ExternalPlugins {
			if item.slug == fmt.Sprintf("external-%d", i) {
				epIdx = i
				break
			}
		}
		if epIdx < 0 {
			return
		}
		// Validate command exists when enabling
		if item.enabled {
			parts := strings.Fields(p.cfg.ExternalPlugins[epIdx].Command)
			if len(parts) > 0 {
				if _, err := exec.LookPath(parts[0]); err != nil {
					for i := range p.items {
						if p.items[i].slug == item.slug {
							p.items[i].enabled = false
							break
						}
					}
					p.flashMessage = fmt.Sprintf("Command not found: %s", parts[0])
					p.flashMessageAt = time.Now()
					return
				}
			}
		}
		p.cfg.ExternalPlugins[epIdx].Enabled = item.enabled
		if err := config.Save(p.cfg); err == nil {
			p.flashMessage = "Restart CCC to apply"
			p.publishConfigSaved("external_plugins")
		} else {
			p.flashMessage = "Failed to save: " + err.Error()
		}
		p.flashMessageAt = time.Now()

	case "datasource":
		// Validate credentials when enabling
		if item.enabled {
			if err := p.validateDataSource(item.slug); err != nil {
				// Revert the toggle
				for i := range p.items {
					if p.items[i].slug == item.slug {
						p.items[i].enabled = false
						break
					}
				}
				p.flashMessage = err.Error()
				p.flashMessageAt = time.Now()
				return
			}
		}

		switch item.slug {
		case "calendar":
			p.cfg.Calendar.Enabled = item.enabled
		case "github":
			p.cfg.GitHub.Enabled = item.enabled
		case "threads":
			p.cfg.Threads.Enabled = item.enabled
		case "granola":
			p.cfg.Granola.Enabled = item.enabled
		case "slack":
			p.cfg.Slack.Enabled = item.enabled
		}
		if err := config.Save(p.cfg); err == nil {
			p.flashMessage = "Changes apply on next refresh"
			p.publishConfigSaved(item.slug)
			if p.bus != nil {
				p.bus.Publish(plugin.Event{
					Source: "settings",
					Topic:  "datasource.toggled",
					Payload: map[string]interface{}{
						"name":    item.slug,
						"enabled": item.enabled,
					},
				})
			}
		} else {
			p.flashMessage = "Failed to save: " + err.Error()
		}
		p.flashMessageAt = time.Now()
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
	}
	return nil
}

// openDetailView enters the detail screen for the item at index idx.
func (p *Plugin) openDetailView(idx int) {
	p.detailView = true
	p.detailIdx = idx
	p.detailCursor = 0

	// Reset provider editing state if applicable.
	item := p.items[idx]
	if sp, ok := p.providers[item.slug]; ok {
		if resetter, ok := sp.(interface{ ResetEditing() }); ok {
			resetter.ResetEditing()
		}
	}
}

func (p *Plugin) handleDetailKey(msg tea.KeyMsg) plugin.Action {
	item := p.items[p.detailIdx]

	// esc is always handled by the settings plugin.
	if msg.String() == "esc" {
		p.detailView = false
		return plugin.NoopAction()
	}

	// Check SettingsProvider first — providers handle their own cursor, editing, etc.
	if sp, ok := p.providers[item.slug]; ok {
		action := sp.HandleSettingsKey(msg)
		if action.Type == plugin.ActionFlash {
			p.flashMessage = action.Payload
			p.flashMessageAt = time.Now()
			return plugin.NoopAction()
		}
		if action.Type != plugin.ActionUnhandled {
			return action
		}
	} else if plug, ok := p.registry.BySlug(item.slug); ok {
		if sp, ok := plug.(plugin.SettingsProvider); ok {
			action := sp.HandleSettingsKey(msg)
			if action.Type == plugin.ActionFlash {
				p.flashMessage = action.Payload
				p.flashMessageAt = time.Now()
				return plugin.NoopAction()
			}
			if action.Type != plugin.ActionUnhandled {
				return action
			}
		}
	}

	// Space toggles the data source (when provider didn't handle it).
	if msg.String() == " " {
		if item.toggleable {
			p.items[p.detailIdx].enabled = !p.items[p.detailIdx].enabled
			p.applyToggle(p.items[p.detailIdx])
		}
		return plugin.NoopAction()
	}

	// Generic navigation for views without a provider (or unhandled keys).
	switch msg.String() {
	case "up", "k":
		if p.detailCursor > 0 {
			p.detailCursor--
		}
	case "down", "j":
		p.detailCursor++
	}

	return plugin.NoopAction()
}

func (p *Plugin) viewDetail(width, height int) string {
	if p.detailIdx >= len(p.items) {
		p.detailView = false
		return ""
	}
	item := p.items[p.detailIdx]
	var lines []string

	lines = append(lines, p.styles.header.Render(strings.ToUpper(item.name)))
	lines = append(lines, "")

	switch item.kind {
	case "builtin-plugin":
		if plug, ok := p.registry.BySlug(item.slug); ok {
			if sp, ok := plug.(plugin.SettingsProvider); ok {
				lines = append(lines, sp.SettingsView(width, height))
			} else {
				lines = append(lines, p.viewDetailBuiltinPlugin(item)...)
			}
		} else {
			lines = append(lines, p.viewDetailBuiltinPlugin(item)...)
		}
	case "external-plugin":
		lines = append(lines, p.viewDetailExternalPlugin(item)...)
	case "datasource":
		if sp, ok := p.providers[item.slug]; ok {
			lines = append(lines, sp.SettingsView(width, height))
		} else {
			lines = append(lines, p.viewDetailBuiltinPlugin(item)...)
		}
	}

	// Flash message
	if p.flashMessage != "" {
		lines = append(lines, "")
		lines = append(lines, p.styles.flash.Render("  > "+p.flashMessage))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  esc back"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return p.styles.panel.Width(width - 4).Render(content)
}

func (p *Plugin) viewDetailBuiltinPlugin(item settingsItem) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("  %s %s",
		p.styles.muted.Render("Slug:"),
		p.styles.itemName.Render(item.slug)))
	lines = append(lines, fmt.Sprintf("  %s %s",
		p.styles.muted.Render("Type:"),
		p.styles.itemName.Render("Core plugin")))
	lines = append(lines, fmt.Sprintf("  %s %s",
		p.styles.muted.Render("Status:"),
		p.styles.enabled.Render("Always enabled")))
	return lines
}

func (p *Plugin) viewDetailExternalPlugin(item settingsItem) []string {
	var lines []string

	statusText := "Disabled"
	statusStyle := p.styles.disabled
	if item.enabled {
		statusText = "Enabled"
		statusStyle = p.styles.enabled
	}

	lines = append(lines, fmt.Sprintf("  %s %s",
		p.styles.muted.Render("Name:"),
		p.styles.itemName.Render(item.name)))
	lines = append(lines, fmt.Sprintf("  %s %s",
		p.styles.muted.Render("Status:"),
		statusStyle.Render(statusText)))

	// Find the matching config entry
	for i, ep := range p.cfg.ExternalPlugins {
		if item.slug == fmt.Sprintf("external-%d", i) {
			lines = append(lines, fmt.Sprintf("  %s %s",
				p.styles.muted.Render("Command:"),
				p.styles.itemName.Render(ep.Command)))
			break
		}
	}
	return lines
}


func (p *Plugin) handleBannerKey(msg tea.KeyMsg) plugin.Action {
	// When editing a text field, route to the textinput.
	if p.bannerEditing {
		switch msg.Type {
		case tea.KeyEnter:
			// Save the value and exit editing.
			p.bannerEditing = false
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

	// Not editing — navigation mode.
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
	case "esc":
		return plugin.Action{Type: plugin.ActionUnhandled}
	}
	return plugin.NoopAction()
}

func (p *Plugin) viewBanner(width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render("SETTINGS"))
	lines = append(lines, "")
	lines = append(lines, p.renderSubTabs())
	lines = append(lines, "")
	lines = append(lines, p.styles.header.Render("BANNER"))
	lines = append(lines, "")

	fields := []struct {
		label string
		value string
	}{
		{"Name", p.cfg.Name},
		{"Subtitle", p.cfg.Subtitle},
	}

	for i, f := range fields {
		cursor := "  "
		if i == p.bannerField {
			cursor = p.styles.pointer.Render("> ")
		}

		if p.bannerEditing && i == p.bannerField {
			var input string
			if i == 0 {
				input = p.bannerNameInput.View()
			} else {
				input = p.bannerSubtitleInput.View()
			}
			lines = append(lines, fmt.Sprintf("%s%s %s",
				cursor, p.styles.muted.Render(f.label+":"), input))
		} else {
			val := f.value
			if val == "" {
				val = "(empty)"
			}
			lines = append(lines, fmt.Sprintf("%s%s %s",
				cursor, p.styles.muted.Render(f.label+":"), p.styles.itemName.Render(val)))
		}
	}

	// Show/hide toggle
	cursor := "  "
	if p.bannerField == 2 {
		cursor = p.styles.pointer.Render("> ")
	}
	status := p.styles.enabled.Render("[on] ")
	if !p.cfg.BannerVisible() {
		status = p.styles.disabled.Render("[off]")
	}
	lines = append(lines, fmt.Sprintf("%s%s %s",
		cursor, status, p.styles.itemName.Render("Show Banner")))

	// Flash message
	if p.flashMessage != "" {
		lines = append(lines, "")
		lines = append(lines, p.styles.flash.Render("  > "+p.flashMessage))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  ↑↓ navigate · enter edit · space toggle · esc back"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return p.styles.panel.Width(width - 4).Render(content)
}

func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg.(type) {
	case tea.WindowSizeMsg:
		m := msg.(tea.WindowSizeMsg)
		p.width = m.Width
		p.height = m.Height
		return false, plugin.NoopAction()
	}

	// Clear flash after 10 seconds
	if p.flashMessage != "" && time.Since(p.flashMessageAt) > 10*time.Second {
		p.flashMessage = ""
	}

	return false, plugin.NoopAction()
}

// syncEnabledFromConfig updates item enabled states from the live config.
// This ensures settings reflects changes made by onboarding or external edits.
func (p *Plugin) syncEnabledFromConfig() {
	for i := range p.items {
		switch p.items[i].kind {
		case "builtin-plugin":
			p.items[i].enabled = p.cfg.PluginEnabled(p.items[i].slug)
		case "datasource":
			switch p.items[i].slug {
			case "calendar":
				p.items[i].enabled = p.cfg.Calendar.Enabled
			case "github":
				p.items[i].enabled = p.cfg.GitHub.Enabled
			case "granola":
				p.items[i].enabled = p.cfg.Granola.Enabled
			case "slack":
				p.items[i].enabled = p.cfg.Slack.Enabled
			case "todos":
				p.items[i].enabled = p.cfg.Todos.Enabled
			case "threads":
				p.items[i].enabled = p.cfg.Threads.Enabled
			}
		}
	}
}

func (p *Plugin) View(width, height, frame int) string {
	p.syncEnabledFromConfig()
	p.width = width
	p.height = height

	viewWidth := ui.ContentMaxWidth
	if width > 0 && width < viewWidth {
		viewWidth = width
	}

	if p.detailView {
		return p.viewDetail(viewWidth, height)
	}

	switch p.subView {
	case "logs":
		return p.viewLogs(viewWidth, height)
	case "palette":
		return p.viewPalette(viewWidth, height)
	case "banner":
		return p.viewBanner(viewWidth, height)
	default:
		return p.viewPlugins(viewWidth, height)
	}
}

func (p *Plugin) viewPlugins(width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render("SETTINGS"))
	lines = append(lines, "")

	// Sub-view tabs
	lines = append(lines, p.renderSubTabs())
	lines = append(lines, "")

	// Plugins section
	lines = append(lines, p.styles.header.Render("PLUGINS"))
	lines = append(lines, "")

	inDataSources := false
	for i, item := range p.items {
		// Add data sources header when we transition
		if item.kind == "datasource" && !inDataSources {
			inDataSources = true
			lines = append(lines, "")
			lines = append(lines, p.styles.header.Render("DATA SOURCES"))
			lines = append(lines, p.styles.muted.Render("  Controls what ccc-refresh fetches"))
			lines = append(lines, "")
		}

		cursor := "  "
		if i == p.cursor {
			cursor = p.styles.pointer.Render("> ")
		}

		status := p.styles.enabled.Render("[on] ")
		if !item.enabled {
			status = p.styles.disabled.Render("[off]")
		}

		nameStyle := p.styles.itemName
		if !item.enabled {
			nameStyle = p.styles.muted
		}

		label := nameStyle.Render(item.name)

		var suffix string
		if !item.toggleable {
			suffix = p.styles.muted.Render(" (core)")
		}
		if item.valid != nil && item.kind == "datasource" {
			if *item.valid {
				suffix += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Render("✓")
			} else {
				suffix += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("✗")
			}
		}

		lines = append(lines, fmt.Sprintf("%s%s %s%s", cursor, status, label, suffix))
	}

	// Flash message
	if p.flashMessage != "" {
		lines = append(lines, "")
		lines = append(lines, p.styles.flash.Render("  > "+p.flashMessage))
	}

	// Footer hints
	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  ↑↓ navigate · space toggle · enter detail · t test connections · l logs · p palette"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return p.styles.panel.Width(width - 4).Render(content)
}

func (p *Plugin) viewLogs(width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render("SETTINGS"))
	lines = append(lines, "")
	lines = append(lines, p.renderSubTabs())
	lines = append(lines, "")
	lines = append(lines, p.styles.header.Render("LOGS"))
	lines = append(lines, "")

	if p.logger == nil {
		lines = append(lines, p.styles.muted.Render("  No logger available"))
	} else {
		entries := p.logger.Recent(100)
		if len(entries) == 0 {
			lines = append(lines, p.styles.muted.Render("  No log entries"))
		} else {
			// Reverse chronological
			maxVisible := height - 14
			if maxVisible < 5 {
				maxVisible = 5
			}

			// Clamp logOffset
			if p.logOffset > len(entries)-maxVisible {
				p.logOffset = len(entries) - maxVisible
			}
			if p.logOffset < 0 {
				p.logOffset = 0
			}

			start := len(entries) - 1 - p.logOffset
			end := start - maxVisible
			if end < 0 {
				end = -1
			}

			for i := start; i > end; i-- {
				e := entries[i]
				timeStr := e.Time.Format("15:04:05")
				var levelStyle lipgloss.Style
				switch e.Level {
				case "ERROR":
					levelStyle = p.styles.logError
				case "WARN":
					levelStyle = p.styles.logWarn
				default:
					levelStyle = p.styles.muted
				}
				line := fmt.Sprintf("  %s %s %s %s",
					p.styles.muted.Render(timeStr),
					levelStyle.Render(fmt.Sprintf("%-5s", e.Level)),
					p.styles.logPlugin.Render(fmt.Sprintf("%-15s", e.Plugin)),
					e.Message,
				)
				lines = append(lines, line)
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  ↑↓ scroll · s plugins · p palette"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return p.styles.panel.Width(width - 4).Render(content)
}

func (p *Plugin) viewPalette(width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render("SETTINGS"))
	lines = append(lines, "")
	lines = append(lines, p.renderSubTabs())
	lines = append(lines, "")
	lines = append(lines, p.styles.header.Render("PALETTE"))
	lines = append(lines, "")

	names := config.PaletteNames()
	for i, name := range names {
		pal := config.GetPalette(name, nil)

		cursor := "  "
		if i == p.paletteCursor {
			cursor = p.styles.pointer.Render("> ")
		}

		active := ""
		if name == p.cfg.Palette {
			active = p.styles.enabled.Render(" (active)")
		}

		// Color swatches
		swatches := renderSwatches(pal)

		nameStyle := p.styles.itemName
		if i == p.paletteCursor {
			nameStyle = nameStyle.Bold(true)
		}

		lines = append(lines, fmt.Sprintf("%s%s%s  %s", cursor, nameStyle.Render(name), active, swatches))
	}

	// Flash message
	if p.flashMessage != "" {
		lines = append(lines, "")
		lines = append(lines, p.styles.flash.Render("  > "+p.flashMessage))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  ←→ cycle · enter apply · s plugins · l logs"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return p.styles.panel.Width(width - 4).Render(content)
}

func (p *Plugin) renderSubTabs() string {
	tabs := []struct {
		label string
		key   string
	}{
		{"Plugins", "plugins"},
		{"Logs", "logs"},
		{"Palette", "palette"},
		{"Banner", "banner"},
	}

	var parts []string
	for _, t := range tabs {
		if t.key == p.subView {
			parts = append(parts, p.styles.activeTab.Render("> "+t.label))
		} else {
			parts = append(parts, p.styles.muted.Render(t.label))
		}
	}
	return "  " + strings.Join(parts, p.styles.muted.Render(" | "))
}

func renderSwatches(pal config.Palette) string {
	colors := []string{pal.Cyan, pal.Yellow, pal.Purple, pal.Green, pal.White}
	var parts []string
	for _, c := range colors {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("██"))
	}
	return strings.Join(parts, " ")
}

// settingsStyles holds styles for the settings plugin.
type settingsStyles struct {
	header    lipgloss.Style
	muted     lipgloss.Style
	pointer   lipgloss.Style
	enabled   lipgloss.Style
	disabled  lipgloss.Style
	itemName  lipgloss.Style
	flash     lipgloss.Style
	panel     lipgloss.Style
	activeTab lipgloss.Style
	logError  lipgloss.Style
	logWarn   lipgloss.Style
	logPlugin lipgloss.Style
}

func newSettingsStyles(p config.Palette) settingsStyles {
	return settingsStyles{
		header:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan)).Bold(true),
		muted:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted)),
		pointer:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.Pointer)),
		enabled:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.Green)),
		disabled:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted)),
		itemName:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.White)),
		flash:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.Green)),
		activeTab: lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan)).Bold(true),
		panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3b4261")),
		logError:  lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")),
		logWarn:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.Yellow)),
		logPlugin: lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan)),
	}
}
