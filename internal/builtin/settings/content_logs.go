package settings

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Logs content (sidebar layout) ---

// filteredLogEntries returns log entries matching the current filter.
// If no filter is active, returns all entries.
func (p *Plugin) filteredLogEntries() []plugin.LogEntry {
	if p.logger == nil {
		return nil
	}
	entries := p.logger.Recent(100)
	filter := strings.TrimSpace(p.logFilterInput.Value())
	if filter == "" {
		return entries
	}
	lowerFilter := strings.ToLower(filter)
	var filtered []plugin.LogEntry
	for _, e := range entries {
		text := strings.ToLower(e.Level + " " + e.Plugin + " " + e.Message)
		if strings.Contains(text, lowerFilter) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func (p *Plugin) viewLogsContent(width, height int) string {
	item := p.selectedNavItem()
	desc := ""
	if item != nil {
		desc = item.Description
	}
	lines := p.renderPaneHeader("LOGS", desc)

	// Show filter input when active, or filter value when set
	if p.logFilterMode {
		lines = append(lines, "  "+p.logFilterInput.View())
		lines = append(lines, "")
	} else if strings.TrimSpace(p.logFilterInput.Value()) != "" {
		lines = append(lines, p.styles.muted.Render(fmt.Sprintf("  filter: %s  (/ to edit, esc to clear)", p.logFilterInput.Value())))
		lines = append(lines, "")
	}

	if p.logger == nil {
		lines = append(lines, p.styles.muted.Render("  No logger available"))
	} else {
		entries := p.filteredLogEntries()
		if len(entries) == 0 {
			if strings.TrimSpace(p.logFilterInput.Value()) != "" {
				lines = append(lines, p.styles.muted.Render("  No matching log entries"))
			} else {
				lines = append(lines, p.styles.muted.Render("  No log entries"))
			}
		} else {
			// Account for header, filter line, hint line, and padding
			extraLines := 4
			if p.logFilterMode || strings.TrimSpace(p.logFilterInput.Value()) != "" {
				extraLines += 2 // filter line + blank line
			}
			maxVisible := height - extraLines - 4
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

			// Scroll position indicator
			if len(entries) > maxVisible {
				topEntry := len(entries) - p.logOffset
				bottomEntry := topEntry - maxVisible + 1
				if bottomEntry < 1 {
					bottomEntry = 1
				}
				posInfo := fmt.Sprintf("  showing %d-%d of %d", bottomEntry, topEntry, len(entries))
				lines = append(lines, "")
				lines = append(lines, p.styles.muted.Render(posInfo))
			}
		}
	}

	lines = append(lines, "")
	if p.logFilterMode {
		lines = append(lines, p.styles.muted.Render("  enter apply  esc cancel"))
	} else {
		lines = append(lines, p.styles.muted.Render("  j/k scroll  ctrl+f/b page  ctrl+d/u half-page  / filter  esc back"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) logsMaxVisible() int {
	panelHeight := p.height - 4
	if panelHeight < 10 {
		panelHeight = 10
	}
	maxVisible := panelHeight - 8
	if maxVisible < 5 {
		maxVisible = 5
	}
	return maxVisible
}

func (p *Plugin) handleLogsContentKey(msg tea.KeyMsg) plugin.Action {
	// When filter input is focused, route keys there
	if p.logFilterMode {
		switch msg.Type {
		case tea.KeyEnter:
			// Apply filter and exit filter mode
			p.logFilterMode = false
			p.logFilterInput.Blur()
			p.logOffset = 0 // reset scroll when filter changes
			return plugin.NoopAction()
		case tea.KeyEsc:
			// Cancel filter: clear the filter text and exit filter mode
			p.logFilterMode = false
			p.logFilterInput.SetValue("")
			p.logFilterInput.Blur()
			p.logOffset = 0
			return plugin.NoopAction()
		default:
			p.logFilterInput, _ = p.logFilterInput.Update(msg)
			return plugin.NoopAction()
		}
	}

	switch msg.String() {
	case "up", "k":
		if p.logOffset > 0 {
			p.logOffset--
		}
	case "down", "j":
		p.logOffset++
	case "ctrl+f":
		p.logOffset += p.logsMaxVisible()
	case "ctrl+b":
		p.logOffset -= p.logsMaxVisible()
		if p.logOffset < 0 {
			p.logOffset = 0
		}
	case "ctrl+d":
		p.logOffset += p.logsMaxVisible() / 2
	case "ctrl+u":
		p.logOffset -= p.logsMaxVisible() / 2
		if p.logOffset < 0 {
			p.logOffset = 0
		}
	case "/":
		p.logFilterMode = true
		p.logFilterInput.Focus()
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}
