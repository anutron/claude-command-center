package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// systemActionResult is a tea.Msg returned after an async system action completes.
type systemActionResult struct {
	slug    string
	message string
	err     error
}

// --- Per-pane cursor state ---
// Stored on Plugin to persist across renders.
// systemCursor tracks cursor position within system content panes.

// systemCursorFor returns the current cursor for a system slug.
func (p *Plugin) systemCursorFor(slug string) int {
	if p.systemCursors == nil {
		return 0
	}
	return p.systemCursors[slug]
}

// setSystemCursor sets the cursor for a system slug.
func (p *Plugin) setSystemCursor(slug string, v int) {
	if p.systemCursors == nil {
		p.systemCursors = make(map[string]int)
	}
	p.systemCursors[slug] = v
}

// ============================================================
// Schedule
// ============================================================

func (p *Plugin) viewScheduleContent(width, height int) string {
	item := p.selectedNavItem()
	desc := ""
	if item != nil {
		desc = item.Description
	}
	lines := p.renderPaneHeader("REFRESH SCHEDULE", desc)

	installed := config.IsScheduleInstalled()
	if installed {
		lines = append(lines, fmt.Sprintf("  %s %s",
			p.styles.muted.Render("Status:"),
			p.styles.enabled.Render("Installed")))
	} else {
		lines = append(lines, fmt.Sprintf("  %s %s",
			p.styles.muted.Render("Status:"),
			p.styles.disabled.Render("Not installed")))
	}
	lines = append(lines, "")

	actions := []string{"Install", "Uninstall"}
	cursor := p.systemCursorFor("system-schedule")
	for i, a := range actions {
		ptr := "  "
		if i == cursor {
			ptr = p.styles.pointer.Render("> ")
		}
		lines = append(lines, fmt.Sprintf("%s%s", ptr, p.styles.itemName.Render(a)))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  up/down navigate  enter run  esc back"))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleScheduleContentKey(msg tea.KeyMsg) plugin.Action {
	cursor := p.systemCursorFor("system-schedule")
	switch msg.String() {
	case "up", "k":
		if cursor > 0 {
			p.setSystemCursor("system-schedule", cursor-1)
		}
	case "down", "j":
		if cursor < 1 {
			p.setSystemCursor("system-schedule", cursor+1)
		}
	case "enter":
		if cursor == 0 {
			return plugin.Action{
				Type: plugin.ActionNoop,
				TeaCmd: func() tea.Msg {
					err := config.InstallSchedule()
					return systemActionResult{slug: "system-schedule", message: "Schedule installed", err: err}
				},
			}
		}
		return plugin.Action{
			Type: plugin.ActionNoop,
			TeaCmd: func() tea.Msg {
				err := config.UninstallSchedule()
				return systemActionResult{slug: "system-schedule", message: "Schedule uninstalled", err: err}
			},
		}
	}
	return plugin.NoopAction()
}

// ============================================================
// MCP Servers
// ============================================================

func (p *Plugin) viewMCPContent(width, height int) string {
	item := p.selectedNavItem()
	desc := ""
	if item != nil {
		desc = item.Description
	}
	lines := p.renderPaneHeader("MCP SERVERS", desc)

	status := config.IsMCPBuilt()
	for _, name := range []string{"gmail"} {
		built, exists := status[name]
		var indicator string
		if exists && built {
			indicator = p.styles.enabled.Render("built")
		} else {
			indicator = p.styles.disabled.Render("not built")
		}
		lines = append(lines, fmt.Sprintf("  %s %s",
			p.styles.itemName.Render(name+":"),
			indicator))
	}
	lines = append(lines, "")

	actions := []string{"Build & Configure"}
	cursor := p.systemCursorFor("system-mcp")
	for i, a := range actions {
		ptr := "  "
		if i == cursor {
			ptr = p.styles.pointer.Render("> ")
		}
		lines = append(lines, fmt.Sprintf("%s%s", ptr, p.styles.itemName.Render(a)))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  enter run  esc back"))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleMCPContentKey(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "enter":
		return plugin.Action{
			Type: plugin.ActionNoop,
			TeaCmd: func() tea.Msg {
				servers, err := config.BuildAndConfigureMCP()
				msg := "MCP configured: " + strings.Join(servers, ", ")
				return systemActionResult{slug: "system-mcp", message: msg, err: err}
			},
		}
	}
	return plugin.NoopAction()
}

// ============================================================
// Skills
// ============================================================

func (p *Plugin) viewSkillsContent(width, height int) string {
	item := p.selectedNavItem()
	desc := ""
	if item != nil {
		desc = item.Description
	}
	lines := p.renderPaneHeader("SKILLS", desc)

	names := config.SkillNames()
	if len(names) == 0 {
		lines = append(lines, p.styles.muted.Render("  No skills found in repo"))
	} else {
		for _, name := range names {
			installed := config.IsSkillInstalled(name)
			var indicator string
			if installed {
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Render("\u2713")
			} else {
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("\u2717")
			}
			lines = append(lines, fmt.Sprintf("  %s %s", indicator, p.styles.itemName.Render(name)))
		}
	}
	lines = append(lines, "")

	actions := []string{"Install All", "Uninstall All"}
	cursor := p.systemCursorFor("system-skills")
	for i, a := range actions {
		ptr := "  "
		if i == cursor {
			ptr = p.styles.pointer.Render("> ")
		}
		lines = append(lines, fmt.Sprintf("%s%s", ptr, p.styles.itemName.Render(a)))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  up/down navigate  enter run  esc back"))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleSkillsContentKey(msg tea.KeyMsg) plugin.Action {
	cursor := p.systemCursorFor("system-skills")
	switch msg.String() {
	case "up", "k":
		if cursor > 0 {
			p.setSystemCursor("system-skills", cursor-1)
		}
	case "down", "j":
		if cursor < 1 {
			p.setSystemCursor("system-skills", cursor+1)
		}
	case "enter":
		if cursor == 0 {
			return plugin.Action{
				Type: plugin.ActionNoop,
				TeaCmd: func() tea.Msg {
					err := config.InstallSkills()
					return systemActionResult{slug: "system-skills", message: "Skills installed", err: err}
				},
			}
		}
		return plugin.Action{
			Type: plugin.ActionNoop,
			TeaCmd: func() tea.Msg {
				err := config.UninstallSkills()
				return systemActionResult{slug: "system-skills", message: "Skills uninstalled", err: err}
			},
		}
	}
	return plugin.NoopAction()
}

// ============================================================
// Shell Integration
// ============================================================

func (p *Plugin) viewShellContent(width, height int) string {
	item := p.selectedNavItem()
	desc := ""
	if item != nil {
		desc = item.Description
	}
	lines := p.renderPaneHeader("SHELL INTEGRATION", desc)

	installed := config.IsShellHookInstalled()
	if installed {
		lines = append(lines, fmt.Sprintf("  %s %s",
			p.styles.muted.Render("Status:"),
			p.styles.enabled.Render("Installed")))
	} else {
		lines = append(lines, fmt.Sprintf("  %s %s",
			p.styles.muted.Render("Status:"),
			p.styles.disabled.Render("Not installed")))
	}
	lines = append(lines, "")

	actions := []string{"Install", "Uninstall"}
	cursor := p.systemCursorFor("system-shell")
	for i, a := range actions {
		ptr := "  "
		if i == cursor {
			ptr = p.styles.pointer.Render("> ")
		}
		lines = append(lines, fmt.Sprintf("%s%s", ptr, p.styles.itemName.Render(a)))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  up/down navigate  enter run  esc back"))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleShellContentKey(msg tea.KeyMsg) plugin.Action {
	cursor := p.systemCursorFor("system-shell")
	switch msg.String() {
	case "up", "k":
		if cursor > 0 {
			p.setSystemCursor("system-shell", cursor-1)
		}
	case "down", "j":
		if cursor < 1 {
			p.setSystemCursor("system-shell", cursor+1)
		}
	case "enter":
		if cursor == 0 {
			return plugin.Action{
				Type: plugin.ActionNoop,
				TeaCmd: func() tea.Msg {
					err := config.InstallShellHook()
					return systemActionResult{slug: "system-shell", message: "Shell hook installed", err: err}
				},
			}
		}
		return plugin.Action{
			Type: plugin.ActionNoop,
			TeaCmd: func() tea.Msg {
				err := config.UninstallShellHook()
				return systemActionResult{slug: "system-shell", message: "Shell hook uninstalled", err: err}
			},
		}
	}
	return plugin.NoopAction()
}

// ============================================================
// HandleMessage integration for systemActionResult
// ============================================================

// handleSystemActionResult processes the result of an async system action.
// Returns true if the message was handled.
func (p *Plugin) handleSystemActionResult(msg systemActionResult) bool {
	if msg.err != nil {
		p.flashMessage = "Error: " + msg.err.Error()
	} else {
		p.flashMessage = msg.message
	}
	p.flashMessageAt = time.Now()
	// Rebuild nav to pick up status changes
	p.rebuildNav()
	return true
}
