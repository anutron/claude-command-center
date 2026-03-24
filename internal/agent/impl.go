package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/creack/pty"
	"github.com/google/uuid"

	tea "github.com/charmbracelet/bubbletea"
)

// defaultRunner is the concrete implementation of Runner.
type defaultRunner struct {
	mu             sync.Mutex
	maxConcurrent  int
	activeSessions map[string]*Session
	sessionQueue   []Request
}

// NewRunner creates a new Runner with the given concurrency limit.
func NewRunner(maxConcurrent int) Runner {
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}
	return &defaultRunner{
		maxConcurrent:  maxConcurrent,
		activeSessions: make(map[string]*Session),
	}
}

// canLaunch must be called with r.mu held.
func (r *defaultRunner) canLaunch() bool {
	return len(r.activeSessions) < r.maxConcurrent
}

func (r *defaultRunner) LaunchOrQueue(req Request) (queued bool, cmd tea.Cmd) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Dedup: reject if this ID is already active or queued.
	if _, active := r.activeSessions[req.ID]; active {
		return false, nil
	}
	for _, q := range r.sessionQueue {
		if q.ID == req.ID {
			return false, nil
		}
	}

	if r.canLaunch() {
		return false, r.launchSession(req)
	}
	r.sessionQueue = append(r.sessionQueue, req)
	return true, nil
}

func (r *defaultRunner) Kill(id string) bool {
	r.mu.Lock()
	sess, ok := r.activeSessions[id]
	if !ok {
		r.mu.Unlock()
		return false
	}
	delete(r.activeSessions, id)
	r.mu.Unlock()
	// Close PTY first (sends SIGHUP to child process group).
	if sess.Pty != nil {
		sess.Pty.Close()
	}
	if sess.Cmd != nil && sess.Cmd.Process != nil {
		sess.Cmd.Process.Kill()
	}
	return true
}

func (r *defaultRunner) SendMessage(id string, message string) error {
	r.mu.Lock()
	sess, ok := r.activeSessions[id]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("no active session for %s", id)
	}
	return SendUserMessage(sess, message)
}

func (r *defaultRunner) Status(id string) *SessionStatus {
	r.mu.Lock()
	sess, ok := r.activeSessions[id]
	if !ok {
		// Check queue
		for _, req := range r.sessionQueue {
			if req.ID == id {
				r.mu.Unlock()
				return &SessionStatus{
					ID:     id,
					Status: "queued",
				}
			}
		}
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	sess.Mu.Lock()
	defer sess.Mu.Unlock()
	return &SessionStatus{
		ID:        id,
		Status:    sess.Status,
		SessionID: sess.SessionID,
		Question:  sess.Question,
		StartedAt: sess.StartedAt,
	}
}

func (r *defaultRunner) Active() []SessionInfo {
	r.mu.Lock()
	result := make([]SessionInfo, 0, len(r.activeSessions))
	for id, sess := range r.activeSessions {
		sess.Mu.Lock()
		info := SessionInfo{
			ID:        id,
			Status:    sess.Status,
			SessionID: sess.SessionID,
			StartedAt: sess.StartedAt,
		}
		sess.Mu.Unlock()
		result = append(result, info)
	}
	r.mu.Unlock()
	return result
}

func (r *defaultRunner) QueueLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sessionQueue)
}

func (r *defaultRunner) Session(id string) *Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeSessions[id]
}

func (r *defaultRunner) DrainQueue() (Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sessionQueue) == 0 || !r.canLaunch() {
		return Request{}, false
	}
	next := r.sessionQueue[0]
	r.sessionQueue = r.sessionQueue[1:]
	return next, true
}

func (r *defaultRunner) CheckProcesses() tea.Cmd {
	r.mu.Lock()
	defer r.mu.Unlock()
	var cmds []tea.Cmd
	for id, sess := range r.activeSessions {
		select {
		case <-sess.done:
			sess.Mu.Lock()
			exitCode := sess.exitCode
			sid := sess.SessionID
			sess.Mu.Unlock()
			capturedID := id
			capturedEC := exitCode
			// Emit SessionIDCapturedMsg before SessionFinishedMsg so the
			// session ID is persisted even when the agent finishes before
			// CheckProcesses runs during the "running" phase.
			if sid != "" {
				capturedSID := sid
				cmds = append(cmds, func() tea.Msg {
					return SessionIDCapturedMsg{ID: capturedID, SessionID: capturedSID}
				})
			}
			cmds = append(cmds, func() tea.Msg {
				return SessionFinishedMsg{ID: capturedID, ExitCode: capturedEC}
			})
		default:
			sess.Mu.Lock()
			sid := sess.SessionID
			status := sess.Status
			question := sess.Question
			sess.Mu.Unlock()

			capturedID := id
			if sid != "" {
				capturedSID := sid
				cmds = append(cmds, func() tea.Msg {
					return SessionIDCapturedMsg{ID: capturedID, SessionID: capturedSID}
				})
			}
			if status == "blocked" {
				capturedQ := question
				cmds = append(cmds, func() tea.Msg {
					return SessionBlockedMsg{ID: capturedID, Question: capturedQ}
				})
			}
		}
	}
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

func (r *defaultRunner) Watch(id string) tea.Cmd {
	r.mu.Lock()
	sess, ok := r.activeSessions[id]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return ListenForSessionEvent(id, sess.EventsCh)
}

func (r *defaultRunner) Shutdown() {
	r.mu.Lock()
	sessions := make([]*Session, 0, len(r.activeSessions))
	for _, sess := range r.activeSessions {
		sessions = append(sessions, sess)
	}
	r.mu.Unlock()

	for _, sess := range sessions {
		// Close PTY first (sends SIGHUP), then SIGINT for good measure.
		if sess.Pty != nil {
			sess.Pty.Close()
		}
		if sess.Cmd != nil && sess.Cmd.Process != nil {
			sess.Cmd.Process.Signal(syscall.SIGINT)
		}
	}
	for _, sess := range sessions {
		if sess.done != nil {
			select {
			case <-sess.done:
			case <-time.After(3 * time.Second):
			}
		}
	}
}

// OnSessionFinished should be called by the host when it receives a
// SessionFinishedMsg, to clean up the session from the active map.
// Returns the finished session (for summary extraction) or nil.
func (r *defaultRunner) onSessionFinished(id string) *Session {
	r.mu.Lock()
	sess, ok := r.activeSessions[id]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.activeSessions, id)
	r.mu.Unlock()
	if sess.Pty != nil {
		sess.Pty.Close()
	}
	return sess
}

// CleanupFinished removes a finished session from the active map and returns it.
// This is the public entry point for the host to call after receiving SessionFinishedMsg.
func (r *defaultRunner) CleanupFinished(id string) *Session {
	return r.onSessionFinished(id)
}

// launchSession starts a headless Claude Code session via PTY and returns a
// tea.Cmd that sends SessionStartedMsg.
func (r *defaultRunner) launchSession(req Request) tea.Cmd {
	return func() tea.Msg {
		// Generate session ID upfront so we can immediately report it and
		// know the native log path before the process starts.
		sessionUUID := uuid.New().String()

		args := []string{"--verbose"}

		if req.ResumeID != "" {
			args = append(args, "--resume", req.ResumeID)
		} else {
			args = append(args, "--session-id", sessionUUID)
		}

		if req.Permission != "" && req.Permission != "default" {
			args = append(args, "--permission-mode", req.Permission)
		}
		if req.Worktree {
			args = append(args, "--worktree")
		}

		cmd := exec.Command("claude", args...)
		if req.ProjectDir != "" {
			cmd.Dir = req.ProjectDir
		}

		// Launch via PTY instead of pipes.
		ptmx, err := pty.Start(cmd)
		if err != nil {
			LogSessionError(req.ID, "pty start: %v", err)
			return SessionFinishedMsg{ID: req.ID, ExitCode: -1}
		}

		// Drain PTY output to prevent the child process from blocking on
		// a full PTY buffer. We read events from the native log instead.
		go io.Copy(io.Discard, ptmx)

		// Write the initial prompt as plain text to the PTY.
		if req.Prompt != "" {
			ptmx.Write([]byte(req.Prompt + "\n"))
		}

		logPath := SessionLogPath(req.ID)

		// Determine the project dir for native log path computation.
		projectDir := req.ProjectDir
		if projectDir == "" {
			projectDir, _ = os.Getwd()
		}

		sess := &Session{
			ID:        req.ID,
			SessionID: sessionUUID,
			Cmd:       cmd,
			Status:    "processing",
			StartedAt: time.Now(),
			Pty:       ptmx,
			LogPath:   logPath,
			EventsCh:  make(chan SessionEvent, 64),
			done:      make(chan struct{}),
			output:    &strings.Builder{},
		}

		// Register the session in the active map so Status/Active/Kill work.
		r.mu.Lock()
		r.activeSessions[req.ID] = sess
		r.mu.Unlock()

		// Tail the native log file for events, cost tracking, and status detection.
		nativeLogPath := NativeLogPath(projectDir, sessionUUID)
		go monitorSessionFromLog(sess, nativeLogPath, req.CostCallback, req.Budget)

		return SessionStartedMsg{
			ID:      req.ID,
			Session: sess,
		}
	}
}

// monitorSessionFromLog tails the Claude native JSONL log file for events,
// cost tracking, and session status. It replaces the old pipe-based monitorSession.
func monitorSessionFromLog(sess *Session, nativeLogPath string, costCb CostCallback, budgetUSD float64) {
	logFile := OpenSessionLog(sess.LogPath)
	if logFile != nil {
		defer logFile.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh := make(chan map[string]interface{}, 64)
	go tailNativeLog(ctx, nativeLogPath, 0, eventCh)

	// Track cumulative cost for per-session budget enforcement.
	var cumulativeCost float64

	// processDone signals when the child process exits.
	processDone := make(chan struct{})
	go func() {
		defer close(processDone)
		_ = sess.Cmd.Wait()
	}()

	defer func() {
		cancel() // stop log tailer

		close(sess.EventsCh)

		exitCode := 0
		// Wait for process to finish (may already be done).
		<-processDone
		if sess.Cmd.ProcessState != nil && !sess.Cmd.ProcessState.Success() {
			exitCode = sess.Cmd.ProcessState.ExitCode()
		}

		sess.Mu.Lock()
		sess.exitCode = exitCode
		sess.Mu.Unlock()

		if logFile != nil {
			fmt.Fprintf(logFile, "\n--- session exited with code %d at %s ---\n", exitCode, time.Now().Format(time.RFC3339))
		}

		close(sess.done)
	}()

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return
			}

			// Serialize to JSON for CCC's own log file and output buffer.
			line, _ := json.Marshal(event)
			lineStr := string(line)

			if logFile != nil {
				fmt.Fprintln(logFile, lineStr)
			}

			sess.Mu.Lock()
			sess.output.WriteString(lineStr)
			sess.output.WriteString("\n")
			sess.Mu.Unlock()

			// Parse into SessionEvents for the live viewer.
			parsedEvents := ParseSessionEvent(event)
			for _, parsed := range parsedEvents {
				sess.Mu.Lock()
				sess.Events = append(sess.Events, parsed)
				sess.Mu.Unlock()
				select {
				case sess.EventsCh <- parsed:
				default:
				}
			}

			// Detect blocking (permission prompts, user questions).
			if DetectBlockingEvent(event) {
				question := ExtractBlockingQuestion(event)
				sess.Mu.Lock()
				sess.Status = "blocked"
				sess.Question = question
				sess.Mu.Unlock()
			}

			// Extract token usage and invoke cost callback.
			if inputTok, outputTok, hasUsage := extractUsageFromEvent(event); hasUsage {
				cost := estimateCost(event, inputTok, outputTok)
				cumulativeCost += cost
				if costCb != nil {
					costCb(inputTok, outputTok, cost)
				}

				// Per-session budget enforcement: if cumulative cost exceeds
				// the budget, send SIGINT to gracefully stop the agent.
				if budgetUSD > 0 && cumulativeCost > budgetUSD {
					if sess.Cmd != nil && sess.Cmd.Process != nil {
						sess.Cmd.Process.Signal(syscall.SIGINT)
					}
				}
			}

		case <-processDone:
			// Process exited. Drain remaining events from the log with a short deadline.
			drainCtx, drainCancel := context.WithTimeout(context.Background(), 2*time.Second)
		drain:
			for {
				select {
				case event, ok := <-eventCh:
					if !ok {
						break drain
					}
					line, _ := json.Marshal(event)
					lineStr := string(line)
					if logFile != nil {
						fmt.Fprintln(logFile, lineStr)
					}
					sess.Mu.Lock()
					sess.output.WriteString(lineStr)
					sess.output.WriteString("\n")
					sess.Mu.Unlock()

					parsedEvents := ParseSessionEvent(event)
					for _, parsed := range parsedEvents {
						sess.Mu.Lock()
						sess.Events = append(sess.Events, parsed)
						sess.Mu.Unlock()
						select {
						case sess.EventsCh <- parsed:
						default:
						}
					}

					if inputTok, outputTok, hasUsage := extractUsageFromEvent(event); hasUsage {
						cost := estimateCost(event, inputTok, outputTok)
						cumulativeCost += cost
						if costCb != nil {
							costCb(inputTok, outputTok, cost)
						}
					}
				case <-drainCtx.Done():
					break drain
				}
			}
			drainCancel()
			return
		}
	}
}

// SendUserMessage writes a plain-text user message to the agent's PTY.
func SendUserMessage(sess *Session, message string) error {
	if sess.Pty == nil {
		return fmt.Errorf("session PTY is not available")
	}
	_, err := sess.Pty.Write([]byte(message + "\n"))
	if err != nil {
		return fmt.Errorf("write to agent PTY: %w", err)
	}
	sess.Mu.Lock()
	if sess.Status == "blocked" {
		sess.Status = "processing"
		sess.Question = ""
	}
	sess.Mu.Unlock()
	return nil
}

// DetectBlockingEvent returns true if a stream-JSON event indicates the agent
// is waiting for user input.
func DetectBlockingEvent(event map[string]interface{}) bool {
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

// ExtractBlockingQuestion tries to extract the question text from a blocking event.
func ExtractBlockingQuestion(event map[string]interface{}) string {
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

// ParseSessionEvent maps a raw stream-json event to one or more SessionEvents.
func ParseSessionEvent(raw map[string]interface{}) []SessionEvent {
	eventType, _ := raw["type"].(string)

	switch eventType {
	case "assistant":
		content := extractContentArray(raw)
		if content == nil {
			return nil
		}
		var events []SessionEvent
		for _, block := range content {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "text":
				text, _ := blockMap["text"].(string)
				events = append(events, SessionEvent{
					Type: "assistant_text",
					Text: text,
				})
			case "tool_use":
				ev := SessionEvent{
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
		ev := SessionEvent{Type: "tool_result"}
		ev.ResultToolID, _ = raw["tool_use_id"].(string)
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
		return []SessionEvent{ev}

	case "result":
		ev := SessionEvent{Type: "assistant_text"}
		switch r := raw["result"].(type) {
		case string:
			ev.Text = r
		case map[string]interface{}:
			ev.Text = ExtractTextFromContent(r)
		}
		return []SessionEvent{ev}

	case "error":
		ev := SessionEvent{Type: "error", IsError: true}
		if errObj, ok := raw["error"].(map[string]interface{}); ok {
			ev.Text, _ = errObj["message"].(string)
		}
		if ev.Text == "" {
			ev.Text, _ = raw["message"].(string)
		}
		return []SessionEvent{ev}

	case "user":
		if msg, ok := raw["message"].(map[string]interface{}); ok {
			switch c := msg["content"].(type) {
			case string:
				if c != "" {
					return []SessionEvent{{Type: "user", Text: c}}
				}
			case []interface{}:
				var events []SessionEvent
				for _, block := range c {
					bm, ok := block.(map[string]interface{})
					if !ok {
						continue
					}
					switch bm["type"] {
					case "text":
						if t, ok := bm["text"].(string); ok && t != "" {
							events = append(events, SessionEvent{Type: "user", Text: t})
						}
					case "tool_result":
						ev := SessionEvent{Type: "tool_result"}
						ev.ResultToolID, _ = bm["tool_use_id"].(string)
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
		return nil

	case "system":
		ev := SessionEvent{Type: "system"}
		ev.Text, _ = raw["message"].(string)
		if ev.Text == "" {
			if subtype, ok := raw["subtype"].(string); ok && subtype != "" {
				ev.Text = subtype
			} else if sid, ok := raw["session_id"].(string); ok && sid != "" {
				ev.Text = "session " + sid[:min(8, len(sid))]
			}
		}
		if ev.Text == "" {
			return nil
		}
		return []SessionEvent{ev}
	}

	return nil
}

// ExtractTextFromContent extracts text from a stream-JSON event's content array.
func ExtractTextFromContent(event map[string]interface{}) string {
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

// ExtractSessionSummary extracts a human-readable summary from a session's output.
func ExtractSessionSummary(sess *Session) string {
	sess.Mu.Lock()
	output := sess.output.String()
	exitCode := sess.exitCode
	sess.Mu.Unlock()

	if output == "" {
		if exitCode == 0 {
			return "Session completed successfully."
		}
		return fmt.Sprintf("Session exited with code %d.", exitCode)
	}

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
			if r, ok := event["result"].(string); ok && r != "" {
				resultText = r
			}
			if msg, ok := event["result"].(map[string]interface{}); ok {
				if text := ExtractTextFromContent(msg); text != "" {
					resultText = text
				}
			}
		case "assistant":
			if text := ExtractTextFromContent(event); text != "" {
				lastAssistantText = text
			}
		case "message":
			if role, _ := event["role"].(string); role == "assistant" {
				if text := ExtractTextFromContent(event); text != "" {
					lastAssistantText = text
				}
			}
		}
	}

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

	const maxLen = 1000
	if len(summary) > maxLen {
		summary = summary[:maxLen]
		if idx := strings.LastIndex(summary, "\n"); idx > maxLen/2 {
			summary = summary[:idx]
		}
		summary += "\n..."
	}
	return strings.TrimSpace(summary)
}

// extractContentArray gets the content array from a stream-json event.
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

// truncateToolInput returns a short string representation of tool input.
func truncateToolInput(input map[string]interface{}) string {
	const maxLen = 80
	s := fmt.Sprintf("%v", input)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// SessionLogDir returns the directory for session log files.
func SessionLogDir() string {
	return filepath.Join(config.DataDir(), "session-logs")
}

// SessionLogPath returns the deterministic full path for a session log file.
func SessionLogPath(id string) string {
	dir := SessionLogDir()
	name := fmt.Sprintf("%s_%s.jsonl", time.Now().Format("2006-01-02T15-04-05"), id)
	return filepath.Join(dir, name)
}

// LogSessionError writes a one-line error to the session log directory.
func LogSessionError(id string, format string, args ...interface{}) {
	dir := SessionLogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := fmt.Sprintf("%s_%s.jsonl", time.Now().Format("2006-01-02T15-04-05"), id)
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "--- LAUNCH ERROR at %s: %s ---\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// OpenSessionLog creates a log file at the given path.
func OpenSessionLog(path string) *os.File {
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

// ListenForSessionEvent returns a tea.Cmd that blocks on the event channel.
func ListenForSessionEvent(id string, ch <-chan SessionEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return SessionEventsDoneMsg{ID: id}
		}
		return SessionEventMsg{ID: id, Event: ev}
	}
}
