package daemon_test

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/daemon"

	tea "github.com/charmbracelet/bubbletea"
)

// mockRunner implements agent.Runner for testing without requiring the claude binary.
type mockRunner struct {
	mu       sync.Mutex
	sessions map[string]*agent.Session
	queue    []agent.Request
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		sessions: make(map[string]*agent.Session),
	}
}

func (m *mockRunner) LaunchOrQueue(req agent.Request) (bool, tea.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess := &agent.Session{
		ID:        req.ID,
		Status:    "processing",
		StartedAt: time.Now(),
	}
	m.sessions[req.ID] = sess
	return false, func() tea.Msg {
		return agent.SessionStartedMsg{ID: req.ID, Session: sess}
	}
}

func (m *mockRunner) Kill(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return false
	}
	delete(m.sessions, id)
	return true
}

func (m *mockRunner) SendMessage(id string, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return &sessionNotFoundError{id}
	}
	return nil
}

func (m *mockRunner) Status(id string) *agent.SessionStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	if !ok {
		return nil
	}
	sess.Mu.Lock()
	defer sess.Mu.Unlock()
	return &agent.SessionStatus{
		ID:        id,
		Status:    sess.Status,
		StartedAt: sess.StartedAt,
	}
}

func (m *mockRunner) Active() []agent.SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []agent.SessionInfo
	for id, sess := range m.sessions {
		sess.Mu.Lock()
		result = append(result, agent.SessionInfo{
			ID:        id,
			Status:    sess.Status,
			StartedAt: sess.StartedAt,
		})
		sess.Mu.Unlock()
	}
	return result
}

func (m *mockRunner) QueueLen() int                              { return 0 }
func (m *mockRunner) Session(id string) *agent.Session           { return nil }
func (m *mockRunner) CheckProcesses() tea.Cmd                    { return nil }
func (m *mockRunner) DrainQueue() (agent.Request, bool)          { return agent.Request{}, false }
func (m *mockRunner) CleanupFinished(id string) *agent.Session   { return nil }
func (m *mockRunner) Watch(id string) tea.Cmd                    { return nil }
func (m *mockRunner) Shutdown()                                  {}

type sessionNotFoundError struct{ id string }

func (e *sessionNotFoundError) Error() string { return "no active session for " + e.id }

func TestLaunchAndListAgents(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	runner := newMockRunner()

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:  filepath.Join(dir, "daemon.sock"),
		DB:          d,
		AgentRunner: runner,
	})
	go srv.Serve()
	defer srv.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// List agents — should be empty
	agents, err := client.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(agents))
	}

	// Launch an agent
	err = client.LaunchAgent(daemon.LaunchAgentParams{
		ID:     "test-agent-1",
		Prompt: "echo hello",
		Dir:    dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Give the launch goroutine a moment to run
	time.Sleep(50 * time.Millisecond)

	// List agents — should show one
	agents, err = client.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].ID != "test-agent-1" {
		t.Fatalf("expected agent ID test-agent-1, got %s", agents[0].ID)
	}
}

func TestAgentStatus(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	runner := newMockRunner()

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:  filepath.Join(dir, "daemon.sock"),
		DB:          d,
		AgentRunner: runner,
	})
	go srv.Serve()
	defer srv.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Status of non-existent agent should return error
	_, err = client.AgentStatus("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %s", err.Error())
	}

	// Launch an agent
	err = client.LaunchAgent(daemon.LaunchAgentParams{
		ID:     "test-agent-2",
		Prompt: "echo hello",
		Dir:    dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	// Query status
	status, err := client.AgentStatus("test-agent-2")
	if err != nil {
		t.Fatal(err)
	}
	if status.ID != "test-agent-2" {
		t.Fatalf("expected agent ID test-agent-2, got %s", status.ID)
	}
	if status.Status != "processing" {
		t.Fatalf("expected status 'processing', got %s", status.Status)
	}
}

func TestStopAgent(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	runner := newMockRunner()

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:  filepath.Join(dir, "daemon.sock"),
		DB:          d,
		AgentRunner: runner,
	})
	go srv.Serve()
	defer srv.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Launch an agent
	err = client.LaunchAgent(daemon.LaunchAgentParams{
		ID:     "test-agent-stop",
		Prompt: "echo hello",
		Dir:    dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	// Stop the agent
	err = client.StopAgent("test-agent-stop")
	if err != nil {
		t.Fatal(err)
	}

	// After stopping, list should be empty
	agents, err := client.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		if a.ID == "test-agent-stop" {
			t.Fatal("expected agent to be removed after stop")
		}
	}

	// Stopping again should fail
	err = client.StopAgent("test-agent-stop")
	if err == nil {
		t.Fatal("expected error stopping already-stopped agent")
	}
}

func TestSendAgentInput(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	runner := newMockRunner()

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:  filepath.Join(dir, "daemon.sock"),
		DB:          d,
		AgentRunner: runner,
	})
	go srv.Serve()
	defer srv.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Send input to non-existent agent should fail
	err = client.SendAgentInput("nonexistent", "hello")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}

	// Launch an agent
	err = client.LaunchAgent(daemon.LaunchAgentParams{
		ID:     "test-agent-input",
		Prompt: "echo hello",
		Dir:    dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	// Send input should succeed
	err = client.SendAgentInput("test-agent-input", "some input")
	if err != nil {
		t.Fatal(err)
	}
}
