package settings

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// --- Budget form values ---

// budgetFormValues holds values bound to the budget huh form fields.
type budgetFormValues struct {
	MaxConcurrent string
	HourlyBudget  string
	DailyBudget   string
	WarningPct    string
}

// buildAgentBudgetForm creates a huh.Form for editing agent budget configuration
// with a read-only live spend section from the daemon.
func (p *Plugin) buildAgentBudgetForm() *huh.Form {
	p.budgetValues = &budgetFormValues{
		MaxConcurrent: fmt.Sprintf("%d", p.cfg.Agent.MaxConcurrent),
		HourlyBudget:  fmt.Sprintf("%.2f", p.cfg.Agent.HourlyBudget),
		DailyBudget:   fmt.Sprintf("%.2f", p.cfg.Agent.DailyBudget),
		WarningPct:    fmt.Sprintf("%.0f", p.cfg.Agent.BudgetWarningPct*100),
	}

	// --- Live Spend (from daemon, read-only) ---
	hourlyLimit := fmt.Sprintf("$%.2f", p.cfg.Agent.HourlyBudget)
	dailyLimit := fmt.Sprintf("$%.2f", p.cfg.Agent.DailyBudget)
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

	// --- Rate Limits (read-only) ---
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
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Max Concurrent Agents").
				CharLimit(4).
				Value(&p.budgetValues.MaxConcurrent).
				Validate(func(s string) error {
					v, err := strconv.Atoi(s)
					if err != nil {
						return errors.New("must be a number")
					}
					if v < 1 || v > 100 {
						return errors.New("must be 1-100")
					}
					return nil
				}),
			huh.NewInput().
				Title("Hourly Budget ($)").
				CharLimit(8).
				Value(&p.budgetValues.HourlyBudget).
				Validate(func(s string) error {
					v, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return errors.New("must be a number")
					}
					if v < 0 {
						return errors.New("cannot be negative")
					}
					return nil
				}),
			huh.NewInput().
				Title("Daily Budget ($)").
				CharLimit(8).
				Value(&p.budgetValues.DailyBudget).
				Validate(func(s string) error {
					v, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return errors.New("must be a number")
					}
					if v < 0 {
						return errors.New("cannot be negative")
					}
					return nil
				}),
			huh.NewInput().
				Title("Warning Threshold (%)").
				CharLimit(3).
				Value(&p.budgetValues.WarningPct).
				Validate(func(s string) error {
					v, err := strconv.Atoi(s)
					if err != nil {
						return errors.New("must be a number")
					}
					if v < 0 || v > 100 {
						return errors.New("must be 0-100")
					}
					return nil
				}),
		),
		huh.NewGroup(
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
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

// saveBudgetValues persists the current budget form values to config without
// rebuilding the form. Used for incremental auto-save on field transitions.
func (p *Plugin) saveBudgetValues() {
	if p.budgetValues == nil {
		return
	}

	changed := false

	if v, err := strconv.Atoi(p.budgetValues.MaxConcurrent); err == nil && v != p.cfg.Agent.MaxConcurrent {
		p.cfg.Agent.MaxConcurrent = v
		changed = true
	}
	if v, err := strconv.ParseFloat(p.budgetValues.HourlyBudget, 64); err == nil && v != p.cfg.Agent.HourlyBudget {
		p.cfg.Agent.HourlyBudget = v
		changed = true
	}
	if v, err := strconv.ParseFloat(p.budgetValues.DailyBudget, 64); err == nil && v != p.cfg.Agent.DailyBudget {
		p.cfg.Agent.DailyBudget = v
		changed = true
	}
	if v, err := strconv.Atoi(p.budgetValues.WarningPct); err == nil {
		pct := float64(v) / 100.0
		if pct != p.cfg.Agent.BudgetWarningPct {
			p.cfg.Agent.BudgetWarningPct = pct
			changed = true
		}
	}

	if !changed {
		return
	}

	if err := config.Save(p.cfg, true); err == nil {
		p.flashMessage = "Budget saved"
		p.publishConfigSaved("agent-budget")
	} else {
		p.flashMessage = "Failed to save: " + err.Error()
	}
	p.flashMessageAt = time.Now()
}

// handleBudgetFormCompletion saves budget form values and rebuilds the form.
func (p *Plugin) handleBudgetFormCompletion() tea.Cmd {
	if p.budgetValues == nil {
		return nil
	}

	vals := p.budgetValues
	p.budgetValues = nil

	if v, err := strconv.Atoi(vals.MaxConcurrent); err == nil {
		p.cfg.Agent.MaxConcurrent = v
	}
	if v, err := strconv.ParseFloat(vals.HourlyBudget, 64); err == nil {
		p.cfg.Agent.HourlyBudget = v
	}
	if v, err := strconv.ParseFloat(vals.DailyBudget, 64); err == nil {
		p.cfg.Agent.DailyBudget = v
	}
	if v, err := strconv.Atoi(vals.WarningPct); err == nil {
		p.cfg.Agent.BudgetWarningPct = float64(v) / 100.0
	}

	if err := config.Save(p.cfg, true); err == nil {
		p.flashMessage = "Budget saved"
		p.publishConfigSaved("agent-budget")
	} else {
		p.flashMessage = "Failed to save: " + err.Error()
	}
	p.flashMessageAt = time.Now()

	// Rebuild the form so it stays on screen with updated values
	form := p.buildAgentBudgetForm()
	p.activeForm = form
	p.activeFormSlug = "agent-budget"

	return form.Init()
}
