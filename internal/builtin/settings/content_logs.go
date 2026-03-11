package settings

import (
	"fmt"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Logs content (sidebar layout) ---

func (p *Plugin) viewLogsContent(width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render("LOGS"))
	lines = append(lines, "")

	if p.logger == nil {
		lines = append(lines, p.styles.muted.Render("  No logger available"))
	} else {
		entries := p.logger.Recent(100)
		if len(entries) == 0 {
			lines = append(lines, p.styles.muted.Render("  No log entries"))
		} else {
			maxVisible := height - 8
			if maxVisible < 5 {
				maxVisible = 5
			}

			if p.logOffset > len(entries)-maxVisible {
				p.logOffset = len(entries) - maxVisible
			}
			if p.logOffset < 0 {
				p.logOffset = 0
			}

			start := len(entries) - 1 - p.logOffset
			end := start - maxVisible
			if end < 0 {
				end = -1
			}

			for i := start; i > end; i-- {
				e := entries[i]
				timeStr := e.Time.Format("15:04:05")
				var levelStyle lipgloss.Style
				switch e.Level {
				case "ERROR":
					levelStyle = p.styles.logError
				case "WARN":
					levelStyle = p.styles.logWarn
				default:
					levelStyle = p.styles.muted
				}
				line := fmt.Sprintf("  %s %s %s %s",
					p.styles.muted.Render(timeStr),
					levelStyle.Render(fmt.Sprintf("%-5s", e.Level)),
					p.styles.logPlugin.Render(fmt.Sprintf("%-15s", e.Plugin)),
					e.Message,
				)
				lines = append(lines, line)
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  up/down scroll  esc back"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleLogsContentKey(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "up", "k":
		if p.logOffset > 0 {
			p.logOffset--
		}
	case "down", "j":
		p.logOffset++
	}
	return plugin.NoopAction()
}
