package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// viewContent renders the content pane based on the currently selected nav item.
func (p *Plugin) viewContent(width, height int) string {
	item := p.selectedNavItem()

	var body string
	if item == nil {
		body = p.styles.muted.Render("  Select an item from the sidebar")
	} else {
		body = p.renderContentForSlug(item, width, height)
	}

	// Pick panel style based on focus zone.
	var panelStyle lipgloss.Style
	if p.focusZone == FocusContent || p.focusZone == FocusEditing {
		panelStyle = p.styles.contentFocused
	} else {
		panelStyle = p.styles.contentUnfocused
	}

	return panelStyle.Width(width).Height(height).Render(body)
}

// renderContentForSlug dispatches to the correct content renderer for a given nav item.
func (p *Plugin) renderContentForSlug(item *NavItem, width, height int) string {
	switch item.Slug {
	// --- Appearance ---
	case "banner":
		return p.viewBannerContent(width, height)
	case "palette":
		return p.viewPaletteContent(width, height)

	// --- System ---
	case "system-schedule":
		return p.viewSystemPlaceholder("Schedule", "Refresh schedule configuration")
	case "system-mcp":
		return p.viewSystemPlaceholder("MCP Servers", "MCP server management")
	case "system-skills":
		return p.viewSystemPlaceholder("Skills", "Skill configuration")
	case "system-shell":
		return p.viewSystemPlaceholder("Shell Integration", "Shell integration settings")
	case "system-logs":
		return p.viewLogsContent(width, height)

	default:
		// Plugin or data source — dispatch by kind.
		switch item.Kind {
		case "plugin":
			return p.viewPluginContent(item, width, height)
		case "datasource":
			return p.viewDatasourceContent(item, width, height)
		}
	}

	return p.styles.muted.Render("  Select an item from the sidebar")
}

// handleContentKey dispatches key events to the correct content handler based on the selected slug.
func (p *Plugin) handleContentKey(msg tea.KeyMsg) plugin.Action {
	// Common escape: return to nav from content.
	if p.focusZone == FocusContent {
		switch msg.String() {
		case "esc", "left", "h":
			p.focusZone = FocusNav
			return plugin.NoopAction()
		}
	}

	item := p.selectedNavItem()
	if item == nil {
		return plugin.NoopAction()
	}

	switch item.Slug {
	case "banner":
		return p.handleBannerContentKey(msg)
	case "palette":
		return p.handlePaletteContentKey(msg)
	case "system-logs":
		return p.handleLogsContentKey(msg)
	case "system-schedule", "system-mcp", "system-skills", "system-shell":
		// Placeholder — no keys handled yet.
		return plugin.NoopAction()
	default:
		switch item.Kind {
		case "plugin":
			return p.handlePluginContentKey(item, msg)
		case "datasource":
			return p.handleDatasourceContentKey(item, msg)
		}
	}

	return plugin.NoopAction()
}

// --- Banner content (delegates to existing banner state) ---

func (p *Plugin) viewBannerContent(width, height int) string {
	var lines []string
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

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  enter edit  space toggle  esc back"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleBannerContentKey(msg tea.KeyMsg) plugin.Action {
	return p.handleBannerKey(msg)
}

// --- Palette content (delegates to existing palette state) ---

func (p *Plugin) viewPaletteContent(width, height int) string {
	var lines []string
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

		swatches := renderSwatches(pal)

		nameStyle := p.styles.itemName
		if i == p.paletteCursor {
			nameStyle = nameStyle.Bold(true)
		}

		lines = append(lines, fmt.Sprintf("%s%s%s  %s", cursor, nameStyle.Render(name), active, swatches))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  up/down cycle  enter apply  esc back"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handlePaletteContentKey(msg tea.KeyMsg) plugin.Action {
	names := config.PaletteNames()
	switch msg.String() {
	case "up", "k":
		if p.paletteCursor > 0 {
			p.paletteCursor--
		}
	case "down", "j":
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
	}
	return plugin.NoopAction()
}

// --- Logs content (delegates to existing logs state) ---

func (p *Plugin) viewLogsContent(width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render("LOGS"))
	lines = append(lines, "")

	if p.logger == nil {
		lines = append(lines, p.styles.muted.Render("  No logger available"))
	} else {
		entries := p.logger.Recent(100)
		if len(entries) == 0 {
			lines = append(lines, p.styles.muted.Render("  No log entries"))
		} else {
			maxVisible := height - 8
			if maxVisible < 5 {
				maxVisible = 5
			}

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
	lines = append(lines, p.styles.muted.Render("  up/down scroll  esc back"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleLogsContentKey(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "up", "k":
		if p.logOffset > 0 {
			p.logOffset--
		}
	case "down", "j":
		p.logOffset++
	}
	return plugin.NoopAction()
}

// --- Plugin content ---

func (p *Plugin) viewPluginContent(item *NavItem, width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render(strings.ToUpper(item.Label)))
	lines = append(lines, "")

	// Check if the plugin implements SettingsProvider
	if plug, ok := p.registry.BySlug(item.Slug); ok {
		if sp, ok := plug.(plugin.SettingsProvider); ok {
			lines = append(lines, sp.SettingsView(width, height))
			return lipgloss.JoinVertical(lipgloss.Left, lines...)
		}
	}

	// Default plugin info
	lines = append(lines, fmt.Sprintf("  %s %s",
		p.styles.muted.Render("Slug:"),
		p.styles.itemName.Render(item.Slug)))
	lines = append(lines, fmt.Sprintf("  %s %s",
		p.styles.muted.Render("Type:"),
		p.styles.itemName.Render(item.Kind)))

	if item.Enabled != nil {
		statusText := "Disabled"
		statusStyle := p.styles.disabled
		if *item.Enabled {
			statusText = "Enabled"
			statusStyle = p.styles.enabled
		}
		lines = append(lines, fmt.Sprintf("  %s %s",
			p.styles.muted.Render("Status:"),
			statusStyle.Render(statusText)))
	}

	// External plugin command info
	if strings.HasPrefix(item.Slug, "external-") {
		for i, ep := range p.cfg.ExternalPlugins {
			if item.Slug == fmt.Sprintf("external-%d", i) {
				lines = append(lines, fmt.Sprintf("  %s %s",
					p.styles.muted.Render("Command:"),
					p.styles.itemName.Render(ep.Command)))
				break
			}
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handlePluginContentKey(item *NavItem, msg tea.KeyMsg) plugin.Action {
	// Delegate to SettingsProvider if available
	if plug, ok := p.registry.BySlug(item.Slug); ok {
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
	return plugin.NoopAction()
}

// --- Data source content ---

func (p *Plugin) viewDatasourceContent(item *NavItem, width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render(strings.ToUpper(item.Label)))
	lines = append(lines, "")

	// Check for a SettingsProvider
	if sp, ok := p.providers[item.Slug]; ok {
		lines = append(lines, sp.SettingsView(width, height))
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	// Default data source info
	lines = append(lines, fmt.Sprintf("  %s %s",
		p.styles.muted.Render("Source:"),
		p.styles.itemName.Render(item.Slug)))

	if item.Enabled != nil {
		statusText := "Disabled"
		statusStyle := p.styles.disabled
		if *item.Enabled {
			statusText = "Enabled"
			statusStyle = p.styles.enabled
		}
		lines = append(lines, fmt.Sprintf("  %s %s",
			p.styles.muted.Render("Status:"),
			statusStyle.Render(statusText)))
	}

	if item.Valid != nil {
		if *item.Valid {
			lines = append(lines, fmt.Sprintf("  %s %s",
				p.styles.muted.Render("Credentials:"),
				p.styles.enabled.Render("Valid")))
		} else {
			lines = append(lines, fmt.Sprintf("  %s %s",
				p.styles.muted.Render("Credentials:"),
				p.styles.logError.Render("Invalid")))
			if item.ValidHint != "" {
				lines = append(lines, fmt.Sprintf("  %s %s",
					p.styles.muted.Render("Hint:"),
					p.styles.logWarn.Render(item.ValidHint)))
			}
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleDatasourceContentKey(item *NavItem, msg tea.KeyMsg) plugin.Action {
	// Delegate to SettingsProvider if available
	if sp, ok := p.providers[item.Slug]; ok {
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
	return plugin.NoopAction()
}

// --- System placeholders ---

func (p *Plugin) viewSystemPlaceholder(title, description string) string {
	var lines []string
	lines = append(lines, p.styles.header.Render(strings.ToUpper(title)))
	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  "+description))
	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  Coming soon"))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
