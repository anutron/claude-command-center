package commandcenter

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

// agentSession tracks a running headless Claude Code session.
type agentSession struct {
	TodoID    string
	SessionID string // Claude session UUID, captured from first stream-JSON event
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
	Status    string // "active", "blocked", "review", "failed"
	Question  string // populated when blocked
	StartedAt time.Time
	Output    strings.Builder // captures output

	// done is closed when the process exits. ExitCode is set before closing.
	done     chan struct{}
	exitCode int

	// mu protects Status, Question, and SessionID for concurrent writes from the goroutine.
	mu sync.Mutex
}

// queuedSession describes a session waiting to launch.
type queuedSession struct {
	TodoID     string
	Prompt     string
	ProjectDir string
	Mode       string
	Perm       string
	Budget     float64
	AutoStart  bool
}

// Tea messages for agent lifecycle.

type agentStartedMsg struct{ todoID string }

type agentStatusMsg struct {
	todoID   string
	status   string
	question string
}

type agentFinishedMsg struct {
	todoID   string
	exitCode int
}

type agentSessionIDMsg struct {
	todoID    string
	sessionID string
}

// agentStartedInternalMsg carries process handles so Update can store them.
type agentStartedInternalMsg struct {
	todoID  string
	session *agentSession
}

// launchAgent starts a headless Claude Code session and returns a tea.Cmd
// that sends agentStartedInternalMsg. A background goroutine monitors stdout
// for stream-JSON events and signals completion via the session's done channel.
func launchAgent(qs queuedSession) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())

		args := []string{
			"--print",
			"--output-format", "stream-json",
			"--verbose",
		}
		if qs.Perm != "" && qs.Perm != "default" {
			args = append(args, "--permission-mode", qs.Perm)
		}
		if qs.Budget >= 0.50 {
			args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", qs.Budget))
		}
		if qs.Mode == "worktree" {
			args = append(args, "--worktree")
		}
		args = append(args, qs.Prompt)

		cmd := exec.CommandContext(ctx, "claude", args...)
		if qs.ProjectDir != "" {
			cmd.Dir = qs.ProjectDir
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			return agentFinishedMsg{todoID: qs.TodoID, exitCode: -1}
		}

		if err := cmd.Start(); err != nil {
			cancel()
			return agentFinishedMsg{todoID: qs.TodoID, exitCode: -1}
		}

		sess := &agentSession{
			TodoID:    qs.TodoID,
			Cmd:       cmd,
			Cancel:    cancel,
			Status:    "active",
			StartedAt: time.Now(),
			done:      make(chan struct{}),
		}

		// Background goroutine: read stream-JSON stdout, detect blocking events,
		// and signal completion via done channel.
		go func() {
			defer func() {
				// Wait for process to finish, capture exit code, then signal done.
				exitCode := 0
				if err := cmd.Wait(); err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
					} else {
						exitCode = -1
					}
				}
				sess.mu.Lock()
				sess.exitCode = exitCode
				sess.mu.Unlock()
				close(sess.done)
			}()

			scanner := bufio.NewScanner(stdout)
			// Allow large lines (stream-JSON can be verbose).
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

			sessionIDCaptured := false
			for scanner.Scan() {
				line := scanner.Text()
				// Capture output for summary extraction.
				sess.mu.Lock()
				sess.Output.WriteString(line)
				sess.Output.WriteString("\n")
				sess.mu.Unlock()

				// Try to parse as JSON; skip malformed lines.
				var event map[string]interface{}
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					continue
				}

				// Capture session_id from the first event that has one.
				if !sessionIDCaptured {
					if sid, ok := event["session_id"].(string); ok && sid != "" {
						sessionIDCaptured = true
						sess.mu.Lock()
						sess.SessionID = sid
						sess.mu.Unlock()
					}
				}

				// Detect tool_use with SendUserMessage → agent is blocked.
				if detectBlockingEvent(event) {
					question := extractBlockingQuestion(event)
					sess.mu.Lock()
					sess.Status = "blocked"
					sess.Question = question
					sess.mu.Unlock()
				}
			}
		}()

		return agentStartedInternalMsg{
			todoID:  qs.TodoID,
			session: sess,
		}
	}
}

// detectBlockingEvent returns true if a stream-JSON event indicates the agent
// is waiting for user input (e.g., a tool_use with name "SendUserMessage" or
// similar).
func detectBlockingEvent(event map[string]interface{}) bool {
	eventType, ok := event["type"].(string)
	if !ok {
		return false
	}
	if eventType == "tool_use" {
		if name, ok := event["name"].(string); ok {
			if name == "SendUserMessage" || name == "AskUser" {
				return true
			}
		}
	}
	// Also check for assistant events that contain tool_use blocks.
	if eventType == "assistant" {
		if content, ok := event["content"].([]interface{}); ok {
			for _, block := range content {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if blockMap["type"] == "tool_use" {
						if name, ok := blockMap["name"].(string); ok {
							if name == "SendUserMessage" || name == "AskUser" {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

// extractBlockingQuestion tries to extract the question text from a blocking event.
func extractBlockingQuestion(event map[string]interface{}) string {
	if input, ok := event["input"].(map[string]interface{}); ok {
		if msg, ok := input["message"].(string); ok {
			return msg
		}
		if q, ok := input["question"].(string); ok {
			return q
		}
	}
	if content, ok := event["content"].([]interface{}); ok {
		for _, block := range content {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if blockMap["type"] == "tool_use" {
					if input, ok := blockMap["input"].(map[string]interface{}); ok {
						if msg, ok := input["message"].(string); ok {
							return msg
						}
					}
				}
			}
		}
	}
	return ""
}

// Concurrency manager methods on Plugin.

// canLaunchAgent checks if there is room to launch another agent session.
func (p *Plugin) canLaunchAgent() bool {
	maxConcurrent := p.cfg.Agent.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return len(p.activeSessions) < maxConcurrent
}

// activeAgentCount returns the number of currently running agent sessions.
func (p *Plugin) activeAgentCount() int {
	return len(p.activeSessions)
}

// queuedAgentCount returns the number of sessions waiting in the queue.
func (p *Plugin) queuedAgentCount() int {
	return len(p.sessionQueue)
}

// launchOrQueueAgent either launches an agent immediately or queues it.
// It also auto-accepts the todo so it moves out of the "new" triage filter.
func (p *Plugin) launchOrQueueAgent(qs queuedSession) tea.Cmd {
	// Auto-accept the todo when launching/queuing an agent
	p.cc.AcceptTodo(qs.TodoID)
	acceptCmd := p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBAcceptTodo(database, qs.TodoID)
	})

	if p.canLaunchAgent() {
		p.setTodoSessionStatus(qs.TodoID, "active")
		p.publishEvent("agent.started", map[string]interface{}{
			"todo_id": qs.TodoID,
		})
		return tea.Batch(acceptCmd, launchAgent(qs))
	}

	// Queue it.
	p.sessionQueue = append(p.sessionQueue, qs)
	p.setTodoSessionStatus(qs.TodoID, "queued")
	p.publishEvent("agent.queued", map[string]interface{}{
		"todo_id": qs.TodoID,
	})
	return tea.Batch(acceptCmd, p.persistSessionStatus(qs.TodoID, "queued"))
}

// onAgentFinished cleans up after an agent finishes and launches the next queued item.
func (p *Plugin) onAgentFinished(todoID string, exitCode int) tea.Cmd {
	var summary string
	if sess, ok := p.activeSessions[todoID]; ok {
		summary = extractSessionSummary(sess)
		if sess.Cancel != nil {
			sess.Cancel()
		}
		delete(p.activeSessions, todoID)
	}

	status := "review"
	if exitCode != 0 {
		status = "failed"
	}
	p.setTodoSessionStatus(todoID, status)
	p.setTodoSessionSummary(todoID, summary)
	p.publishEvent("agent.completed", map[string]interface{}{
		"todo_id":   todoID,
		"exit_code": exitCode,
		"status":    status,
	})

	var cmds []tea.Cmd
	cmds = append(cmds, p.persistSessionStatusAndSummary(todoID, status, summary))

	// Check queue for next auto-start item.
	if len(p.sessionQueue) > 0 && p.canLaunchAgent() {
		next := p.sessionQueue[0]
		p.sessionQueue = p.sessionQueue[1:]
		if next.AutoStart {
			p.setTodoSessionStatus(next.TodoID, "active")
			p.publishEvent("agent.started", map[string]interface{}{
				"todo_id": next.TodoID,
			})
			cmds = append(cmds, launchAgent(next))
		}
	}

	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

// setTodoSessionStatus updates the session status of a todo in-memory.
func (p *Plugin) setTodoSessionStatus(todoID, status string) {
	if p.cc == nil {
		return
	}
	for i := range p.cc.Todos {
		if p.cc.Todos[i].ID == todoID {
			p.cc.Todos[i].SessionStatus = status
			return
		}
	}
}

// persistSessionStatus returns a tea.Cmd that writes the session status to the DB.
func (p *Plugin) persistSessionStatus(todoID, status string) tea.Cmd {
	return p.dbWriteCmd(func(database *sql.DB) error {
		return db.DBUpdateTodoSessionStatus(database, todoID, status)
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

// persistSessionStatusAndSummary returns a tea.Cmd that writes both session status and summary to the DB.
func (p *Plugin) persistSessionStatusAndSummary(todoID, status, summary string) tea.Cmd {
	return p.dbWriteCmd(func(database *sql.DB) error {
		now := db.FormatTime(time.Now())
		_, err := database.Exec(`UPDATE cc_todos SET session_status = NULLIF(?, ''), session_summary = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
			status, summary, now, todoID)
		if err != nil {
			return fmt.Errorf("update todo session status+summary %s: %w", todoID, err)
		}
		return nil
	})
}

// extractSessionSummary extracts a human-readable summary from the agent session's
// stream-JSON output. It parses the JSON lines to find assistant text content blocks,
// returning the text from the last assistant message as the summary.
func extractSessionSummary(sess *agentSession) string {
	sess.mu.Lock()
	output := sess.Output.String()
	exitCode := sess.exitCode
	sess.mu.Unlock()

	if output == "" {
		if exitCode == 0 {
			return "Session completed successfully."
		}
		return fmt.Sprintf("Session exited with code %d.", exitCode)
	}

	// Parse stream-JSON lines to extract assistant text content.
	// Claude CLI stream-JSON emits one JSON object per line. We look for:
	// 1. "result" type events (final output) with a "result" field containing text
	// 2. "assistant" type events with "content" arrays containing text blocks
	// 3. "content_block_delta" events with text deltas
	//
	// We collect all text from the last assistant turn.
	var lastAssistantText string
	var resultText string

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)

		switch eventType {
		case "result":
			// The result event may contain a "result" field with the final text,
			// or a "subtype" of "success" with text content.
			if r, ok := event["result"].(string); ok && r != "" {
				resultText = r
			}
			// Also check for nested content in result
			if msg, ok := event["result"].(map[string]interface{}); ok {
				if text := extractTextFromContent(msg); text != "" {
					resultText = text
				}
			}

		case "assistant":
			// Assistant message with content array
			if text := extractTextFromContent(event); text != "" {
				lastAssistantText = text
			}

		case "message":
			// Some stream-JSON formats wrap in "message" type
			if role, _ := event["role"].(string); role == "assistant" {
				if text := extractTextFromContent(event); text != "" {
					lastAssistantText = text
				}
			}
		}
	}

	// Prefer result text over assistant text (result is the final output)
	summary := resultText
	if summary == "" {
		summary = lastAssistantText
	}
	if summary == "" {
		if exitCode == 0 {
			return "Session completed successfully."
		}
		return fmt.Sprintf("Session exited with code %d.", exitCode)
	}

	// Truncate if needed
	const maxLen = 1000
	if len(summary) > maxLen {
		summary = summary[:maxLen]
		// Trim to last complete line
		if idx := strings.LastIndex(summary, "\n"); idx > maxLen/2 {
			summary = summary[:idx]
		}
		summary += "\n..."
	}
	return strings.TrimSpace(summary)
}

// extractTextFromContent extracts text from a stream-JSON event's content array.
// It handles the common format: {"content": [{"type": "text", "text": "..."}]}
func extractTextFromContent(event map[string]interface{}) string {
	content, ok := event["content"].([]interface{})
	if !ok {
		return ""
	}
	var texts []string
	for _, block := range content {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := blockMap["type"].(string)
		if blockType == "text" {
			if text, ok := blockMap["text"].(string); ok && text != "" {
				texts = append(texts, text)
			}
		}
	}
	return strings.Join(texts, "\n")
}

// checkAgentProcesses polls active sessions for completion and status changes.
// Called from handleTickMsg on every tick to detect finished processes and
// status changes (e.g., blocked) set by the background goroutine.
func (p *Plugin) checkAgentProcesses() tea.Cmd {
	var cmds []tea.Cmd
	for todoID, sess := range p.activeSessions {
		// Check if process has finished via the done channel.
		select {
		case <-sess.done:
			sess.mu.Lock()
			exitCode := sess.exitCode
			sess.mu.Unlock()
			tid := todoID
			ec := exitCode
			cmds = append(cmds, func() tea.Msg {
				return agentFinishedMsg{todoID: tid, exitCode: ec}
			})
		default:
			// Still running. Check for status changes from the goroutine.
			sess.mu.Lock()
			sid := sess.SessionID
			status := sess.Status
			question := sess.Question
			sess.mu.Unlock()

			// Propagate session ID to the todo if not yet set.
			if sid != "" && p.cc != nil {
				for _, t := range p.cc.Todos {
					if t.ID == todoID && t.SessionID != sid {
						tid := todoID
						s := sid
						cmds = append(cmds, func() tea.Msg {
							return agentSessionIDMsg{todoID: tid, sessionID: s}
						})
						break
					}
				}
			}

			// Only send blocked status if the in-memory todo status differs.
			if status == "blocked" && p.cc != nil {
				tid := todoID
				q := question
				for _, t := range p.cc.Todos {
					if t.ID == tid && t.SessionStatus != "blocked" {
						cmds = append(cmds, func() tea.Msg {
							return agentStatusMsg{todoID: tid, status: "blocked", question: q}
						})
						break
					}
				}
			}
		}
	}
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}
