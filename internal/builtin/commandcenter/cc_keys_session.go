package commandcenter

import (
	"os"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// handleSessionViewer handles key input when the session viewer is active.
func (p *Plugin) handleSessionViewer(msg tea.KeyMsg) plugin.Action {
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
		// Phase 3 will implement message input. For now, set a flag placeholder.
		// TODO: implement message input in Phase 3
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
