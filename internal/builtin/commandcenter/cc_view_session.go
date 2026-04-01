package commandcenter

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// renderSessionViewer renders the full-screen session viewer sub-view.
func (p *Plugin) renderSessionViewer(width, height int) string {
	viewWidth := ui.ContentMaxWidth
	if width > 0 && width < viewWidth {
		viewWidth = width
	}
	viewHeight := height
	if viewHeight < 10 {
		viewHeight = 10
	}
	innerWidth := viewWidth - 4
	if innerWidth < 40 {
		innerWidth = 40
	}

	s := &p.styles

	// Find the todo for the title
	var todoTitle string
	if todo := p.sessionViewerTodo(); todo != nil {
		todoTitle = flattenTitle(todo.Title)
	}
	titleMax := innerWidth - len("SESSION VIEWER — ") - 2
	if titleMax < 10 {
		titleMax = 10
	}
	title := truncateToWidth(todoTitle, titleMax)
	header := s.SectionHeader.Render("SESSION VIEWER — " + title)

	// Status line
	statusLine := p.buildSessionStatusLine(s)

	// Divider
	divider := s.DescMuted.Render(strings.Repeat("\u2500", innerWidth-2))

	// Resize viewport to fit current dimensions.
	// Chrome: header(1) + blank(1) + statusLine(1) + divider(1) + blank(1) + viewport + blank(1) + hints(1) + border(2) = 8
	// When input mode is active, add space for the label(1) + textarea(2) + blank(1) = 4
	inputChrome := 0
	if p.sessionViewerInputting {
		inputChrome = 4
	}
	vpHeight := viewHeight - 8 - inputChrome
	if vpHeight < 3 {
		vpHeight = 3
	}
	vpWidth := innerWidth - 4
	if vpWidth < 40 {
		vpWidth = 40
	}

	if p.sessionViewerVP.Width != vpWidth || p.sessionViewerVP.Height != vpHeight {
		p.sessionViewerVP.Width = vpWidth
		p.sessionViewerVP.Height = vpHeight
		// Re-set content after dimension change so viewport recalculates line window.
		content := p.buildSessionViewerContent(s)
		p.sessionViewerVP.SetContent(content)
	}

	// Hints
	var hints string
	if p.sessionViewerInputting {
		hints = s.Hint.Render("enter send \u00b7 esc cancel")
	} else if p.sessionViewerDone && p.isReplayingActiveSession() {
		hints = s.Hint.Render("j/k scroll \u00b7 G bottom \u00b7 g top \u00b7 o join \u00b7 esc back \u00b7 end of log")
	} else if p.sessionViewerDone {
		hints = s.Hint.Render("j/k scroll \u00b7 G bottom \u00b7 g top \u00b7 o join \u00b7 esc back \u00b7 session ended")
	} else {
		hints = s.Hint.Render("j/k scroll \u00b7 G bottom \u00b7 g top \u00b7 c message \u00b7 o join \u00b7 esc back")
	}

	parts := []string{
		"",
		"  " + header,
		"  " + statusLine,
		"  " + divider,
		"",
		lipgloss.NewStyle().PaddingLeft(2).Render(p.sessionViewerVP.View()),
	}

	// Add textarea input area when in input mode
	if p.sessionViewerInputting {
		inputLabel := s.SectionHeader.Render("MESSAGE:")
		parts = append(parts, "", "  "+inputLabel, "  "+p.sessionViewerInput.View())
	}

	parts = append(parts, "", "  "+hints)

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return s.PanelBorder.Width(innerWidth).Render(content)
}

// buildSessionStatusLine builds the status bar for the session viewer.
func (p *Plugin) buildSessionStatusLine(s *ccStyles) string {
	// Status indicator — check daemon for agent status.
	var statusPart string
	var sessionIDPart string
	var elapsedPart string

	if p.sessionViewerDone && p.isReplayingActiveSession() {
		statusPart = lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("streaming \u25cf")
	} else if p.sessionViewerDone {
		statusPart = lipgloss.NewStyle().Foreground(s.ColorGreen).Bold(true).Render("completed \u25cf")
	} else if dc := p.daemonClient(); dc != nil {
		if agentStatus, err := dc.AgentStatus(p.sessionViewerTodoID); err == nil {
			switch agentStatus.Status {
			case "blocked":
				statusPart = lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")).Bold(true).Render("blocked \u25cf")
			case "processing":
				statusPart = lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("active (daemon) \u25cf")
			default:
				statusPart = lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render(agentStatus.Status + " (daemon) \u25cf")
			}
			sid := agentStatus.SessionID
			if sid != "" {
				if len(sid) > 8 {
					sid = sid[:8]
				}
				sessionIDPart = s.DescMuted.Render("Session: " + sid)
			}
			if agentStatus.StartedAt != "" {
				if t, err := time.Parse(time.RFC3339, agentStatus.StartedAt); err == nil {
					elapsed := time.Since(t)
					if elapsed < time.Minute {
						elapsedPart = s.DescMuted.Render(fmt.Sprintf("%ds elapsed", int(elapsed.Seconds())))
					} else {
						mins := int(elapsed.Minutes())
						secs := int(elapsed.Seconds()) % 60
						elapsedPart = s.DescMuted.Render(fmt.Sprintf("%dm %02ds elapsed", mins, secs))
					}
				}
			}
		} else {
			statusPart = s.DescMuted.Render("inactive")
		}
	} else {
		statusPart = s.DescMuted.Render("inactive")
	}

	parts := []string{"Status: " + statusPart}
	if sessionIDPart != "" {
		parts = append(parts, sessionIDPart)
	}
	if elapsedPart != "" {
		parts = append(parts, elapsedPart)
	}

	return strings.Join(parts, " | ")
}

// buildSessionViewerContent renders all events into a single string for the viewport.
func (p *Plugin) buildSessionViewerContent(s *ccStyles) string {
	// Events come from the daemon replay buffer.
	var events []sessionEvent
	if len(p.sessionViewerReplayEvents) > 0 {
		events = p.sessionViewerReplayEvents
	}

	if len(events) == 0 {
		if p.sessionViewerDone {
			return s.DescMuted.Render("Session has ended. No events captured.")
		}
		return s.DescMuted.Render("Waiting for events...")
	}

	wrapWidth := p.sessionViewerVP.Width
	var lines []string
	for _, ev := range events {
		line := renderEventLine(ev, s, wrapWidth)
		if line != "" {
			lines = append(lines, line)
		}
	}

	if p.sessionViewerDone && p.isReplayingActiveSession() {
		lines = append(lines, "", s.DescMuted.Render("--- end of log ---"))
	} else if p.sessionViewerDone {
		lines = append(lines, "", s.DescMuted.Render("--- session ended ---"))
	}

	return strings.Join(lines, "\n")
}

// isReplayingActiveSession returns true when the viewer is showing a log replay
// for a session whose todo is still in an agent state (running/enqueued/blocked).
// This distinguishes "end of log file" from "session actually finished".
func (p *Plugin) isReplayingActiveSession() bool {
	if len(p.sessionViewerReplayEvents) == 0 {
		return false // not in replay mode
	}
	todo := p.sessionViewerTodo()
	return todo != nil && db.IsAgentStatus(todo.Status)
}

// sessionViewerTodo returns the todo for the session viewer.
func (p *Plugin) sessionViewerTodo() *db.Todo {
	if p.cc == nil || p.sessionViewerTodoID == "" {
		return nil
	}
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == p.sessionViewerTodoID {
			return &p.cc.Todos[i]
		}
	}
	return nil
}

// initSessionViewer sets up the session viewer state for a given todo.
// If the TUI has restarted but the agent still runs in the daemon, it reconnects
// by fetching current agent status and replaying events from the log file.
func (p *Plugin) initSessionViewer(todoID string) {
	p.sessionViewerActive = true
	p.sessionViewerTodoID = todoID
	p.sessionViewerAutoScroll = true
	p.sessionViewerDone = false
	p.sessionViewerInputting = false

	// Try to reconnect via daemon for event replay.
	p.tryDaemonReconnect(todoID)

	p.sessionViewerVP = viewport.New(80, 20) // will be resized on render
	p.sessionViewerVP.SetContent(p.buildSessionViewerContent(&p.styles))
	// Jump to bottom
	p.sessionViewerVP.GotoBottom()
}

// tryDaemonReconnect attempts to recover session viewer state from the daemon
// when the TUI has restarted but the agent is still running in the daemon process.
func (p *Plugin) tryDaemonReconnect(todoID string) {
	dc := p.daemonClient()
	if dc == nil {
		return
	}

	status, err := dc.AgentStatus(todoID)
	if err != nil {
		// Agent not found in daemon — nothing to reconnect to.
		return
	}

	// Agent exists in daemon. Populate replay events from the log file if available.
	if todo := p.sessionViewerTodo(); todo != nil && todo.SessionLogPath != "" {
		_ = p.loadReplayEvents(todo.SessionLogPath)
	}

	// Set viewer state based on daemon-reported status.
	switch status.Status {
	case "blocked":
		// Agent is alive but waiting for input.
		p.sessionViewerDone = false
	case "completed", "failed":
		p.sessionViewerDone = true
	default:
		// "processing", "queued", or anything else — agent is active.
		p.sessionViewerDone = false
	}
}

// loadReplayEvents reads events from a JSONL log file into sessionViewerReplayEvents.
func (p *Plugin) loadReplayEvents(logPath string) error {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return err
	}

	var events []sessionEvent
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		parsed := parseSessionEvent(raw)
		events = append(events, parsed...)
	}
	p.sessionViewerReplayEvents = events
	return nil
}

// initSessionViewerFromLog sets up the session viewer from a saved JSONL log file on disk.
func (p *Plugin) initSessionViewerFromLog(todoID, logPath string) error {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("cannot read session log: %w", err)
	}

	var events []sessionEvent
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue // skip malformed lines
		}
		parsed := parseSessionEvent(raw)
		events = append(events, parsed...)
	}

	p.sessionViewerReplayEvents = events
	p.sessionViewerActive = true
	p.sessionViewerTodoID = todoID
	p.sessionViewerDone = true
	p.sessionViewerAutoScroll = false
	p.sessionViewerListening = false
	p.sessionViewerInputting = false

	p.sessionViewerVP = viewport.New(80, 20) // will be resized on render
	p.sessionViewerVP.SetContent(p.buildSessionViewerContent(&p.styles))
	p.sessionViewerVP.GotoTop()

	return nil
}

// updateSessionViewerContent refreshes the viewport content and optionally auto-scrolls.
func (p *Plugin) updateSessionViewerContent() {
	content := p.buildSessionViewerContent(&p.styles)
	p.sessionViewerVP.SetContent(content)
	if p.sessionViewerAutoScroll {
		p.sessionViewerVP.GotoBottom()
	}
}
