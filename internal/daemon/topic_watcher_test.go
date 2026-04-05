package daemon

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func twTestDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestTopicWatcher_ScanPicksUpTopicFromFile(t *testing.T) {
	database := twTestDB(t)

	reg := newSessionRegistry(database)

	// Register a session with PID 12345.
	if err := reg.register(RegisterSessionParams{
		SessionID: "ccc-sess-001",
		PID:       12345,
		Project:   "/tmp/test-project",
	}); err != nil {
		t.Fatal(err)
	}

	// Create a temp directory to act as ~/.claude/session-topics/
	dir := t.TempDir()

	// Write a pid map file: pid-12345.map → claude-uuid-abc
	os.WriteFile(filepath.Join(dir, "pid-12345.map"), []byte("claude-uuid-abc"), 0644)

	// Write a topic file: claude-uuid-abc.txt → "Fixing the widget"
	os.WriteFile(filepath.Join(dir, "claude-uuid-abc.txt"), []byte("Fixing the widget"), 0644)

	tw := &topicWatcher{
		dir:      dir,
		registry: reg,
		lastSeen: make(map[string]string),
	}

	updated := tw.scan()
	if len(updated) != 1 {
		t.Fatalf("expected 1 update, got %d: %v", len(updated), updated)
	}

	// Verify the session topic was updated.
	sessions := reg.list()
	var found bool
	for _, s := range sessions {
		if s.SessionID == "ccc-sess-001" {
			if s.Topic != "Fixing the widget" {
				t.Errorf("topic = %q, want %q", s.Topic, "Fixing the widget")
			}
			found = true
		}
	}
	if !found {
		t.Error("session ccc-sess-001 not found in registry")
	}
}

func TestTopicWatcher_SkipsUnchangedTopics(t *testing.T) {
	database := twTestDB(t)
	reg := newSessionRegistry(database)

	reg.register(RegisterSessionParams{
		SessionID: "ccc-sess-002",
		PID:       22222,
		Project:   "/tmp/test",
	})

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pid-22222.map"), []byte("claude-uuid-def"), 0644)
	os.WriteFile(filepath.Join(dir, "claude-uuid-def.txt"), []byte("Initial topic"), 0644)

	tw := &topicWatcher{
		dir:      dir,
		registry: reg,
		lastSeen: make(map[string]string),
	}

	// First scan should pick it up.
	updated := tw.scan()
	if len(updated) != 1 {
		t.Fatalf("first scan: expected 1 update, got %d", len(updated))
	}

	// Second scan with same content should skip.
	updated = tw.scan()
	if len(updated) != 0 {
		t.Fatalf("second scan: expected 0 updates, got %d", len(updated))
	}

	// Change the topic.
	os.WriteFile(filepath.Join(dir, "claude-uuid-def.txt"), []byte("Updated topic"), 0644)
	updated = tw.scan()
	if len(updated) != 1 {
		t.Fatalf("third scan: expected 1 update, got %d", len(updated))
	}
}

func TestTopicWatcher_IgnoresUnknownPIDs(t *testing.T) {
	database := twTestDB(t)
	reg := newSessionRegistry(database)

	dir := t.TempDir()
	// Write files for a PID that has no registered session.
	os.WriteFile(filepath.Join(dir, "pid-99999.map"), []byte("orphan-uuid"), 0644)
	os.WriteFile(filepath.Join(dir, "orphan-uuid.txt"), []byte("Orphan topic"), 0644)

	tw := &topicWatcher{
		dir:      dir,
		registry: reg,
		lastSeen: make(map[string]string),
	}

	updated := tw.scan()
	if len(updated) != 0 {
		t.Fatalf("expected 0 updates for unknown PID, got %d", len(updated))
	}
}

func TestTopicWatcher_OnUpdateCallback(t *testing.T) {
	database := twTestDB(t)
	reg := newSessionRegistry(database)

	reg.register(RegisterSessionParams{
		SessionID: "ccc-sess-003",
		PID:       33333,
		Project:   "/tmp/test",
	})

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pid-33333.map"), []byte("claude-uuid-ghi"), 0644)
	os.WriteFile(filepath.Join(dir, "claude-uuid-ghi.txt"), []byte("Callback test"), 0644)

	callbackCalled := false
	tw := &topicWatcher{
		dir:      dir,
		registry: reg,
		interval: 50 * time.Millisecond,
		stop:     make(chan struct{}),
		lastSeen: make(map[string]string),
		onUpdate: func() { callbackCalled = true },
	}

	tw.start()
	time.Sleep(150 * time.Millisecond)
	tw.shutdown()

	if !callbackCalled {
		t.Error("onUpdate callback was not called")
	}
}

func TestTopicWatcher_MultipleSessions(t *testing.T) {
	database := twTestDB(t)
	reg := newSessionRegistry(database)

	// Register two sessions with different PIDs.
	for i, pid := range []int{11111, 22222} {
		reg.register(RegisterSessionParams{
			SessionID: fmt.Sprintf("ccc-multi-%d", i),
			PID:       pid,
			Project:   "/tmp/test",
		})
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pid-11111.map"), []byte("uuid-aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "pid-22222.map"), []byte("uuid-bbb"), 0644)
	os.WriteFile(filepath.Join(dir, "uuid-aaa.txt"), []byte("Topic A"), 0644)
	os.WriteFile(filepath.Join(dir, "uuid-bbb.txt"), []byte("Topic B"), 0644)

	tw := &topicWatcher{
		dir:      dir,
		registry: reg,
		lastSeen: make(map[string]string),
	}

	updated := tw.scan()
	if len(updated) != 2 {
		t.Fatalf("expected 2 updates, got %d: %v", len(updated), updated)
	}

	sessions := reg.list()
	topics := make(map[string]string)
	for _, s := range sessions {
		topics[s.SessionID] = s.Topic
	}
	if topics["ccc-multi-0"] != "Topic A" {
		t.Errorf("session 0 topic = %q, want %q", topics["ccc-multi-0"], "Topic A")
	}
	if topics["ccc-multi-1"] != "Topic B" {
		t.Errorf("session 1 topic = %q, want %q", topics["ccc-multi-1"], "Topic B")
	}
}
