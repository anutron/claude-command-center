package settings

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

// buildAgentBudgetForm creates a read-only huh.Form displaying the current
// budget configuration and live spend status from the daemon.
func (p *Plugin) buildAgentBudgetForm() *huh.Form {
	// --- Budget Caps ---
	hourlyLimit := fmt.Sprintf("$%.2f", p.cfg.Agent.HourlyBudget)
	dailyLimit := fmt.Sprintf("$%.2f", p.cfg.Agent.DailyBudget)
	warningPct := fmt.Sprintf("%.0f%%", p.cfg.Agent.BudgetWarningPct*100)

	// --- Live Spend (from daemon) ---
	var hourlySpend, dailySpend, activeAgents, emergencyStatus string

	if p.daemonClientFunc != nil {
		client := p.daemonClientFunc()
		if client != nil {
			status, err := client.GetBudgetStatus()
			if err == nil {
				hourlySpend = fmt.Sprintf("$%.2f / %s", status.HourlySpent, hourlyLimit)
				dailySpend = fmt.Sprintf("$%.2f / %s", status.DailySpent, dailyLimit)
				activeAgents = fmt.Sprintf("%d", status.ActiveAgents)
				if status.EmergencyStopped {
					emergencyStatus = p.styles.logError.Render("ACTIVE — all agents stopped")
				} else {
					emergencyStatus = p.styles.enabled.Render("off")
				}
			} else {
				hourlySpend = p.styles.muted.Render("unable to query daemon")
				dailySpend = hourlySpend
				activeAgents = p.styles.muted.Render("unknown")
				emergencyStatus = p.styles.muted.Render("unknown")
			}
		} else {
			hourlySpend = p.styles.muted.Render("daemon not connected")
			dailySpend = hourlySpend
			activeAgents = p.styles.muted.Render("N/A")
			emergencyStatus = p.styles.muted.Render("N/A")
		}
	} else {
		hourlySpend = p.styles.muted.Render("daemon not available")
		dailySpend = hourlySpend
		activeAgents = p.styles.muted.Render("N/A")
		emergencyStatus = p.styles.muted.Render("N/A")
	}

	// --- Rate Limits ---
	maxPerHour := fmt.Sprintf("%d", p.cfg.Agent.MaxLaunchesPerAutomationPerHour)
	cooldown := fmt.Sprintf("%d minutes", p.cfg.Agent.CooldownMinutes)
	backoffBase := fmt.Sprintf("%d seconds", p.cfg.Agent.FailureBackoffBaseSec)
	backoffMax := fmt.Sprintf("%d seconds", p.cfg.Agent.FailureBackoffMaxSec)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Current Spend").
				Description(fmt.Sprintf(
					"%s\n  %s %s\n  %s %s\n  %s %s",
					p.styles.muted.Render("Live budget usage from daemon:"),
					p.styles.muted.Render("Hourly:"),
					hourlySpend,
					p.styles.muted.Render("Daily:"),
					dailySpend,
					p.styles.muted.Render("Active agents:"),
					activeAgents,
				)),
			huh.NewNote().
				Title("Emergency Stop").
				Description(fmt.Sprintf(
					"  %s %s",
					p.styles.muted.Render("Status:"),
					emergencyStatus,
				)),
			huh.NewNote().
				Title("Budget Caps").
				Description(fmt.Sprintf(
					"%s\n  %s %s\n  %s %s\n  %s %s",
					p.styles.muted.Render("Configured spending limits:"),
					p.styles.muted.Render("Hourly limit:"),
					hourlyLimit,
					p.styles.muted.Render("Daily limit:"),
					dailyLimit,
					p.styles.muted.Render("Warning at:"),
					warningPct,
				)),
			huh.NewNote().
				Title("Rate Limits").
				Description(fmt.Sprintf(
					"%s\n  %s %s\n  %s %s\n  %s %s\n  %s %s",
					p.styles.muted.Render("Agent launch throttling:"),
					p.styles.muted.Render("Max launches/automation/hr:"),
					maxPerHour,
					p.styles.muted.Render("Budget cooldown:"),
					cooldown,
					p.styles.muted.Render("Failure backoff (initial):"),
					backoffBase,
					p.styles.muted.Render("Failure backoff (max):"),
					backoffMax,
				)),
		),
	).WithShowHelp(false).WithShowErrors(false).WithTheme(p.styles.huhTheme)

	return form
}
