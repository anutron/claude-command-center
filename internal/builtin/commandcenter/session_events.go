package commandcenter

import (
	"fmt"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/charmbracelet/lipgloss"
)

// sessionEvent is a type alias for agent.SessionEvent within the CC package.
type sessionEvent = agent.SessionEvent

// parseSessionEvent delegates to the agent package.
func parseSessionEvent(raw map[string]interface{}) []sessionEvent {
	return agent.ParseSessionEvent(raw)
}

// agentEventMsg carries a single parsed event from the agent's stdout.
type agentEventMsg struct {
	todoID string
	event  sessionEvent
}

// agentEventsDoneMsg signals that the event channel for a session has closed.
type agentEventsDoneMsg struct {
	todoID string
}

// renderEventLine renders a single session event as a styled line for the viewer.
// wrapWidth is the available character width for text wrapping (0 = no wrap).
func renderEventLine(ev sessionEvent, styles *ccStyles, wrapWidth int) string {
	// labelWidth accounts for "icon label  " prefix (~14 chars).
	const labelWidth = 14
	textWidth := wrapWidth - labelWidth
	if textWidth < 20 {
		textWidth = 20
	}

	switch ev.Type {
	case "assistant_text":
		icon := lipgloss.NewStyle().Foreground(styles.ColorCyan).Bold(true).Render("\u25c6")
		label := lipgloss.NewStyle().Foreground(styles.ColorCyan).Bold(true).Render("Assistant")
		text := wrapText(ev.Text, textWidth)
		return fmt.Sprintf("%s %s  %s", icon, label, text)

	case "tool_use":
		icon := lipgloss.NewStyle().Foreground(styles.ColorYellow).Render("\u25b8")
		label := lipgloss.NewStyle().Foreground(styles.ColorYellow).Bold(true).Render("Tool: " + ev.ToolName)
		return fmt.Sprintf("%s %s", icon, label)

	case "tool_result":
		status := "success"
		color := styles.ColorGreen
		if ev.IsError {
			status = "error"
			color = lipgloss.Color("#FF5555")
		}
		icon := lipgloss.NewStyle().Foreground(color).Render("\u25c2")
		label := lipgloss.NewStyle().Foreground(color).Render("Result (" + status + ")")
		return fmt.Sprintf("%s %s", icon, label)

	case "error":
		icon := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render("\u26a0")
		label := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render("BLOCKED:")
		text := wrapText(ev.Text, textWidth)
		return fmt.Sprintf("%s %s %s", icon, label, text)

	case "user":
		icon := lipgloss.NewStyle().Foreground(styles.ColorPurple).Render("\u25b7")
		label := lipgloss.NewStyle().Foreground(styles.ColorPurple).Bold(true).Render("You")
		text := wrapText(ev.Text, textWidth)
		return fmt.Sprintf("%s %s  %s", icon, label, text)

	case "system":
		icon := lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("\u25cf")
		label := lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("System")
		text := wrapText(ev.Text, textWidth)
		return fmt.Sprintf("%s %s  %s", icon, label, text)

	default:
		return ""
	}
}

