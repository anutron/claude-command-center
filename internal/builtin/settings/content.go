package settings

import (
	"strings"

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
