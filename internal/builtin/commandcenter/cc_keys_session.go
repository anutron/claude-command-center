package commandcenter

import (
	"os"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// handleSessionViewer handles key input when the session viewer is active.
func (p *Plugin) handleSessionViewer(msg tea.KeyMsg) plugin.Action {
	// When inputting, route keys to the textarea first.
	if p.sessionViewerInputting {
		return p.handleSessionViewerInput(msg)
	}

	switch msg.String() {
	case "j", "down":
		p.sessionViewerAutoScroll = false
		p.sessionViewerVP.LineDown(1)
		return plugin.ConsumedAction()

	case "k", "up":
		p.sessionViewerAutoScroll = false
		p.sessionViewerVP.LineUp(1)
		return plugin.ConsumedAction()

	case "G":
		// Jump to bottom and re-enable auto-scroll
		p.sessionViewerVP.GotoBottom()
		p.sessionViewerAutoScroll = true
		return plugin.ConsumedAction()

	case "g":
		// Jump to top, disable auto-scroll
		p.sessionViewerVP.GotoTop()
		p.sessionViewerAutoScroll = false
		return plugin.ConsumedAction()

	case "c":
		// Open message input textarea
		p.sessionViewerInputting = true
		ta := textarea.New()
		ta.Placeholder = "Type a message..."
		ta.CharLimit = 0
		ta.ShowLineNumbers = false
		ta.SetWidth(p.textareaWidth())
		ta.SetHeight(2)
		ta.FocusedStyle.Base = ta.FocusedStyle.Base.Foreground(p.styles.ColorWhite)
		ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
		ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(p.styles.ColorWhite)
		ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(p.styles.ColorMuted)
		ta.Focus()
		p.sessionViewerInput = ta
		return plugin.ConsumedAction()

	case "o":
		// Join session interactively (existing launch flow)
		if todo := p.sessionViewerTodo(); todo != nil && todo.SessionID != "" {
			dir := todo.ProjectDir
			if dir == "" {
				home, _ := os.UserHomeDir()
				dir = home
			}
			// Exit session viewer before launching
			p.sessionViewerActive = false
			return plugin.Action{
				Type: "launch",
				Args: map[string]string{
					"dir":       dir,
					"resume_id": todo.SessionID,
					"todo_id":   todo.ID,
				},
			}
		}
		return plugin.ConsumedAction()

	case "esc":
		// Exit viewer, back to detail view
		p.sessionViewerActive = false
		return plugin.ConsumedAction()
	}

	// Pass viewport-related messages (page up/down, etc.)
	var cmd tea.Cmd
	p.sessionViewerVP, cmd = p.sessionViewerVP.Update(msg)
	if cmd != nil {
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}
	return plugin.ConsumedAction()
}

// handleSessionViewerInput handles key input when the session viewer textarea is active.
func (p *Plugin) handleSessionViewerInput(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "esc":
		// Cancel input, blur textarea
		p.sessionViewerInputting = false
		p.sessionViewerInput.Blur()
		return plugin.ConsumedAction()

	case "enter":
		// Send message to agent
		text := p.sessionViewerInput.Value()
		if text == "" {
			// Don't send empty messages, just cancel
			p.sessionViewerInputting = false
			p.sessionViewerInput.Blur()
			return plugin.ConsumedAction()
		}

		sent := false
		// Try daemon RPC first for sending input.
		if dc := p.daemonClient(); dc != nil {
			if err := dc.SendAgentInput(p.sessionViewerTodoID, text); err != nil {
				if p.logger != nil {
					p.logger.Warn("commandcenter", "daemon SendAgentInput failed, falling back to local", "err", err)
				}
			} else {
				sent = true
			}
		}

		// Local runner fallback.
		if !sent {
			sess := p.agentRunner.Session(p.sessionViewerTodoID)
			if sess != nil {
				if err := agent.SendUserMessage(sess, text); err != nil {
					if p.logger != nil {
						p.logger.Warn("commandcenter", "failed to send user message", "err", err)
					}
				} else {
					sent = true
				}
			}
		}

		if sent {
			// Append a user event for display (works for both daemon and local).
			if sess := p.agentRunner.Session(p.sessionViewerTodoID); sess != nil {
				userEvent := sessionEvent{
					Type:      "user",
					Text:      text,
					Timestamp: time.Now().Format(time.RFC3339),
				}
				sess.Mu.Lock()
				sess.Events = append(sess.Events, userEvent)
				sess.Mu.Unlock()
			}
			p.updateSessionViewerContent()
		}

		// Clear input and exit input mode
		p.sessionViewerInputting = false
		p.sessionViewerInput.Blur()
		return plugin.ConsumedAction()
	}

	// Pass all other keys to the textarea
	var cmd tea.Cmd
	p.sessionViewerInput, cmd = p.sessionViewerInput.Update(msg)
	if cmd != nil {
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}
	return plugin.ConsumedAction()
}
