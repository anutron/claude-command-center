package daemon_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
)

func TestListAgentHistory_Empty(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath: filepath.Join(dir, "s"),
		DB:         d,
	})
	go srv.Serve()
	defer srv.Shutdown()
	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	entries, err := client.ListAgentHistory(24)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestListAgentHistory_DefaultWindow(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath: filepath.Join(dir, "s"),
		DB:         d,
	})
	go srv.Serve()
	defer srv.Shutdown()
	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// windowHours=0 should default to 24 hours — no error expected
	_, err = client.ListAgentHistory(0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamAgentOutput_NoRunner(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)

	// Server with no agent runner — all sessions are considered done.
	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath: filepath.Join(dir, "s"),
		DB:         d,
	})
	go srv.Serve()
	defer srv.Shutdown()
	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	result, err := client.StreamAgentOutput("nonexistent-agent")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatal("expected Done=true when no runner is configured")
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected no events, got %d", len(result.Events))
	}
}

func TestStreamAgentOutput_AgentNotFound(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	runner := newMockRunner()

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:  filepath.Join(dir, "s"),
		DB:          d,
		AgentRunner: runner,
	})
	go srv.Serve()
	defer srv.Shutdown()
	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Agent not in the runner — session already cleaned up.
	result, err := client.StreamAgentOutput("ghost-agent")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatal("expected Done=true when agent session is not found")
	}
}

func TestStreamAgentOutput_MissingAgentID(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	runner := newMockRunner()

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:  filepath.Join(dir, "s"),
		DB:          d,
		AgentRunner: runner,
	})
	go srv.Serve()
	defer srv.Shutdown()
	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.StreamAgentOutput("")
	if err == nil {
		t.Fatal("expected error for empty agent_id")
	}
}
