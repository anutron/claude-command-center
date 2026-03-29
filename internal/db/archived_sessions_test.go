package db

import (
	"testing"
	"time"
)

func TestArchivedSessionInsertAndLoad(t *testing.T) {
	d := setupTestDB(t)

	now := FormatTime(time.Now())
	s := ArchivedSession{
		SessionID:    "archived-sess-001",
		Topic:        "Fix login bug",
		Project:      "myapp",
		Repo:         "my-repo",
		Branch:       "fix/login",
		WorktreePath: "/tmp/worktrees/fix-login",
		RegisteredAt: now,
		EndedAt:      now,
	}

	if err := DBInsertArchivedSession(d, s); err != nil {
		t.Fatalf("DBInsertArchivedSession: %v", err)
	}

	sessions, err := DBLoadArchivedSessions(d)
	if err != nil {
		t.Fatalf("DBLoadArchivedSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 archived session, got %d", len(sessions))
	}

	got := sessions[0]
	if got.SessionID != s.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, s.SessionID)
	}
	if got.Topic != s.Topic {
		t.Errorf("Topic: got %q, want %q", got.Topic, s.Topic)
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
	if got.RegisteredAt != s.RegisteredAt {
		t.Errorf("RegisteredAt: got %q, want %q", got.RegisteredAt, s.RegisteredAt)
	}
	if got.EndedAt != s.EndedAt {
		t.Errorf("EndedAt: got %q, want %q", got.EndedAt, s.EndedAt)
	}
}

func TestArchivedSessionDelete(t *testing.T) {
	d := setupTestDB(t)

	now := FormatTime(time.Now())
	s := ArchivedSession{
		SessionID:    "archived-sess-002",
		Topic:        "Refactor DB layer",
		RegisteredAt: now,
		EndedAt:      now,
	}

	if err := DBInsertArchivedSession(d, s); err != nil {
		t.Fatalf("DBInsertArchivedSession: %v", err)
	}

	if err := DBDeleteArchivedSession(d, s.SessionID); err != nil {
		t.Fatalf("DBDeleteArchivedSession: %v", err)
	}

	sessions, err := DBLoadArchivedSessions(d)
	if err != nil {
		t.Fatalf("DBLoadArchivedSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 archived sessions after delete, got %d", len(sessions))
	}
}

func TestArchivedSessionUpsert(t *testing.T) {
	d := setupTestDB(t)

	now := FormatTime(time.Now())
	s := ArchivedSession{
		SessionID:    "archived-sess-003",
		Topic:        "Original topic",
		RegisteredAt: now,
		EndedAt:      now,
	}

	if err := DBInsertArchivedSession(d, s); err != nil {
		t.Fatalf("DBInsertArchivedSession (first): %v", err)
	}

	// Insert again with a different topic — should upsert (one row, updated topic)
	s.Topic = "Updated topic"
	if err := DBInsertArchivedSession(d, s); err != nil {
		t.Fatalf("DBInsertArchivedSession (second): %v", err)
	}

	sessions, err := DBLoadArchivedSessions(d)
	if err != nil {
		t.Fatalf("DBLoadArchivedSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 archived session after upsert, got %d", len(sessions))
	}
	if sessions[0].Topic != "Updated topic" {
		t.Errorf("Topic: got %q, want %q", sessions[0].Topic, "Updated topic")
	}
}
