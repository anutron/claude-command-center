package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Data source content (sidebar layout) ---

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
