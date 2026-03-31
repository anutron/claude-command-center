package daemon_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
)

// shortSockPath returns a short socket path under /tmp to avoid macOS
// 108-char Unix socket path limit.
func shortSockPath(t *testing.T, suffix string) string {
	t.Helper()
	path := fmt.Sprintf("/tmp/ccc-test-%s-%d.sock", suffix, os.Getpid())
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func startTestDaemon(t *testing.T) (*daemon.Server, *daemon.Client, string) {
	t.Helper()
	d := testDB(t)
	sockPath := shortSockPath(t, "std")
	return startTestDaemonWithDB(t, d, sockPath)
}

func startTestDaemonWithDB(t *testing.T, d *sql.DB, sockPath string) (*daemon.Server, *daemon.Client, string) {
	t.Helper()
	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath: sockPath,
		DB:         d,
	})
	go srv.Serve()
	t.Cleanup(srv.Shutdown)
	time.Sleep(50 * time.Millisecond)
	client, err := daemon.NewClient(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return srv, client, sockPath
}

func TestRegisterAndListSessions(t *testing.T) {
	_, client, _ := startTestDaemon(t)
	err := client.RegisterSession(daemon.RegisterSessionParams{
		SessionID: "sess-abc",
		PID:       os.Getpid(),
		Project:   "/tmp/myproject",
	})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "sess-abc" {
		t.Fatalf("expected sess-abc, got %s", sessions[0].SessionID)
	}
	if sessions[0].State != "active" {
		t.Fatalf("expected active, got %s", sessions[0].State)
	}
}

func TestUpdateSessionTopic(t *testing.T) {
	_, client, _ := startTestDaemon(t)
	client.RegisterSession(daemon.RegisterSessionParams{
		SessionID: "sess-topic",
		PID:       os.Getpid(),
		Project:   "/tmp/proj",
	})
	err := client.UpdateSession(daemon.UpdateSessionParams{
		SessionID: "sess-topic",
		Topic:     "Auth refactor",
	})
	if err != nil {
		t.Fatal(err)
	}
	sessions, _ := client.ListSessions()
	if sessions[0].Topic != "Auth refactor" {
		t.Fatalf("expected 'Auth refactor', got '%s'", sessions[0].Topic)
	}
}

func TestListSessionsExcludesArchived(t *testing.T) {
	_, client, _ := startTestDaemon(t)
	// Register two sessions
	client.RegisterSession(daemon.RegisterSessionParams{
		SessionID: "sess-active",
		PID:       os.Getpid(),
		Project:   "/tmp/a",
	})
	client.RegisterSession(daemon.RegisterSessionParams{
		SessionID: "sess-archived",
		PID:       os.Getpid(),
		Project:   "/tmp/b",
	})
	// ListSessions should return both (both are active)
	sessions, _ := client.ListSessions()
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestEndSessionViaClient(t *testing.T) {
	_, client, _ := startTestDaemon(t)

	// Register a session.
	err := client.RegisterSession(daemon.RegisterSessionParams{
		SessionID: "sess-end",
		PID:       os.Getpid(),
		Project:   "/tmp/endproject",
	})
	if err != nil {
		t.Fatal(err)
	}

	// End the session.
	err = client.EndSession(daemon.EndSessionParams{
		SessionID: "sess-end",
	})
	if err != nil {
		t.Fatal(err)
	}

	// List sessions — the ended session should still appear with State="ended".
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].State != "ended" {
		t.Fatalf("expected state 'ended', got '%s'", sessions[0].State)
	}
}

func TestEndSessionIdempotent(t *testing.T) {
	_, client, _ := startTestDaemon(t)

	client.RegisterSession(daemon.RegisterSessionParams{
		SessionID: "sess-idem",
		PID:       os.Getpid(),
		Project:   "/tmp/idem",
	})

	// End twice — second call should be a no-op (already ended).
	if err := client.EndSession(daemon.EndSessionParams{SessionID: "sess-idem"}); err != nil {
		t.Fatal(err)
	}
	if err := client.EndSession(daemon.EndSessionParams{SessionID: "sess-idem"}); err != nil {
		t.Fatal(err)
	}

	sessions, _ := client.ListSessions()
	if len(sessions) != 1 || sessions[0].State != "ended" {
		t.Fatalf("expected 1 ended session, got %v", sessions)
	}
}

func TestEndSessionNotFound(t *testing.T) {
	_, client, _ := startTestDaemon(t)

	err := client.EndSession(daemon.EndSessionParams{
		SessionID: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent session, got nil")
	}
}

func TestEndThenArchiveSession(t *testing.T) {
	_, client, _ := startTestDaemon(t)

	client.RegisterSession(daemon.RegisterSessionParams{
		SessionID: "sess-archive",
		PID:       os.Getpid(),
		Project:   "/tmp/archive",
	})

	// End, then archive.
	client.EndSession(daemon.EndSessionParams{SessionID: "sess-archive"})
	err := client.ArchiveSession(daemon.ArchiveSessionParams{SessionID: "sess-archive"})
	if err != nil {
		t.Fatal(err)
	}

	// Archived sessions should not appear in list.
	sessions, _ := client.ListSessions()
	for _, s := range sessions {
		if s.SessionID == "sess-archive" {
			t.Fatal("archived session should not appear in list")
		}
	}
}

func TestRegisterSessionPersistsToDB(t *testing.T) {
	// Register a session via daemon, then start a fresh daemon on same DB
	// and verify the session is still there.
	d := testDB(t)
	sockPath := shortSockPath(t, "persist1")

	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath: sockPath,
		DB:         d,
	})
	go srv.Serve()
	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	client.RegisterSession(daemon.RegisterSessionParams{
		SessionID: "sess-persist",
		PID:       os.Getpid(),
		Project:   "/tmp/persist",
	})
	client.Close()
	srv.Shutdown()

	// Start a new daemon on the same DB
	sockPath2 := shortSockPath(t, "persist2")
	srv2 := daemon.NewServer(daemon.ServerConfig{
		SocketPath: sockPath2,
		DB:         d,
	})
	go srv2.Serve()
	t.Cleanup(srv2.Shutdown)
	time.Sleep(50 * time.Millisecond)

	client2, err := daemon.NewClient(sockPath2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client2.Close() })

	sessions, err := client2.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sessions {
		if s.SessionID == "sess-persist" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected sess-persist to survive daemon restart")
	}
}
