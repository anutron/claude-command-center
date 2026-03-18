package commandcenter

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

// agentSession tracks a running headless Claude Code session.
type agentSession struct {
	TodoID    string
	SessionID string // Claude session UUID, captured from first stream-JSON event
	Cmd       *exec.Cmd
	Status    string // "active", "blocked", "review", "failed"
	Question  string // populated when blocked
	StartedAt time.Time
	Output    strings.Builder // captures output
	LogPath   string          // full path to the session log file

	// Stdin pipe for sending messages to the agent (stream-json input).
	Stdin io.WriteCloser

	// Events tracks parsed session events for the live viewer.
	Events   []sessionEvent
	EventsCh chan sessionEvent

	// done is closed when the process exits. ExitCode is set before closing.
	done     chan struct{}
	exitCode int

	// mu protects Status, Question, SessionID, and Events for concurrent writes from the goroutine.
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
	ResumeID   string // If set, resume an existing session instead of starting a new one
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
		args := []string{
			"--print",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
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
		if qs.ResumeID != "" {
			args = append(args, "--resume", qs.ResumeID)
		}
		// Append summary instructions so the agent self-reports what it did.
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

		// Don't pass prompt as positional arg — with --input-format stream-json,
		// the CLI expects the initial prompt via stdin, not as a CLI argument.
		cmd := exec.Command("claude", args...)
		if qs.ProjectDir != "" {
			cmd.Dir = qs.ProjectDir
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			logSessionError(qs.TodoID, "stdout pipe: %v", err)
			return agentFinishedMsg{todoID: qs.TodoID, exitCode: -1}
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			logSessionError(qs.TodoID, "stdin pipe: %v", err)
			return agentFinishedMsg{todoID: qs.TodoID, exitCode: -1}
		}

		if err := cmd.Start(); err != nil {
			logSessionError(qs.TodoID, "start: %v", err)
			return agentFinishedMsg{todoID: qs.TodoID, exitCode: -1}
		}

		// Send the initial prompt via stdin as a stream-json user message.
		initMsg := map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role":    "user",
				"content": enhancedPrompt,
			},
		}
		if initData, err := json.Marshal(initMsg); err == nil {
			initData = append(initData, '\n')
			stdin.Write(initData)
		}

		// Compute the log path deterministically before the goroutine starts
		// so it can be persisted to DB when the session is registered.
		logPath := sessionLogPath(qs.TodoID)

		sess := &agentSession{
			TodoID:    qs.TodoID,
			Cmd:       cmd,
			Status:    "active",
			StartedAt: time.Now(),
			Stdin:     stdin,
			LogPath:   logPath,
			EventsCh:  make(chan sessionEvent, 64),
			done:      make(chan struct{}),
		}

		// Background goroutine: read stream-JSON stdout, detect blocking events,
		// parse events for the live viewer, and signal completion via done channel.
		go func() {
			// Open a log file for forensic replay of this session.
			logFile := openSessionLog(logPath)
			if logFile != nil {
				defer logFile.Close()
			}

			defer func() {
				// Close the events channel before waiting so listeners know
				// no more events will arrive.
				close(sess.EventsCh)

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

				// Log the exit code.
				if logFile != nil {
					fmt.Fprintf(logFile, "\n--- session exited with code %d at %s ---\n", exitCode, time.Now().Format(time.RFC3339))
				}

				close(sess.done)
			}()

			scanner := bufio.NewScanner(stdout)
			// Allow large lines (stream-JSON can be verbose).
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

			sessionIDCaptured := false
			for scanner.Scan() {
				line := scanner.Text()

				// Write raw line to log file.
				if logFile != nil {
					fmt.Fprintln(logFile, line)
				}

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

				// Parse event for the live session viewer.
				parsedEvents := parseSessionEvent(event)
				for _, parsed := range parsedEvents {
					sess.mu.Lock()
					sess.Events = append(sess.Events, parsed)
					sess.mu.Unlock()
					// Non-blocking send to EventsCh — drop if buffer is full.
					select {
					case sess.EventsCh <- parsed:
					default:
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

// killAgent terminates a running agent session for a given todo.
// It sends SIGTERM to the process, closes stdin, and cleans up the session.
// Returns a tea.Cmd for any DB/event side effects, or nil if no session was running.
func (p *Plugin) killAgent(todoID string) tea.Cmd {
	sess, ok := p.activeSessions[todoID]
	if !ok {
		return nil
	}
	// Close stdin so the process knows we're done.
	if sess.Stdin != nil {
		sess.Stdin.Close()
	}
	// Terminate the process.
	if sess.Cmd != nil && sess.Cmd.Process != nil {
		sess.Cmd.Process.Kill()
	}
	delete(p.activeSessions, todoID)

	p.setTodoSessionStatus(todoID, "failed")
	p.publishEvent("agent.killed", map[string]interface{}{
		"todo_id": todoID,
	})

	// If the session viewer is watching this session, mark it done.
	if p.sessionViewerActive && p.sessionViewerTodoID == todoID {
		p.sessionViewerDone = true
		p.sessionViewerListening = false
		p.updateSessionViewerContent()
	}

	return p.persistSessionStatus(todoID, "failed")
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

	// Check if the agent already submitted a summary via `ccc update-todo`.
	// That command writes directly to the DB, so we reload the todo to check.
	if p.database != nil {
		if dbTodo, err := db.DBLoadTodoByID(p.database, todoID); err == nil && dbTodo != nil && dbTodo.SessionSummary != "" {
			summary = dbTodo.SessionSummary
		}
	}

	if sess, ok := p.activeSessions[todoID]; ok {
		// Fall back to extraction from stream output if no summary was submitted.
		if summary == "" {
			summary = extractSessionSummary(sess)
		}
		// Close stdin pipe if still open.
		if sess.Stdin != nil {
			sess.Stdin.Close()
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

// sendUserMessage writes an NDJSON user message to the agent's stdin pipe.
// The format follows the Claude CLI stream-json input protocol.
func sendUserMessage(sess *agentSession, message string) error {
	if sess.Stdin == nil {
		return fmt.Errorf("session stdin is not available")
	}
	payload := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": message,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal user message: %w", err)
	}
	data = append(data, '\n')
	_, err = sess.Stdin.Write(data)
	if err != nil {
		return fmt.Errorf("write to agent stdin: %w", err)
	}
	// Clear blocked status since we've responded.
	sess.mu.Lock()
	if sess.Status == "blocked" {
		sess.Status = "active"
		sess.Question = ""
	}
	sess.mu.Unlock()
	return nil
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

// sessionLogDir returns the directory for session log files.
func sessionLogDir() string {
	return filepath.Join(config.DataDir(), "session-logs")
}

// logSessionError writes a one-line error to the session log directory for
// launch failures that happen before the goroutine starts.
func logSessionError(todoID string, format string, args ...interface{}) {
	dir := sessionLogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := fmt.Sprintf("%s_%s.jsonl", time.Now().Format("2006-01-02T15-04-05"), todoID)
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "--- LAUNCH ERROR at %s: %s ---\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// sessionLogPath returns the deterministic full path for a session log file.
// This is computed before the goroutine starts so the path can be persisted to DB.
func sessionLogPath(todoID string) string {
	dir := sessionLogDir()
	name := fmt.Sprintf("%s_%s.jsonl", time.Now().Format("2006-01-02T15-04-05"), todoID)
	return filepath.Join(dir, name)
}

// openSessionLog creates a log file at the given path for a headless session's
// raw stream-json output.
// Returns nil if the file cannot be created (non-fatal — logging is best-effort).
func openSessionLog(path string) *os.File {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil
	}
	fmt.Fprintf(f, "--- session started at %s ---\n", time.Now().Format(time.RFC3339))
	return f
}
