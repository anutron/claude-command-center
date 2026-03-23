package settings

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// buildAgentSandboxForm creates a read-only huh.Form displaying the current
// agent sandbox configuration values.
func (p *Plugin) buildAgentSandboxForm() *huh.Form {
	// Toggle: todo_write_learned_paths
	writePathsEnabled := p.cfg.Agent.TodoWriteLearnedPathsEnabled()
	var writePathsStatus string
	if writePathsEnabled {
		writePathsStatus = p.styles.enabled.Render("on")
	} else {
		writePathsStatus = p.styles.disabled.Render("off")
	}

	// List: todo_extra_write_paths
	var extraPaths string
	if len(p.cfg.Agent.TodoExtraWritePaths) == 0 {
		extraPaths = p.styles.muted.Render("none configured")
	} else {
		var pathLines []string
		for _, path := range p.cfg.Agent.TodoExtraWritePaths {
			pathLines = append(pathLines, fmt.Sprintf("  • %s", path))
		}
		extraPaths = strings.Join(pathLines, "\n")
	}

	// List: autonomous_allowed_domains
	var domains string
	if len(p.cfg.Agent.AutonomousAllowedDomains) == 0 {
		domains = p.styles.muted.Render("none configured")
	} else {
		var domainLines []string
		for _, d := range p.cfg.Agent.AutonomousAllowedDomains {
			domainLines = append(domainLines, fmt.Sprintf("  • %s", d))
		}
		domains = strings.Join(domainLines, "\n")
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Write Learned Paths").
				Description(fmt.Sprintf(
					"%s %s\n%s",
					p.styles.muted.Render("Allow todo agents to write to all session paths:"),
					writePathsStatus,
					p.styles.muted.Render("When enabled, agents can write to paths discovered during sessions."),
				)),
			huh.NewNote().
				Title("Additional Write Paths").
				Description(fmt.Sprintf(
					"%s\n%s",
					p.styles.muted.Render("Extra paths agents are allowed to write to:"),
					extraPaths,
				)),
			huh.NewNote().
				Title("Autonomous Allowed Domains").
				Description(fmt.Sprintf(
					"%s\n%s",
					p.styles.muted.Render("Domains agents can access in autonomous mode:"),
					domains,
				)),
		),
	).WithShowHelp(false).WithShowErrors(false).WithTheme(p.styles.huhTheme)

	return form
}
