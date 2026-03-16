package commandcenter

import (
	"fmt"
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
	viewHeight := height - 14
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
	sess := p.activeSessions[p.sessionViewerTodoID]

	// Status indicator
	var statusPart string
	if p.sessionViewerDone {
		statusPart = lipgloss.NewStyle().Foreground(s.ColorGreen).Bold(true).Render("completed \u25cf")
	} else if sess != nil {
		sess.mu.Lock()
		status := sess.Status
		sess.mu.Unlock()
		switch status {
		case "blocked":
			statusPart = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render("blocked \u25cf")
		default:
			statusPart = lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("active \u25cf")
		}
	} else {
		statusPart = s.DescMuted.Render("inactive")
	}

	// Session ID
	var sessionIDPart string
	if sess != nil {
		sess.mu.Lock()
		sid := sess.SessionID
		sess.mu.Unlock()
		if sid != "" {
			if len(sid) > 8 {
				sid = sid[:8]
			}
			sessionIDPart = s.DescMuted.Render("Session: " + sid)
		}
	}

	// Elapsed time
	var elapsedPart string
	if sess != nil {
		elapsed := time.Since(sess.StartedAt)
		if elapsed < time.Minute {
			elapsedPart = s.DescMuted.Render(fmt.Sprintf("%ds elapsed", int(elapsed.Seconds())))
		} else {
			mins := int(elapsed.Minutes())
			secs := int(elapsed.Seconds()) % 60
			elapsedPart = s.DescMuted.Render(fmt.Sprintf("%dm %02ds elapsed", mins, secs))
		}
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
	sess := p.activeSessions[p.sessionViewerTodoID]
	var events []sessionEvent
	if sess != nil {
		sess.mu.Lock()
		events = make([]sessionEvent, len(sess.Events))
		copy(events, sess.Events)
		sess.mu.Unlock()
	}

	if len(events) == 0 {
		if p.sessionViewerDone {
			return s.DescMuted.Render("Session has ended. No events captured.")
		}
		return s.DescMuted.Render("Waiting for events...")
	}

	var lines []string
	for _, ev := range events {
		line := renderEventLine(ev, s)
		if line != "" {
			lines = append(lines, line)
		}
	}

	if p.sessionViewerDone {
		lines = append(lines, "", s.DescMuted.Render("--- session ended ---"))
	}

	return strings.Join(lines, "\n")
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
func (p *Plugin) initSessionViewer(todoID string) {
	p.sessionViewerActive = true
	p.sessionViewerTodoID = todoID
	p.sessionViewerAutoScroll = true
	p.sessionViewerDone = false
	p.sessionViewerInputting = false

	p.sessionViewerVP = viewport.New(80, 20) // will be resized on render
	p.sessionViewerVP.SetContent(p.buildSessionViewerContent(&p.styles))
	// Jump to bottom
	p.sessionViewerVP.GotoBottom()
}

// updateSessionViewerContent refreshes the viewport content and optionally auto-scrolls.
func (p *Plugin) updateSessionViewerContent() {
	content := p.buildSessionViewerContent(&p.styles)
	p.sessionViewerVP.SetContent(content)
	if p.sessionViewerAutoScroll {
		p.sessionViewerVP.GotoBottom()
	}
}
