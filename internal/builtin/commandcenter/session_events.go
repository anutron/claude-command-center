package commandcenter

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// sessionEvent represents a parsed event from the Claude CLI stream-json output.
type sessionEvent struct {
	Timestamp    string // raw timestamp from the event, if present
	Type         string // assistant_text, tool_use, tool_result, error, user, system
	Text         string // text content for assistant_text and error types
	ToolName     string // tool name for tool_use events
	ToolInput    string // truncated tool input for tool_use events
	ToolID       string // tool_use id for correlating with results
	ResultToolID string // tool_use_id from tool_result events
	ResultText   string // content from tool_result events
	IsError      bool   // true if tool_result is an error or event is an error type
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

// parseSessionEvent maps a raw stream-json event (already unmarshaled) to a sessionEvent.
func parseSessionEvent(raw map[string]interface{}) sessionEvent {
	eventType, _ := raw["type"].(string)

	var ev sessionEvent

	switch eventType {
	case "assistant":
		content, ok := raw["content"].([]interface{})
		if !ok {
			return ev
		}
		for _, block := range content {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "text":
				text, _ := blockMap["text"].(string)
				ev.Type = "assistant_text"
				ev.Text = text
			case "tool_use":
				ev.Type = "tool_use"
				ev.ToolName, _ = blockMap["name"].(string)
				ev.ToolID, _ = blockMap["id"].(string)
				if input, ok := blockMap["input"].(map[string]interface{}); ok {
					ev.ToolInput = truncateToolInput(input)
				}
			}
		}

	case "tool_result":
		ev.Type = "tool_result"
		ev.ResultToolID, _ = raw["tool_use_id"].(string)
		// Extract content — can be string or array of content blocks
		switch c := raw["content"].(type) {
		case string:
			ev.ResultText = c
		case []interface{}:
			for _, block := range c {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if text, ok := blockMap["text"].(string); ok {
						ev.ResultText = text
						break
					}
				}
			}
		}
		if isErr, ok := raw["is_error"].(bool); ok {
			ev.IsError = isErr
		}

	case "result":
		ev.Type = "assistant_text"
		// Result events may have a "result" string or nested content
		switch r := raw["result"].(type) {
		case string:
			ev.Text = r
		case map[string]interface{}:
			ev.Text = extractTextFromContent(r)
		}

	case "error":
		ev.Type = "error"
		ev.IsError = true
		if errObj, ok := raw["error"].(map[string]interface{}); ok {
			ev.Text, _ = errObj["message"].(string)
		}
		if ev.Text == "" {
			ev.Text, _ = raw["message"].(string)
		}

	case "user":
		ev.Type = "user"
		if msg, ok := raw["message"].(map[string]interface{}); ok {
			if content, ok := msg["content"].(string); ok {
				ev.Text = content
			}
		}

	case "system":
		ev.Type = "system"
		ev.Text, _ = raw["message"].(string)
	}

	return ev
}

// truncateToolInput returns a short string representation of tool input for display.
func truncateToolInput(input map[string]interface{}) string {
	// Show the first key=value pair or a short summary
	const maxLen = 80
	s := fmt.Sprintf("%v", input)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// renderEventLine renders a single session event as a styled line for the viewer.
func renderEventLine(ev sessionEvent, styles *ccStyles) string {
	switch ev.Type {
	case "assistant_text":
		icon := lipgloss.NewStyle().Foreground(styles.ColorCyan).Bold(true).Render("◆")
		label := lipgloss.NewStyle().Foreground(styles.ColorCyan).Bold(true).Render("Assistant")
		text := truncateForViewer(ev.Text, 200)
		return fmt.Sprintf("%s %s  %s", icon, label, text)

	case "tool_use":
		icon := lipgloss.NewStyle().Foreground(styles.ColorYellow).Render("▸")
		label := lipgloss.NewStyle().Foreground(styles.ColorYellow).Bold(true).Render("Tool: " + ev.ToolName)
		return fmt.Sprintf("%s %s", icon, label)

	case "tool_result":
		status := "success"
		color := styles.ColorGreen
		if ev.IsError {
			status = "error"
			color = lipgloss.Color("#FF5555")
		}
		icon := lipgloss.NewStyle().Foreground(color).Render("◂")
		label := lipgloss.NewStyle().Foreground(color).Render("Result (" + status + ")")
		return fmt.Sprintf("%s %s", icon, label)

	case "error":
		icon := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render("⚠")
		label := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render("BLOCKED:")
		text := truncateForViewer(ev.Text, 200)
		return fmt.Sprintf("%s %s %s", icon, label, text)

	case "user":
		icon := lipgloss.NewStyle().Foreground(styles.ColorPurple).Render("▷")
		label := lipgloss.NewStyle().Foreground(styles.ColorPurple).Bold(true).Render("You")
		text := truncateForViewer(ev.Text, 200)
		return fmt.Sprintf("%s %s  %s", icon, label, text)

	case "system":
		icon := lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("●")
		label := lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("System")
		text := truncateForViewer(ev.Text, 200)
		return fmt.Sprintf("%s %s  %s", icon, label, text)

	default:
		return ""
	}
}

// truncateForViewer truncates text to maxLen for display, replacing newlines with spaces.
func truncateForViewer(s string, maxLen int) string {
	// Replace newlines with spaces for single-line display
	cleaned := ""
	for _, r := range s {
		if r == '\n' || r == '\r' {
			cleaned += " "
		} else {
			cleaned += string(r)
		}
	}
	if len(cleaned) > maxLen {
		return cleaned[:maxLen] + "..."
	}
	return cleaned
}

// listenForAgentEvent returns a tea.Cmd that blocks on the event channel and
// returns an agentEventMsg when an event arrives, or agentEventsDoneMsg when
// the channel is closed. This is the idiomatic bubbletea async pattern.
func listenForAgentEvent(todoID string, ch <-chan sessionEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentEventsDoneMsg{todoID: todoID}
		}
		return agentEventMsg{todoID: todoID, event: ev}
	}
}
