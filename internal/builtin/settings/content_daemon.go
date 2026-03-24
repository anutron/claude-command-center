package settings

import (
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// getDaemonState queries the daemon for its current state.
// Returns "stopped" if daemon is unreachable.
func (p *Plugin) getDaemonState() string {
	if p.daemonClientFunc == nil {
		return "stopped"
	}
	client := p.daemonClientFunc()
	if client == nil {
		return "stopped"
	}
	status, err := client.GetDaemonStatus()
	if err != nil {
		return "stopped"
	}
	return status.State
}

// daemonFormValues holds values bound to the daemon huh form fields.
type daemonFormValues struct {
	Action string
}

// buildDaemonForm creates a huh.Form showing daemon status with contextual
// actions: Pause/Stop when running, Resume/Stop when paused, Start when stopped.
func (p *Plugin) buildDaemonForm() *huh.Form {
	state := p.getDaemonState()

	// Status display.
	var statusText string
	switch state {
	case "running":
		statusText = p.styles.enabled.Render("Running")
	case "paused":
		statusText = p.styles.logWarn.Render("Paused")
	default:
		statusText = p.styles.disabled.Render("Stopped")
	}

	// Contextual action options.
	var options []huh.Option[string]
	switch state {
	case "running":
		options = []huh.Option[string]{
			huh.NewOption("Pause", "pause"),
			huh.NewOption("Stop", "stop"),
		}
	case "paused":
		options = []huh.Option[string]{
			huh.NewOption("Resume", "resume"),
			huh.NewOption("Stop", "stop"),
		}
	default: // stopped
		options = []huh.Option[string]{
			huh.NewOption("Start", "start"),
		}
	}

	p.daemonValues = &daemonFormValues{
		Action: options[0].Value,
	}

	// Build live info when daemon is reachable.
	var infoDesc string
	if state != "stopped" && p.daemonClientFunc != nil {
		client := p.daemonClientFunc()
		if client != nil {
			if bs, err := client.GetBudgetStatus(); err == nil {
				infoDesc = fmt.Sprintf(
					"  %s %d\n  %s $%.2f / $%.0f/hr\n  %s $%.2f / $%.0f",
					p.styles.muted.Render("Active agents:"),
					bs.ActiveAgents,
					p.styles.muted.Render("Hourly spend:"),
					bs.HourlySpent, bs.HourlyLimit,
					p.styles.muted.Render("Daily spend:"),
					bs.DailySpent, bs.DailyLimit,
				)
				if bs.EmergencyStopped {
					infoDesc += "\n  " + p.styles.logError.Render("Emergency stop ACTIVE")
				}
			}
		}
	}

	groups := []*huh.Group{
		huh.NewGroup(
			huh.NewNote().
				Title("Status").
				Description("  "+statusText),
			huh.NewSelect[string]().
				Title("Action").
				Options(options...).
				Value(&p.daemonValues.Action),
		),
	}

	if infoDesc != "" {
		groups = append(groups, huh.NewGroup(
			huh.NewNote().
				Title("Live Info").
				Description(infoDesc),
		))
	}

	form := huh.NewForm(groups...).
		WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

// saveDaemonValues applies the selected daemon action on field transition.
func (p *Plugin) saveDaemonValues() {
	if p.daemonValues == nil {
		return
	}
	if msg := p.applyDaemonAction(p.daemonValues.Action); msg != "" {
		p.flashMessage = msg
		p.flashMessageAt = time.Now()
	}
}

// handleDaemonFormCompletion applies the action and rebuilds the form.
func (p *Plugin) handleDaemonFormCompletion() tea.Cmd {
	if p.daemonValues == nil {
		return nil
	}

	action := p.daemonValues.Action
	p.daemonValues = nil

	if msg := p.applyDaemonAction(action); msg != "" {
		p.flashMessage = msg
		p.flashMessageAt = time.Now()
	}

	// Brief pause so the daemon has time to start/stop before we rebuild.
	time.Sleep(300 * time.Millisecond)

	form := p.buildDaemonForm()
	p.activeForm = form
	p.activeFormSlug = "agent-daemon"
	return form.Init()
}

// applyDaemonAction executes a daemon lifecycle action.
func (p *Plugin) applyDaemonAction(action string) string {
	switch action {
	case "start":
		if err := daemon.StartProcess(); err != nil {
			return "Failed to start: " + err.Error()
		}
		return "Daemon started"
	case "stop":
		if p.daemonClientFunc != nil {
			if client := p.daemonClientFunc(); client != nil {
				if err := client.ShutdownDaemon(); err != nil {
					return "Failed to stop: " + err.Error()
				}
				return "Daemon shutting down"
			}
		}
		return "Daemon not connected"
	case "pause":
		if p.daemonClientFunc != nil {
			if client := p.daemonClientFunc(); client != nil {
				if err := client.PauseDaemon(); err != nil {
					return "Failed to pause: " + err.Error()
				}
				return "Daemon paused"
			}
		}
		return "Daemon not connected"
	case "resume":
		if p.daemonClientFunc != nil {
			if client := p.daemonClientFunc(); client != nil {
				if err := client.ResumeDaemon(); err != nil {
					return "Failed to resume: " + err.Error()
				}
				return "Daemon resumed"
			}
		}
		return "Daemon not connected"
	}
	return ""
}
