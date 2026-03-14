package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// systemActionResult is a tea.Msg returned after an async system action completes.
type systemActionResult struct {
	slug    string
	message string
	err     error
}

// systemFormValues holds the selected action from a system pane form.
type systemFormValues struct {
	Action string
}

// ============================================================
// Schedule
// ============================================================

func (p *Plugin) buildScheduleForm() *huh.Form {
	p.systemValues = &systemFormValues{}

	installed := config.IsScheduleInstalled()
	statusLine := "Not installed"
	statusStyle := p.styles.disabled
	if installed {
		statusLine = "Installed"
		statusStyle = p.styles.enabled
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Description(fmt.Sprintf("%s %s",
					p.styles.muted.Render("Status:"),
					statusStyle.Render(statusLine))),
			huh.NewSelect[string]().
				Title("ACTIONS").
				Options(
					huh.NewOption("Install", "install"),
					huh.NewOption("Uninstall", "uninstall"),
				).
				Value(&p.systemValues.Action),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

func (p *Plugin) handleScheduleFormCompletion() tea.Cmd {
	if p.systemValues == nil {
		return nil
	}
	action := p.systemValues.Action
	p.systemValues = nil

	// Rebuild form immediately so it stays on screen
	form := p.buildScheduleForm()
	p.activeForm = form
	p.activeFormSlug = "system-schedule"
	initCmd := form.Init()

	switch action {
	case "install":
		return tea.Batch(initCmd, func() tea.Msg {
			err := config.InstallSchedule()
			return systemActionResult{slug: "system-schedule", message: "Schedule installed", err: err}
		})
	case "uninstall":
		return tea.Batch(initCmd, func() tea.Msg {
			err := config.UninstallSchedule()
			return systemActionResult{slug: "system-schedule", message: "Schedule uninstalled", err: err}
		})
	}
	return initCmd
}

// ============================================================
// MCP Servers
// ============================================================

func (p *Plugin) buildMCPForm() *huh.Form {
	p.systemValues = &systemFormValues{}

	status := config.IsMCPBuilt()
	var statusLines []string
	for _, name := range []string{"gmail"} {
		built, exists := status[name]
		var indicator string
		if exists && built {
			indicator = p.styles.enabled.Render("built")
		} else {
			indicator = p.styles.disabled.Render("not built")
		}
		statusLines = append(statusLines, fmt.Sprintf("%s %s",
			p.styles.itemName.Render(name+":"),
			indicator))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Description(strings.Join(statusLines, "\n")),
			huh.NewSelect[string]().
				Title("ACTIONS").
				Options(
					huh.NewOption("Build & Configure", "build"),
				).
				Value(&p.systemValues.Action),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

func (p *Plugin) handleMCPFormCompletion() tea.Cmd {
	if p.systemValues == nil {
		return nil
	}
	action := p.systemValues.Action
	p.systemValues = nil

	form := p.buildMCPForm()
	p.activeForm = form
	p.activeFormSlug = "system-mcp"
	initCmd := form.Init()

	if action == "build" {
		return tea.Batch(initCmd, func() tea.Msg {
			servers, err := config.BuildAndConfigureMCP()
			msg := "MCP configured: " + strings.Join(servers, ", ")
			return systemActionResult{slug: "system-mcp", message: msg, err: err}
		})
	}
	return initCmd
}

// ============================================================
// Skills
// ============================================================

func (p *Plugin) buildSkillsForm() *huh.Form {
	p.systemValues = &systemFormValues{}

	var statusLines []string
	names := config.SkillNames()
	if len(names) == 0 {
		statusLines = append(statusLines, p.styles.muted.Render("No skills found in repo"))
	} else {
		for _, name := range names {
			installed := config.IsSkillInstalled(name)
			var indicator string
			if installed {
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Render("\u2713")
			} else {
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("\u2717")
			}
			statusLines = append(statusLines, fmt.Sprintf("%s %s", indicator, p.styles.itemName.Render(name)))
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Description(strings.Join(statusLines, "\n")),
			huh.NewSelect[string]().
				Title("ACTIONS").
				Options(
					huh.NewOption("Install All", "install"),
					huh.NewOption("Uninstall All", "uninstall"),
				).
				Value(&p.systemValues.Action),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

func (p *Plugin) handleSkillsFormCompletion() tea.Cmd {
	if p.systemValues == nil {
		return nil
	}
	action := p.systemValues.Action
	p.systemValues = nil

	form := p.buildSkillsForm()
	p.activeForm = form
	p.activeFormSlug = "system-skills"
	initCmd := form.Init()

	switch action {
	case "install":
		return tea.Batch(initCmd, func() tea.Msg {
			err := config.InstallSkills()
			return systemActionResult{slug: "system-skills", message: "Skills installed", err: err}
		})
	case "uninstall":
		return tea.Batch(initCmd, func() tea.Msg {
			err := config.UninstallSkills()
			return systemActionResult{slug: "system-skills", message: "Skills uninstalled", err: err}
		})
	}
	return initCmd
}

// ============================================================
// Shell Integration
// ============================================================

func (p *Plugin) buildShellForm() *huh.Form {
	p.systemValues = &systemFormValues{}

	installed := config.IsShellHookInstalled()
	statusLine := "Not installed"
	statusStyle := p.styles.disabled
	if installed {
		statusLine = "Installed"
		statusStyle = p.styles.enabled
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Description(fmt.Sprintf("%s %s",
					p.styles.muted.Render("Status:"),
					statusStyle.Render(statusLine))),
			huh.NewSelect[string]().
				Title("ACTIONS").
				Options(
					huh.NewOption("Install", "install"),
					huh.NewOption("Uninstall", "uninstall"),
				).
				Value(&p.systemValues.Action),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

func (p *Plugin) handleShellFormCompletion() tea.Cmd {
	if p.systemValues == nil {
		return nil
	}
	action := p.systemValues.Action
	p.systemValues = nil

	form := p.buildShellForm()
	p.activeForm = form
	p.activeFormSlug = "system-shell"
	initCmd := form.Init()

	switch action {
	case "install":
		return tea.Batch(initCmd, func() tea.Msg {
			err := config.InstallShellHook()
			return systemActionResult{slug: "system-shell", message: "Shell hook installed", err: err}
		})
	case "uninstall":
		return tea.Batch(initCmd, func() tea.Msg {
			err := config.UninstallShellHook()
			return systemActionResult{slug: "system-shell", message: "Shell hook uninstalled", err: err}
		})
	}
	return initCmd
}

// ============================================================
// HandleMessage integration for systemActionResult
// ============================================================

// handleSystemActionResult processes the result of an async system action.
// Returns true if the message was handled.
func (p *Plugin) handleSystemActionResult(msg systemActionResult) (bool, tea.Cmd) {
	if msg.err != nil {
		p.flashMessage = "Error: " + msg.err.Error()
	} else {
		p.flashMessage = msg.message
	}
	p.flashMessageAt = time.Now()
	// Rebuild nav to pick up status changes
	p.rebuildNav()

	// Rebuild the active system form so it reflects the new status
	if p.activeFormSlug == msg.slug {
		var form *huh.Form
		switch msg.slug {
		case "system-schedule":
			form = p.buildScheduleForm()
		case "system-mcp":
			form = p.buildMCPForm()
		case "system-skills":
			form = p.buildSkillsForm()
		case "system-shell":
			form = p.buildShellForm()
		}
		if form != nil {
			p.activeForm = form
			return true, form.Init()
		}
	}

	return true, nil
}
