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

// parseSessionEvent maps a raw stream-json event (already unmarshaled) to one or more sessionEvents.
// Assistant messages with multiple content blocks (e.g., text + tool_use) produce one event per block.
func parseSessionEvent(raw map[string]interface{}) []sessionEvent {
	eventType, _ := raw["type"].(string)

	switch eventType {
	case "assistant":
		// Stream-json nests content under "message.content", not top-level "content".
		content := extractContentArray(raw)
		if content == nil {
			return nil
		}
		var events []sessionEvent
		for _, block := range content {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "text":
				text, _ := blockMap["text"].(string)
				events = append(events, sessionEvent{
					Type: "assistant_text",
					Text: text,
				})
			case "tool_use":
				ev := sessionEvent{
					Type: "tool_use",
				}
				ev.ToolName, _ = blockMap["name"].(string)
				ev.ToolID, _ = blockMap["id"].(string)
				if input, ok := blockMap["input"].(map[string]interface{}); ok {
					ev.ToolInput = truncateToolInput(input)
				}
				events = append(events, ev)
			}
		}
		return events

	case "tool_result":
		ev := sessionEvent{Type: "tool_result"}
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
		return []sessionEvent{ev}

	case "result":
		ev := sessionEvent{Type: "assistant_text"}
		// Result events may have a "result" string or nested content
		switch r := raw["result"].(type) {
		case string:
			ev.Text = r
		case map[string]interface{}:
			ev.Text = extractTextFromContent(r)
		}
		return []sessionEvent{ev}

	case "error":
		ev := sessionEvent{Type: "error", IsError: true}
		if errObj, ok := raw["error"].(map[string]interface{}); ok {
			ev.Text, _ = errObj["message"].(string)
		}
		if ev.Text == "" {
			ev.Text, _ = raw["message"].(string)
		}
		return []sessionEvent{ev}

	case "user":
		// User events in stream-json: message.content can be a string or an array
		// of content blocks (tool_result, text, etc.).
		if msg, ok := raw["message"].(map[string]interface{}); ok {
			switch c := msg["content"].(type) {
			case string:
				if c != "" {
					return []sessionEvent{{Type: "user", Text: c}}
				}
			case []interface{}:
				var events []sessionEvent
				for _, block := range c {
					bm, ok := block.(map[string]interface{})
					if !ok {
						continue
					}
					switch bm["type"] {
					case "text":
						if t, ok := bm["text"].(string); ok && t != "" {
							events = append(events, sessionEvent{Type: "user", Text: t})
						}
					case "tool_result":
						ev := sessionEvent{Type: "tool_result"}
						ev.ResultToolID, _ = bm["tool_use_id"].(string)
						// tool_result content can be string or nested
						switch rc := bm["content"].(type) {
						case string:
							ev.ResultText = rc
						case []interface{}:
							for _, rb := range rc {
								if rbm, ok := rb.(map[string]interface{}); ok {
									if t, ok := rbm["text"].(string); ok {
										ev.ResultText = t
										break
									}
								}
							}
						}
						events = append(events, ev)
					}
				}
				if len(events) > 0 {
					return events
				}
			}
		}
		// No displayable content — skip.
		return nil

	case "system":
		ev := sessionEvent{Type: "system"}
		// Try "message" first, then fall back to describing the event from other fields.
		ev.Text, _ = raw["message"].(string)
		if ev.Text == "" {
			if subtype, ok := raw["subtype"].(string); ok && subtype != "" {
				ev.Text = subtype
			} else if sid, ok := raw["session_id"].(string); ok && sid != "" {
				ev.Text = "session " + sid[:min(8, len(sid))]
			}
		}
		// Skip system events with no displayable content.
		if ev.Text == "" {
			return nil
		}
		return []sessionEvent{ev}
	}

	return nil
}

// extractContentArray gets the content array from a stream-json event.
// Stream-json nests content under "message.content"; falls back to top-level "content".
func extractContentArray(raw map[string]interface{}) []interface{} {
	if msg, ok := raw["message"].(map[string]interface{}); ok {
		if content, ok := msg["content"].([]interface{}); ok {
			return content
		}
	}
	if content, ok := raw["content"].([]interface{}); ok {
		return content
	}
	return nil
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
		icon := lipgloss.NewStyle().Foreground(styles.ColorCyan).Bold(true).Render("◆")
		label := lipgloss.NewStyle().Foreground(styles.ColorCyan).Bold(true).Render("Assistant")
		text := wrapText(ev.Text, textWidth)
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
		text := wrapText(ev.Text, textWidth)
		return fmt.Sprintf("%s %s %s", icon, label, text)

	case "user":
		icon := lipgloss.NewStyle().Foreground(styles.ColorPurple).Render("▷")
		label := lipgloss.NewStyle().Foreground(styles.ColorPurple).Bold(true).Render("You")
		text := wrapText(ev.Text, textWidth)
		return fmt.Sprintf("%s %s  %s", icon, label, text)

	case "system":
		icon := lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("●")
		label := lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("System")
		text := wrapText(ev.Text, textWidth)
		return fmt.Sprintf("%s %s  %s", icon, label, text)

	default:
		return ""
	}
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
