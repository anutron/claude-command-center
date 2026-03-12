package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// datasourceRecheckResult is a tea.Msg carrying the result of a live re-check.
type datasourceRecheckResult struct {
	Slug   string
	Result plugin.ValidationResult
}

// --- Data source content (sidebar layout) ---

func (p *Plugin) viewDatasourceContent(item *NavItem, width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render(strings.ToUpper(item.Label)))
	if item.Description != "" {
		lines = append(lines, "  "+p.styles.muted.Render(item.Description))
	}
	lines = append(lines, "")

	// Check for a SettingsProvider
	if sp, ok := p.providers[item.Slug]; ok {
		lines = append(lines, sp.SettingsView(width, height))
		// Append validation status + recheck hint below the provider view
		lines = append(lines, "")
		lines = append(lines, p.viewValidationStatus(item))
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

	// Tiered validation display
	lines = append(lines, "")
	lines = append(lines, p.viewValidationStatus(item))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// viewValidationStatus renders the tiered credential validation status and instructions.
func (p *Plugin) viewValidationStatus(item *NavItem) string {
	var lines []string

	greenCheck := lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Render("\u2713")
	yellowWarn := lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68")).Render("\u26a0")
	redX := lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("\u2717")

	switch item.ValidationStatus {
	case "ok":
		lines = append(lines, fmt.Sprintf("  %s %s",
			greenCheck,
			p.styles.enabled.Render("Credentials configured")))
		if item.ValidationMsg != "" {
			lines = append(lines, "  "+p.styles.muted.Render(item.ValidationMsg))
		}

	case "incomplete":
		lines = append(lines, fmt.Sprintf("  %s %s",
			yellowWarn,
			p.styles.logWarn.Render("Credentials incomplete")))
		if item.ValidationMsg != "" {
			lines = append(lines, "  "+p.styles.muted.Render(item.ValidationMsg))
		}
		if item.ValidHint != "" {
			lines = append(lines, "")
			lines = append(lines, "  "+p.styles.logWarn.Render("Fix: ")+p.styles.muted.Render(item.ValidHint))
		}

	case "no_client":
		lines = append(lines, fmt.Sprintf("  %s %s",
			redX,
			p.styles.logError.Render("OAuth client credentials missing")))
		if item.ValidationMsg != "" {
			lines = append(lines, "  "+p.styles.muted.Render(item.ValidationMsg))
		}
		lines = append(lines, "")
		lines = append(lines, "  "+p.styles.muted.Render("To fix:"))
		lines = append(lines, "  "+p.styles.muted.Render("1. Create OAuth credentials in Google Cloud Console"))
		lines = append(lines, "  "+p.styles.muted.Render("2. Add clientId and clientSecret to your credential file"))
		if item.ValidHint != "" {
			lines = append(lines, "  "+p.styles.muted.Render("   "+item.ValidHint))
		}

	case "missing":
		lines = append(lines, fmt.Sprintf("  %s %s",
			redX,
			p.styles.logError.Render("Credentials not found")))
		if item.ValidationMsg != "" {
			lines = append(lines, "  "+p.styles.muted.Render(item.ValidationMsg))
		}
		if item.ValidHint != "" {
			lines = append(lines, "")
			lines = append(lines, "  "+p.styles.muted.Render("Setup: ")+p.styles.muted.Render(item.ValidHint))
		}

	default:
		// Legacy fallback
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
	}

	lines = append(lines, "")
	lines = append(lines, "  "+p.styles.muted.Render("r re-check credentials"))

	return strings.Join(lines, "\n")
}

func (p *Plugin) handleDatasourceContentKey(item *NavItem, msg tea.KeyMsg) plugin.Action {
	// Handle re-check key
	if msg.String() == "r" {
		slug := item.Slug
		cmd := func() tea.Msg {
			result := p.validateDataSourceResult(slug)
			return datasourceRecheckResult{Slug: slug, Result: result}
		}
		p.flashMessage = "Re-checking " + item.Label + " credentials..."
		p.flashMessageAt = time.Now()
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}

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
