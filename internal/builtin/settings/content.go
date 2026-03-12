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
	if item == nil {
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

