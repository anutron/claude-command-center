package settings

import (
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// viewContent renders the content pane based on the currently selected nav item.
func (p *Plugin) viewContent(width, height int) string {
	item := p.selectedNavItem()

	var body string
	if p.focusZone == FocusForm && p.activeForm != nil {
		// Render the huh form above the normal content
		formView := p.activeForm.View()
		body = formView
		if item != nil {
			// Show a condensed version of the datasource content below the form
			body = formView + "\n\n" + p.renderContentForSlug(item, width, height)
		}
	} else if item == nil {
		body = p.styles.muted.Render("  Select an item from the sidebar")
	} else {
		body = p.renderContentForSlug(item, width, height)
	}

	// Pick panel style based on focus zone.
	var panelStyle lipgloss.Style
	if p.focusZone == FocusContent || p.focusZone == FocusEditing || p.focusZone == FocusForm {
		panelStyle = p.styles.contentFocused
	} else {
		panelStyle = p.styles.contentUnfocused
	}

	return panelStyle.Width(width).Height(height).Render(body)
}

// renderPaneHeader renders a styled header title with an optional dimmed description line below it.
func (p *Plugin) renderPaneHeader(title, description string) []string {
	lines := []string{p.styles.header.Render(title)}
	if description != "" {
		lines = append(lines, "  "+p.styles.muted.Render(description))
	}
	lines = append(lines, "")
	return lines
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
		return p.viewScheduleContent(width, height)
	case "system-mcp":
		return p.viewMCPContent(width, height)
	case "system-skills":
		return p.viewSkillsContent(width, height)
	case "system-shell":
		return p.viewShellContent(width, height)
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
	// But first, if the active provider is in editing mode, let it handle esc
	// so that pressing Escape cancels the edit instead of jumping to nav.
	if p.focusZone == FocusContent {
		switch msg.String() {
		case "esc", "left", "h":
			// Give the active SettingsProvider a chance to handle esc first
			// (e.g. to cancel an inline text edit like a GitHub repo input).
			if sp := p.activeProvider(); sp != nil {
				action := sp.HandleSettingsKey(msg)
				if action.Type == plugin.ActionFlash {
					p.flashMessage = action.Payload
					p.flashMessageAt = currentTime()
					return plugin.NoopAction()
				}
				if action.Type != plugin.ActionUnhandled {
					return action
				}
			}
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
	case "system-schedule":
		return p.handleScheduleContentKey(msg)
	case "system-mcp":
		return p.handleMCPContentKey(msg)
	case "system-skills":
		return p.handleSkillsContentKey(msg)
	case "system-shell":
		return p.handleShellContentKey(msg)
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

