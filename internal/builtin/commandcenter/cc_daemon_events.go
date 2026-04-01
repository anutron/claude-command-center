package commandcenter

import (
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

// listenForDaemonAgentEvents polls the daemon's StreamAgentOutput RPC for new
// events. It delivers one event per message so the session viewer updates
// incrementally, then re-polls for more.
func listenForDaemonAgentEvents(todoID string, client *daemon.Client, offset int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(500 * time.Millisecond)

		result, err := client.StreamAgentOutput(todoID)
		if err != nil {
			return agentEventsDoneMsg{todoID: todoID}
		}

		// Deliver the next unseen event.
		if len(result.Events) > offset {
			ev := result.Events[offset]
			return daemonAgentEventMsg{
				todoID: todoID,
				event:  ev,
				offset: offset + 1,
				done:   result.Done && offset+1 >= len(result.Events),
			}
		}

		// No new events yet.
		if result.Done {
			return agentEventsDoneMsg{todoID: todoID}
		}

		// Keep polling — more events may arrive.
		return daemonAgentPollMsg{todoID: todoID, offset: offset}
	}
}

// daemonAgentEventMsg carries a single event fetched from the daemon.
type daemonAgentEventMsg struct {
	todoID string
	event  sessionEvent
	offset int
	done   bool
}

// daemonAgentPollMsg signals that no new events were available and the
// listener should re-poll after a delay.
type daemonAgentPollMsg struct {
	todoID string
	offset int
}
