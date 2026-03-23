package commandcenter

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

// queuedSession describes a session waiting to launch.
// This is the CC-specific wrapper around agent.Request that includes
// CC-specific fields like TodoID and prompt enhancement.
type queuedSession struct {
	TodoID     string
	Prompt     string
	ProjectDir string
	Mode       string
	Perm       string
	Budget     float64
	AutoStart  bool
	ResumeID   string // If set, resume an existing session instead of starting a new one
}

// toAgentRequest converts a CC-specific queuedSession to a generic agent.Request.
// It enhances the prompt with summary instructions so the agent self-reports.
func (qs queuedSession) toAgentRequest() agent.Request {
	enhancedPrompt := fmt.Sprintf(`%s

---

When you have completed your work, run this command to submit a detailed summary of what you did:

ccc update-todo --id %s --session-summary "$(cat <<'SUMMARY'
## What was done
<files created/modified and why>

## Key decisions
<choices made and rationale>

## Needs review
<anything requiring human attention>

## Open questions
<unresolved items, if any>
SUMMARY
)"`, qs.Prompt, qs.TodoID)

	return agent.Request{
		ID:         qs.TodoID,
		Prompt:     enhancedPrompt,
		ProjectDir: qs.ProjectDir,
		Worktree:   qs.Mode == "worktree",
		Permission: qs.Perm,
		Budget:     qs.Budget,
		ResumeID:   qs.ResumeID,
		AutoStart:  qs.AutoStart,
	}
}

// killAgent terminates a running agent session for a given todo.
// Returns a tea.Cmd for any DB/event side effects, or nil if no session was running.
func (p *Plugin) killAgent(todoID string) tea.Cmd {
	if !p.agentRunner.Kill(todoID) {
		return nil
	}

	p.setTodoStatus(todoID, db.StatusBacklog)
	p.publishEvent("agent.killed", map[string]interface{}{
		"todo_id": todoID,
	})

	// If the session viewer is watching this session, mark it done.
	if p.sessionViewerActive && p.sessionViewerTodoID == todoID {
		p.sessionViewerDone = true
		p.sessionViewerListening = false
		p.updateSessionViewerContent()
	}

	return p.persistTodoStatus(todoID, db.StatusBacklog)
}

// canLaunchAgent checks if there is room to launch another agent session.
func (p *Plugin) canLaunchAgent() bool {
	maxConcurrent := p.cfg.Agent.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return len(p.agentRunner.Active()) < maxConcurrent
}

// activeAgentCount returns the number of currently running agent sessions.
func (p *Plugin) activeAgentCount() int {
	return len(p.agentRunner.Active())
}

// queuedAgentCount returns the number of sessions waiting in the queue.
func (p *Plugin) queuedAgentCount() int {
	return p.agentRunner.QueueLen()
}

// launchOrQueueAgent either launches an agent immediately or queues it.
// It also auto-accepts the todo so it moves out of the "new" triage filter.
func (p *Plugin) launchOrQueueAgent(qs queuedSession) tea.Cmd {
	// Auto-accept the todo when launching/queuing an agent
	p.cc.AcceptTodo(qs.TodoID)
	acceptCmd := p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBAcceptTodo(database, qs.TodoID)
	})

	req := qs.toAgentRequest()
	queued, launchCmd := p.agentRunner.LaunchOrQueue(req)

	if !queued {
		p.setTodoStatus(qs.TodoID, db.StatusRunning)
		p.publishEvent("agent.started", map[string]interface{}{
			"todo_id": qs.TodoID,
		})
		return tea.Batch(acceptCmd, launchCmd)
	}

	// Queued.
	p.setTodoStatus(qs.TodoID, db.StatusEnqueued)
	p.publishEvent("agent.queued", map[string]interface{}{
		"todo_id": qs.TodoID,
	})
	return tea.Batch(acceptCmd, p.persistTodoStatus(qs.TodoID, db.StatusEnqueued))
}

// onAgentFinished cleans up after an agent finishes and launches the next queued item.
func (p *Plugin) onAgentFinished(todoID string, exitCode int) tea.Cmd {
	var summary string

	// Check if the agent already submitted a summary via `ccc update-todo`.
	if p.database != nil {
		if dbTodo, err := db.DBLoadTodoByID(p.database, todoID); err == nil && dbTodo != nil && dbTodo.SessionSummary != "" {
			summary = dbTodo.SessionSummary
		}
	}

	// Clean up the session from the runner and extract summary if needed.
	if sess := p.agentRunner.CleanupFinished(todoID); sess != nil && summary == "" {
		summary = agent.ExtractSessionSummary(sess)
	}

	status := db.StatusReview
	if exitCode != 0 {
		status = db.StatusFailed
	}
	p.setTodoStatus(todoID, status)
	p.setTodoSessionSummary(todoID, summary)
	p.publishEvent("agent.completed", map[string]interface{}{
		"todo_id":   todoID,
		"exit_code": exitCode,
		"status":    status,
	})

	var cmds []tea.Cmd
	cmds = append(cmds, p.persistTodoStatusAndSummary(todoID, status, summary))

	// Check queue for next auto-start item.
	if next, ok := p.agentRunner.DrainQueue(); ok {
		if next.AutoStart {
			p.setTodoStatus(next.ID, db.StatusRunning)
			p.publishEvent("agent.started", map[string]interface{}{
				"todo_id": next.ID,
			})
			_, launchCmd := p.agentRunner.LaunchOrQueue(next)
			if launchCmd != nil {
				cmds = append(cmds, launchCmd)
			}
		}
	}

	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

// checkAgentProcesses polls active sessions for completion and status changes.
func (p *Plugin) checkAgentProcesses() tea.Cmd {
	return p.agentRunner.CheckProcesses()
}

// setTodoStatus updates the status of a todo in-memory.
func (p *Plugin) setTodoStatus(todoID, status string) {
	if p.cc == nil {
		return
	}
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == todoID {
			p.cc.Todos[i].Status = status
			return
		}
	}
}

// persistTodoStatus returns a tea.Cmd that writes the status to the DB.
func (p *Plugin) persistTodoStatus(todoID, status string) tea.Cmd {
	return p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBUpdateTodoStatus(database, todoID, status)
	})
}

// setTodoProjectDir updates the project dir of a todo in-memory.
func (p *Plugin) setTodoProjectDir(todoID, projectDir string) {
	if p.cc == nil {
		return
	}
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == todoID {
			p.cc.Todos[i].ProjectDir = projectDir
			return
		}
	}
}

// persistProjectDir returns a tea.Cmd that writes the project dir to the DB.
func (p *Plugin) persistProjectDir(todoID, projectDir string) tea.Cmd {
	return p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBUpdateTodoProjectDir(database, todoID, projectDir)
	})
}

// setTodoLaunchMode updates the launch mode of a todo in-memory.
func (p *Plugin) setTodoLaunchMode(todoID, launchMode string) {
	if p.cc == nil {
		return
	}
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == todoID {
			p.cc.Todos[i].LaunchMode = launchMode
			return
		}
	}
}

// persistLaunchMode returns a tea.Cmd that writes the launch mode to the DB.
func (p *Plugin) persistLaunchMode(todoID, launchMode string) tea.Cmd {
	return p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBUpdateTodoLaunchMode(database, todoID, launchMode)
	})
}

// setTodoSessionLogPath updates the session log path of a todo in-memory.
func (p *Plugin) setTodoSessionLogPath(todoID, path string) {
	if p.cc == nil {
		return
	}
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == todoID {
			p.cc.Todos[i].SessionLogPath = path
			return
		}
	}
}

// persistSessionLogPath returns a tea.Cmd that writes the session log path to the DB.
func (p *Plugin) persistSessionLogPath(todoID, path string) tea.Cmd {
	return p.dbWriteCmd(func(database *sql.DB) error {
		now := db.FormatTime(time.Now())
		_, err := database.Exec(`UPDATE cc_todos SET session_log_path = NULLIF(?, ''), updated_at = ? WHERE id = ?`, path, now, todoID)
		return err
	})
}

// setTodoSessionSummary updates the session summary of a todo in-memory.
func (p *Plugin) setTodoSessionSummary(todoID, summary string) {
	if p.cc == nil {
		return
	}
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == todoID {
			p.cc.Todos[i].SessionSummary = summary
			return
		}
	}
}

// persistTodoStatusAndSummary returns a tea.Cmd that writes both status and summary to the DB.
func (p *Plugin) persistTodoStatusAndSummary(todoID, status, summary string) tea.Cmd {
	return p.dbWriteCmd(func(database *sql.DB) error {
		now := db.FormatTime(time.Now())
		_, err := database.Exec(`UPDATE cc_todos SET status = ?, session_summary = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
			status, summary, now, todoID)
		if err != nil {
			return fmt.Errorf("update todo status+summary %s: %w", todoID, err)
		}
		return nil
	})
}
