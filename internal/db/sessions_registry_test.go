package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestInsertAndLoadSession(t *testing.T) {
	d := setupTestDB(t)

	now := FormatTime(time.Now())
	s := SessionRecord{
		SessionID:    "sess-abc-123",
		Topic:        "Implement feature X",
		PID:          12345,
		Project:      "ccc",
		Repo:         "claude-command-center",
		Branch:       "feature/x",
		WorktreePath: "/tmp/worktree",
		State:        "active",
		RegisteredAt: now,
	}

	if err := DBInsertSession(d, s); err != nil {
		t.Fatalf("DBInsertSession: %v", err)
	}

	sessions, err := DBLoadActiveSessions(d)
	if err != nil {
		t.Fatalf("DBLoadActiveSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 active session, got %d", len(sessions))
	}

	got := sessions[0]
	if got.SessionID != s.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, s.SessionID)
	}
	if got.Topic != s.Topic {
		t.Errorf("Topic: got %q, want %q", got.Topic, s.Topic)
	}
	if got.PID != s.PID {
		t.Errorf("PID: got %d, want %d", got.PID, s.PID)
	}
	if got.Project != s.Project {
		t.Errorf("Project: got %q, want %q", got.Project, s.Project)
	}
	if got.Repo != s.Repo {
		t.Errorf("Repo: got %q, want %q", got.Repo, s.Repo)
	}
	if got.Branch != s.Branch {
		t.Errorf("Branch: got %q, want %q", got.Branch, s.Branch)
	}
	if got.WorktreePath != s.WorktreePath {
		t.Errorf("WorktreePath: got %q, want %q", got.WorktreePath, s.WorktreePath)
	}
	if got.State != "active" {
		t.Errorf("State: got %q, want %q", got.State, "active")
	}

	// Also verify via DBLoadSessions (all sessions)
	all, err := DBLoadSessions(d)
	if err != nil {
		t.Fatalf("DBLoadSessions: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 total session, got %d", len(all))
	}
}

func TestUpdateSessionTopic(t *testing.T) {
	d := setupTestDB(t)

	now := FormatTime(time.Now())
	s := SessionRecord{
		SessionID:    "sess-topic-test",
		Topic:        "Original topic",
		PID:          99999,
		State:        "active",
		RegisteredAt: now,
	}
	if err := DBInsertSession(d, s); err != nil {
		t.Fatalf("DBInsertSession: %v", err)
	}

	// Update topic
	if err := DBUpdateSession(d, "sess-topic-test", map[string]interface{}{
		"topic": "Updated topic",
	}); err != nil {
		t.Fatalf("DBUpdateSession: %v", err)
	}

	sessions, err := DBLoadSessions(d)
	if err != nil {
		t.Fatalf("DBLoadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Topic != "Updated topic" {
		t.Errorf("Topic: got %q, want %q", sessions[0].Topic, "Updated topic")
	}
}

func TestUpdateSessionState(t *testing.T) {
	d := setupTestDB(t)

	now := FormatTime(time.Now())
	s := SessionRecord{
		SessionID:    "sess-state-test",
		PID:          11111,
		State:        "active",
		RegisteredAt: now,
	}
	if err := DBInsertSession(d, s); err != nil {
		t.Fatalf("DBInsertSession: %v", err)
	}

	// Update state to "ended"
	if err := DBUpdateSessionState(d, "sess-state-test", "ended"); err != nil {
		t.Fatalf("DBUpdateSessionState: %v", err)
	}

	sessions, err := DBLoadSessions(d)
	if err != nil {
		t.Fatalf("DBLoadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].State != "ended" {
		t.Errorf("State: got %q, want %q", sessions[0].State, "ended")
	}
	if sessions[0].EndedAt == "" {
		t.Error("EndedAt should be set when state transitions to ended")
	}

	// "ended" sessions should still appear in active sessions query
	active, err := DBLoadActiveSessions(d)
	if err != nil {
		t.Fatalf("DBLoadActiveSessions: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active/ended session, got %d", len(active))
	}

	// Update to "archived" — should NOT appear in active sessions
	if err := DBUpdateSessionState(d, "sess-state-test", "archived"); err != nil {
		t.Fatalf("DBUpdateSessionState (archived): %v", err)
	}
	active, err = DBLoadActiveSessions(d)
	if err != nil {
		t.Fatalf("DBLoadActiveSessions: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active sessions after archiving, got %d", len(active))
	}
}
