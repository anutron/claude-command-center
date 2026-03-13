package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Plugin content (sidebar layout) ---

func (p *Plugin) viewPluginContent(item *NavItem, width, height int) string {
	lines := p.renderPaneHeader(strings.ToUpper(item.Label), item.Description)

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
