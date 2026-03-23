package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func makePR(id, repo string, number int, title string) PullRequest {
	now := time.Now()
	return PullRequest{
		ID:             id,
		Repo:           repo,
		Number:         number,
		Title:          title,
		URL:            "https://github.com/" + repo + "/pull/" + string(rune('0'+number)),
		Author:         "user",
		Draft:          false,
		CreatedAt:      now,
		UpdatedAt:      now,
		MyRole:         "author",
		LastActivityAt: now,
		FetchedAt:      now,
		HeadSHA:        "abc123",
	}
}

func TestDBSavePullRequests_UpsertPreservesAgentColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	pr := makePR("owner/repo#1", "owner/repo", 1, "Original title")

	// First save
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := DBSavePullRequests(tx, []PullRequest{pr}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Set agent columns via direct SQL
	_, err = db.Exec(`UPDATE cc_pull_requests SET
		agent_status = 'completed',
		agent_session_id = 'session-123',
		agent_category = 'review',
		agent_head_sha = 'abc123',
		agent_summary = 'Looks good'
		WHERE id = ?`, pr.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Second save with updated title
	pr.Title = "Updated title"
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := DBSavePullRequests(tx, []PullRequest{pr}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Verify title changed but agent columns preserved
	var title, agentStatus, agentSessionID, agentCategory, agentHeadSHA, agentSummary string
	err = db.QueryRow(`SELECT title, agent_status, agent_session_id, agent_category, agent_head_sha, agent_summary
		FROM cc_pull_requests WHERE id = ?`, pr.ID).Scan(
		&title, &agentStatus, &agentSessionID, &agentCategory, &agentHeadSHA, &agentSummary)
	if err != nil {
		t.Fatal(err)
	}

	if title != "Updated title" {
		t.Errorf("expected title 'Updated title', got %q", title)
	}
	if agentStatus != "completed" {
		t.Errorf("expected agent_status 'completed', got %q", agentStatus)
	}
	if agentSessionID != "session-123" {
		t.Errorf("expected agent_session_id 'session-123', got %q", agentSessionID)
	}
	if agentCategory != "review" {
		t.Errorf("expected agent_category 'review', got %q", agentCategory)
	}
	if agentHeadSHA != "abc123" {
		t.Errorf("expected agent_head_sha 'abc123', got %q", agentHeadSHA)
	}
	if agentSummary != "Looks good" {
		t.Errorf("expected agent_summary 'Looks good', got %q", agentSummary)
	}
}

func TestDBSavePullRequests_ArchivesMissingPRs(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	prA := makePR("owner/repo#1", "owner/repo", 1, "PR A")
	prB := makePR("owner/repo#2", "owner/repo", 2, "PR B")

	// Save both PRs
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := DBSavePullRequests(tx, []PullRequest{prA, prB}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Save only PR A — PR B should become archived
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := DBSavePullRequests(tx, []PullRequest{prA}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Verify PR B is archived
	var stateA, stateB string
	err = db.QueryRow(`SELECT state FROM cc_pull_requests WHERE id = ?`, prA.ID).Scan(&stateA)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(`SELECT state FROM cc_pull_requests WHERE id = ?`, prB.ID).Scan(&stateB)
	if err != nil {
		t.Fatal(err)
	}

	if stateA != "open" {
		t.Errorf("expected PR A state 'open', got %q", stateA)
	}
	if stateB != "archived" {
		t.Errorf("expected PR B state 'archived', got %q", stateB)
	}
}

func TestDBSavePullRequests_ReactivatesArchivedPRs(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	pr := makePR("owner/repo#1", "owner/repo", 1, "PR One")

	// Save PR
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := DBSavePullRequests(tx, []PullRequest{pr}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Archive it via SQL
	_, err = db.Exec(`UPDATE cc_pull_requests SET state = 'archived' WHERE id = ?`, pr.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Save again — should reactivate
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := DBSavePullRequests(tx, []PullRequest{pr}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var state string
	err = db.QueryRow(`SELECT state FROM cc_pull_requests WHERE id = ?`, pr.ID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != "open" {
		t.Errorf("expected state 'open' after reactivation, got %q", state)
	}
}

func TestDBSetPRIgnored(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "r#1", Repo: "r", Number: 1, Title: "T", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()

	// Ignore
	if err := DBSetPRIgnored(d, "r#1", true); err != nil {
		t.Fatal(err)
	}
	var ignored bool
	d.QueryRow(`SELECT ignored FROM cc_pull_requests WHERE id='r#1'`).Scan(&ignored)
	if !ignored {
		t.Error("expected ignored=true")
	}

	// Restore
	if err := DBSetPRIgnored(d, "r#1", false); err != nil {
		t.Fatal(err)
	}
	d.QueryRow(`SELECT ignored FROM cc_pull_requests WHERE id='r#1'`).Scan(&ignored)
	if ignored {
		t.Error("expected ignored=false")
	}
}

func TestDBIgnoredRepos_AddRemoveLoad(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	// Add
	if err := DBAddIgnoredRepo(d, "org/repo-a"); err != nil {
		t.Fatal(err)
	}
	if err := DBAddIgnoredRepo(d, "org/repo-b"); err != nil {
		t.Fatal(err)
	}
	// Duplicate is no-op
	if err := DBAddIgnoredRepo(d, "org/repo-a"); err != nil {
		t.Fatal(err)
	}

	repos, err := DBLoadIgnoredRepos(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(repos))
	}

	// Remove
	if err := DBRemoveIgnoredRepo(d, "org/repo-a"); err != nil {
		t.Fatal(err)
	}
	repos, _ = DBLoadIgnoredRepos(d)
	if len(repos) != 1 || repos[0] != "org/repo-b" {
		t.Errorf("expected [org/repo-b], got %v", repos)
	}
}

func TestDBUpdatePRAgentStatus(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	pr := makePR("owner/repo#1", "owner/repo", 1, "PR One")

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := DBSavePullRequests(tx, []PullRequest{pr}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	err = DBUpdatePRAgentStatus(db, pr.ID, "running", "sess-456", "respond", "def789", "Working on it")
	if err != nil {
		t.Fatal(err)
	}

	var agentStatus, agentSessionID, agentCategory, agentHeadSHA, agentSummary string
	err = db.QueryRow(`SELECT agent_status, agent_session_id, agent_category, agent_head_sha, agent_summary
		FROM cc_pull_requests WHERE id = ?`, pr.ID).Scan(
		&agentStatus, &agentSessionID, &agentCategory, &agentHeadSHA, &agentSummary)
	if err != nil {
		t.Fatal(err)
	}

	if agentStatus != "running" {
		t.Errorf("expected agent_status 'running', got %q", agentStatus)
	}
	if agentSessionID != "sess-456" {
		t.Errorf("expected agent_session_id 'sess-456', got %q", agentSessionID)
	}
	if agentCategory != "respond" {
		t.Errorf("expected agent_category 'respond', got %q", agentCategory)
	}
	if agentHeadSHA != "def789" {
		t.Errorf("expected agent_head_sha 'def789', got %q", agentHeadSHA)
	}
	if agentSummary != "Working on it" {
		t.Errorf("expected agent_summary 'Working on it', got %q", agentSummary)
	}
}
