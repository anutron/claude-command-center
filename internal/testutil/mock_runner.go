package testutil

import (
	"fmt"
	"strings"
	"sync"

	"github.com/anutron/claude-command-center/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
)

// MockRunner implements agent.Runner for testing. It tracks launches,
// simulates sessions, and allows tests to control agent state.
type MockRunner struct {
	mu       sync.Mutex
	sessions map[string]*agent.Session
	queue    []agent.Request
	max      int
	launches []agent.Request // all launch requests received
}

// NewMockRunner creates a mock runner with the given concurrency limit.
func NewMockRunner(maxConcurrent int) *MockRunner {
	return &MockRunner{
		sessions: make(map[string]*agent.Session),
		max:      maxConcurrent,
	}
}

// AddSession injects a fake running session for testing.
func (r *MockRunner) AddSession(id string, sess *agent.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[id] = sess
}

// RemoveSession removes a session (simulates completion).
func (r *MockRunner) RemoveSession(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// Launches returns all launch requests received.
func (r *MockRunner) Launches() []agent.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]agent.Request{}, r.launches...)
}

// NewFakeSession creates a minimal Session with an events channel for testing.
func NewFakeSession(id string) *agent.Session {
	return &agent.Session{
		ID:       id,
		Status:   "processing",
		EventsCh: make(chan agent.SessionEvent, 100),
	}
}

// --- agent.Runner interface ---

func (r *MockRunner) LaunchOrQueue(req agent.Request) (bool, tea.Cmd) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.launches = append(r.launches, req)

	if len(r.sessions) >= r.max {
		r.queue = append(r.queue, req)
		return true, nil
	}
	sess := &agent.Session{
		ID:       req.ID,
		Status:   "processing",
		EventsCh: make(chan agent.SessionEvent, 100),
	}
	r.sessions[req.ID] = sess
	return false, func() tea.Msg {
		return agent.SessionStartedMsg{ID: req.ID}
	}
}

func (r *MockRunner) Kill(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; ok {
		delete(r.sessions, id)
		return true
	}
	return false
}

func (r *MockRunner) SendMessage(id string, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; !ok {
		return fmt.Errorf("session %s not found", id)
	}
	return nil
}

func (r *MockRunner) Status(id string) *agent.SessionStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sess, ok := r.sessions[id]; ok {
		return &agent.SessionStatus{
			ID:        sess.ID,
			Status:    sess.Status,
			SessionID: sess.SessionID,
			Question:  sess.Question,
			StartedAt: sess.StartedAt,
		}
	}
	// Check queue
	for _, req := range r.queue {
		if req.ID == id {
			return &agent.SessionStatus{ID: id, Status: "queued"}
		}
	}
	return nil
}

func (r *MockRunner) Active() []agent.SessionInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	var infos []agent.SessionInfo
	for _, sess := range r.sessions {
		infos = append(infos, agent.SessionInfo{
			ID:        sess.ID,
			Status:    sess.Status,
			SessionID: sess.SessionID,
			StartedAt: sess.StartedAt,
		})
	}
	return infos
}

func (r *MockRunner) QueueLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.queue)
}

func (r *MockRunner) Session(id string) *agent.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[id]
}

func (r *MockRunner) CheckProcesses() tea.Cmd { return nil }

func (r *MockRunner) DrainQueue() (agent.Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		return agent.Request{}, false
	}
	req := r.queue[0]
	r.queue = r.queue[1:]
	return req, true
}

func (r *MockRunner) CleanupFinished(id string) *agent.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	sess := r.sessions[id]
	delete(r.sessions, id)
	return sess
}

func (r *MockRunner) Watch(id string) tea.Cmd { return nil }

func (r *MockRunner) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = make(map[string]*agent.Session)
	r.queue = nil
}

// Verify MockRunner implements Runner at compile time.
var _ agent.Runner = (*MockRunner)(nil)

// Suppress unused import warning for strings.
var _ = strings.Builder{}
