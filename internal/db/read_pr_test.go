package db

import (
	"testing"
	"time"
)

func TestDBLoadPullRequests_FiltersArchived(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "r#1", Repo: "r", Number: 1, Title: "Open", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()
	d.Exec(`INSERT INTO cc_pull_requests (id, repo, number, title, url, author, created_at, updated_at, last_activity_at, fetched_at, state)
		VALUES ('r#2', 'r', 2, 'Archived', 'u', 'a', datetime('now'), datetime('now'), datetime('now'), datetime('now'), 'archived')`)

	prs, err := DBLoadPullRequests(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].ID != "r#1" {
		t.Errorf("expected 1 open PR, got %d", len(prs))
	}
}

func TestDBLoadPullRequests_ReadsAgentColumns(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "r#1", Repo: "r", Number: 1, Title: "T", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now, HeadSHA: "abc"},
	})
	tx.Commit()

	DBUpdatePRAgentStatus(d, "r#1", "completed", "sess-1", "review", "abc", "Looks good")

	prs, _ := DBLoadPullRequests(d)
	if len(prs) != 1 {
		t.Fatal("expected 1 PR")
	}
	if prs[0].AgentStatus != "completed" || prs[0].AgentSessionID != "sess-1" || prs[0].AgentSummary != "Looks good" {
		t.Errorf("agent columns not loaded: status=%q session=%q summary=%q",
			prs[0].AgentStatus, prs[0].AgentSessionID, prs[0].AgentSummary)
	}
	if prs[0].HeadSHA != "abc" {
		t.Errorf("HeadSHA not loaded: got %q", prs[0].HeadSHA)
	}
}

func TestDBLoadPullRequests_FiltersIgnoredPRs(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "r#1", Repo: "r", Number: 1, Title: "Visible", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
		{ID: "r#2", Repo: "r", Number: 2, Title: "Ignored", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()

	DBSetPRIgnored(d, "r#2", true)

	prs, err := DBLoadPullRequests(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].ID != "r#1" {
		t.Errorf("expected 1 visible PR, got %d", len(prs))
	}
}

func TestDBLoadPullRequests_FiltersIgnoredRepos(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "good/repo#1", Repo: "good/repo", Number: 1, Title: "Visible", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
		{ID: "bad/repo#1", Repo: "bad/repo", Number: 1, Title: "Hidden", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()

	DBAddIgnoredRepo(d, "bad/repo")

	prs, err := DBLoadPullRequests(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].Repo != "good/repo" {
		t.Errorf("expected 1 PR from good/repo, got %d", len(prs))
	}
}

func TestDBLoadIgnoredPRs(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "r#1", Repo: "r", Number: 1, Title: "Ignored", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
		{ID: "r#2", Repo: "r", Number: 2, Title: "Not Ignored", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()

	DBSetPRIgnored(d, "r#1", true)

	prs, err := DBLoadIgnoredPRs(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].ID != "r#1" {
		t.Errorf("expected 1 ignored PR, got %d", len(prs))
	}
}
