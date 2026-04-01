package prs

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
)

func setupPRPlugin(t *testing.T) *Plugin {
	t.Helper()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{Palette: "aurora"}
	p := &Plugin{}
	err = p.Init(plugin.Context{
		DB:     database,
		Config: cfg,
		Bus:    plugin.NewBus(),
		Logger: plugin.NewMemoryLogger(),
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	p.HandleMessage(tea.WindowSizeMsg{Width: 120, Height: 40})
	return p
}

// ---------------------------------------------------------------------------
// Daemon agent.finished / agent.started NotifyMsg handling
// ---------------------------------------------------------------------------

func TestDaemonAgentFinished_TransitionsToCompleted(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "Review PR", Category: CategoryReview,
			LastActivityAt: recentTime(), AgentStatus: "running", HeadSHA: "abc"},
	})

	// Simulate daemon broadcasting agent.finished with exit code 0.
	msg := plugin.NotifyMsg{
		Event: "agent.finished",
		Data:  []byte(`{"id":"o/r#1","exit_code":0}`),
	}
	handled, _ := p.HandleMessage(msg)
	if !handled {
		t.Fatal("expected HandleMessage to handle agent.finished NotifyMsg for PR agent")
	}

	// In-memory status should transition to completed.
	for _, pr := range p.prs {
		if pr.ID == "o/r#1" {
			if pr.AgentStatus != "completed" {
				t.Errorf("expected agent status %q, got %q", "completed", pr.AgentStatus)
			}
			return
		}
	}
	t.Fatal("PR o/r#1 not found")
}

func TestDaemonAgentFinished_NonZeroExitShowsFailed(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#2", Repo: "o/r", Number: 2, Title: "Fail PR", Category: CategoryReview,
			LastActivityAt: recentTime(), AgentStatus: "running", HeadSHA: "abc"},
	})

	msg := plugin.NotifyMsg{
		Event: "agent.finished",
		Data:  []byte(`{"id":"o/r#2","exit_code":1}`),
	}
	handled, _ := p.HandleMessage(msg)
	if !handled {
		t.Fatal("expected HandleMessage to handle agent.finished NotifyMsg")
	}

	for _, pr := range p.prs {
		if pr.ID == "o/r#2" {
			if pr.AgentStatus != "failed" {
				t.Errorf("expected agent status %q, got %q", "failed", pr.AgentStatus)
			}
			return
		}
	}
	t.Fatal("PR o/r#2 not found")
}

func TestDaemonAgentFinished_IgnoresUnknownID(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "PR", Category: CategoryReview,
			LastActivityAt: recentTime(), AgentStatus: "running", HeadSHA: "abc"},
	})

	// Agent ID that doesn't match any PR — should not be handled.
	msg := plugin.NotifyMsg{
		Event: "agent.finished",
		Data:  []byte(`{"id":"unknown-todo-id","exit_code":0}`),
	}
	handled, _ := p.HandleMessage(msg)
	if handled {
		t.Error("expected HandleMessage to NOT handle agent.finished for non-PR agent ID")
	}
}

func TestDaemonAgentStarted_TransitionsToRunning(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#3", Repo: "o/r", Number: 3, Title: "Start PR", Category: CategoryReview,
			LastActivityAt: recentTime(), AgentStatus: "pending", HeadSHA: "abc"},
	})

	// Simulate daemon broadcasting agent.started.
	msg := plugin.NotifyMsg{
		Event: "agent.started",
		Data:  []byte(`{"id":"o/r#3","status":"processing","started_at":"2026-03-31T12:00:00Z"}`),
	}
	handled, _ := p.HandleMessage(msg)
	if !handled {
		t.Fatal("expected HandleMessage to handle agent.started NotifyMsg for PR agent")
	}

	for _, pr := range p.prs {
		if pr.ID == "o/r#3" {
			if pr.AgentStatus != "running" {
				t.Errorf("expected agent status %q, got %q", "running", pr.AgentStatus)
			}
			return
		}
	}
	t.Fatal("PR o/r#3 not found")
}

func TestDaemonAgentSessionID_UpdatesPRSessionID(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "PR", Category: CategoryReview,
			LastActivityAt: recentTime(), AgentStatus: "running", HeadSHA: "abc"},
	})

	msg := plugin.NotifyMsg{
		Event: "agent.session_id",
		Data:  []byte(`{"id":"o/r#1","session_id":"uuid-pr-456"}`),
	}
	handled, _ := p.HandleMessage(msg)
	if !handled {
		t.Fatal("expected HandleMessage to handle agent.session_id NotifyMsg for PR")
	}

	for _, pr := range p.prs {
		if pr.ID == "o/r#1" {
			if pr.AgentSessionID != "uuid-pr-456" {
				t.Errorf("expected session ID %q, got %q", "uuid-pr-456", pr.AgentSessionID)
			}
			return
		}
	}
	t.Fatal("PR not found")
}

// loadPRsIntoPlugin inserts all PRs in a single transaction (so
// DBSavePullRequests doesn't archive earlier ones) and triggers a reload.
func loadPRsIntoPlugin(t *testing.T, p *Plugin, prs []db.PullRequest) {
	t.Helper()
	tx, err := p.database.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := db.DBSavePullRequests(tx, prs); err != nil {
		tx.Rollback()
		t.Fatalf("save PRs: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	// Execute the Refresh command to load PRs from DB.
	cmd := p.Refresh()
	if cmd != nil {
		msg := cmd()
		p.HandleMessage(msg)
	}
}

func recentTime() time.Time {
	return time.Now().Add(-1 * time.Hour)
}

// ---------------------------------------------------------------------------
// Tab Bar and Navigation
// ---------------------------------------------------------------------------

func TestView_TabBarShowsCategoryCounts(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "W1", Category: CategoryWaiting, LastActivityAt: recentTime()},
		{ID: "o/r#2", Repo: "o/r", Number: 2, Title: "W2", Category: CategoryWaiting, LastActivityAt: recentTime()},
		{ID: "o/r#3", Repo: "o/r", Number: 3, Title: "W3", Category: CategoryWaiting, LastActivityAt: recentTime()},
		{ID: "o/r#4", Repo: "o/r", Number: 4, Title: "R1", Category: CategoryRespond, LastActivityAt: recentTime()},
		{ID: "o/r#5", Repo: "o/r", Number: 5, Title: "R2", Category: CategoryRespond, LastActivityAt: recentTime()},
	})

	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "Waiting (3)")
	testutil.AssertViewContains(t, view, "Respond (2)")
	testutil.AssertViewContains(t, view, "Review (0)")
	testutil.AssertViewContains(t, view, "Stale (0)")
}

func TestView_CategorySwitchViaNumberKeys(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "Respond PR", Category: CategoryRespond, LastActivityAt: recentTime()},
	})

	// Start on Waiting tab (0)
	if p.activeTab != 0 {
		t.Fatalf("expected initial activeTab=0, got %d", p.activeTab)
	}

	// Press "2" to switch to Respond tab
	p.HandleKey(testutil.KeyMsg("2"))
	if p.activeTab != 1 {
		t.Fatalf("expected activeTab=1 after pressing 2, got %d", p.activeTab)
	}

	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "Respond PR")
}

func TestView_CategorySwitchViaArrows(t *testing.T) {
	p := setupPRPlugin(t)

	// Start on tab 0
	if p.activeTab != 0 {
		t.Fatalf("expected initial activeTab=0, got %d", p.activeTab)
	}

	// Press right to advance
	p.HandleKey(testutil.KeyMsg("right"))
	if p.activeTab != 1 {
		t.Errorf("expected activeTab=1 after right, got %d", p.activeTab)
	}

	// Press right again
	p.HandleKey(testutil.KeyMsg("right"))
	if p.activeTab != 2 {
		t.Errorf("expected activeTab=2 after second right, got %d", p.activeTab)
	}

	// Press left to go back
	p.HandleKey(testutil.KeyMsg("left"))
	if p.activeTab != 1 {
		t.Errorf("expected activeTab=1 after left, got %d", p.activeTab)
	}

	// Wrap around: go to tab 0 then left should wrap to tab 3
	p.HandleKey(testutil.KeyMsg("1")) // go to tab 0
	p.HandleKey(testutil.KeyMsg("left"))
	if p.activeTab != 3 {
		t.Errorf("expected activeTab=3 after wrapping left, got %d", p.activeTab)
	}
}

func TestView_CursorNavigationWraps(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "PR-A", Category: CategoryWaiting, LastActivityAt: recentTime()},
		{ID: "o/r#2", Repo: "o/r", Number: 2, Title: "PR-B", Category: CategoryWaiting, LastActivityAt: recentTime()},
		{ID: "o/r#3", Repo: "o/r", Number: 3, Title: "PR-C", Category: CategoryWaiting, LastActivityAt: recentTime()},
	})

	// Cursor starts at 0
	if p.cursors[0] != 0 {
		t.Fatalf("expected cursor=0, got %d", p.cursors[0])
	}

	// Move down twice
	p.HandleKey(testutil.KeyMsg("j"))
	p.HandleKey(testutil.KeyMsg("j"))
	if p.cursors[0] != 2 {
		t.Errorf("expected cursor=2 after 2 downs, got %d", p.cursors[0])
	}

	// Move down once more: should wrap to 0
	p.HandleKey(testutil.KeyMsg("j"))
	if p.cursors[0] != 0 {
		t.Errorf("expected cursor=0 after wrap, got %d", p.cursors[0])
	}

	// Move up from 0: should wrap to 2
	p.HandleKey(testutil.KeyMsg("k"))
	if p.cursors[0] != 2 {
		t.Errorf("expected cursor=2 after up-wrap, got %d", p.cursors[0])
	}
}

func TestView_CursorPreservedPerTab(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "W1", Category: CategoryWaiting, LastActivityAt: recentTime()},
		{ID: "o/r#2", Repo: "o/r", Number: 2, Title: "W2", Category: CategoryWaiting, LastActivityAt: recentTime()},
		{ID: "o/r#3", Repo: "o/r", Number: 3, Title: "R1", Category: CategoryRespond, LastActivityAt: recentTime()},
		{ID: "o/r#4", Repo: "o/r", Number: 4, Title: "R2", Category: CategoryRespond, LastActivityAt: recentTime()},
	})

	// Move cursor down in Waiting tab (tab 0)
	p.HandleKey(testutil.KeyMsg("j"))
	if p.cursors[0] != 1 {
		t.Fatalf("expected cursor=1 in waiting tab, got %d", p.cursors[0])
	}

	// Switch to Respond tab
	p.HandleKey(testutil.KeyMsg("2"))
	if p.cursors[1] != 0 {
		t.Errorf("expected cursor=0 in respond tab, got %d", p.cursors[1])
	}

	// Switch back to Waiting tab
	p.HandleKey(testutil.KeyMsg("1"))
	if p.cursors[0] != 1 {
		t.Errorf("expected cursor=1 preserved in waiting tab, got %d", p.cursors[0])
	}
}

// ---------------------------------------------------------------------------
// PR List Rendering
// ---------------------------------------------------------------------------

func TestView_PRListShowsRepoNumberTitle(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "acme/widgets#42", Repo: "acme/widgets", Number: 42, Title: "Add new widget", Category: CategoryWaiting, LastActivityAt: recentTime()},
	})

	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "acme/widgets#42")
	testutil.AssertViewContains(t, view, "Add new widget")
}

func TestView_EmptyStatePerCategory(t *testing.T) {
	p := setupPRPlugin(t)
	// No PRs loaded — each tab should show its empty message.

	tests := []struct {
		key     string
		message string
	}{
		{"1", categoryEmptyMessage[CategoryWaiting]},
		{"2", categoryEmptyMessage[CategoryRespond]},
		{"3", categoryEmptyMessage[CategoryReview]},
		{"4", categoryEmptyMessage[CategoryStale]},
	}

	for _, tt := range tests {
		p.HandleKey(testutil.KeyMsg(tt.key))
		view := p.View(120, 40, 0)
		testutil.AssertViewContains(t, view, tt.message)
	}
}

func TestView_WaitingTabShowsReviewerStatus(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{
			ID:                    "o/r#10",
			Repo:                  "o/r",
			Number:                10,
			Title:                 "Feature PR",
			Category:              CategoryWaiting,
			LastActivityAt:        recentTime(),
			ReviewerLogins:        []string{"alice", "bob"},
			PendingReviewerLogins: []string{"bob"},
		},
	})

	view := p.View(120, 40, 0)
	// alice has reviewed (checkmark), bob is pending (hourglass)
	testutil.AssertViewContains(t, view, "alice \u2713")
	testutil.AssertViewContains(t, view, "bob \u23f3")
}

func TestView_RespondTabShowsThreadCount(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{
			ID:                    "o/r#20",
			Repo:                  "o/r",
			Number:                20,
			Title:                 "Fix bug",
			Category:              CategoryRespond,
			LastActivityAt:        recentTime(),
			UnresolvedThreadCount: 3,
		},
	})

	// Switch to Respond tab
	p.HandleKey(testutil.KeyMsg("2"))
	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "3 threads")
}

// ---------------------------------------------------------------------------
// Ignore Functionality
// ---------------------------------------------------------------------------

func TestView_IgnorePRRemovesFromView(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "Doomed PR", Category: CategoryWaiting, LastActivityAt: recentTime()},
	})

	// Verify PR row is visible (repo#number format)
	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "o/r#1")

	// Press 'i' to ignore
	action := p.HandleKey(testutil.KeyMsg("i"))

	// Execute the returned refresh command to reload from DB
	if action.TeaCmd != nil {
		msg := action.TeaCmd()
		p.HandleMessage(msg)
	}

	view = p.View(120, 40, 0)
	// The PR row (repo#number) should be gone; the flash message may
	// still mention the title but the list row should not.
	testutil.AssertViewNotContains(t, view, "o/r#1")
}

func TestView_IgnoreRepoRemovesAllRepoPRs(t *testing.T) {
	p := setupPRPlugin(t)
	// bad/repo PRs have more recent activity so they sort first (cursor starts on one).
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "bad/repo#1", Repo: "bad/repo", Number: 1, Title: "Bad PR 1", Category: CategoryWaiting, LastActivityAt: time.Now().Add(-10 * time.Minute)},
		{ID: "bad/repo#2", Repo: "bad/repo", Number: 2, Title: "Bad PR 2", Category: CategoryWaiting, LastActivityAt: time.Now().Add(-20 * time.Minute)},
		{ID: "good/repo#3", Repo: "good/repo", Number: 3, Title: "Good PR", Category: CategoryWaiting, LastActivityAt: time.Now().Add(-2 * time.Hour)},
	})

	// Verify all are visible
	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "bad/repo#1")
	testutil.AssertViewContains(t, view, "good/repo#3")

	// Cursor is on first PR (bad/repo#1, most recent).
	// Press 'I' to ignore the whole repo.
	action := p.HandleKey(testutil.KeyMsg("I"))

	// Execute the returned refresh command
	if action.TeaCmd != nil {
		msg := action.TeaCmd()
		p.HandleMessage(msg)
	}

	view = p.View(120, 40, 0)
	testutil.AssertViewNotContains(t, view, "bad/repo#1")
	testutil.AssertViewNotContains(t, view, "bad/repo#2")
	testutil.AssertViewContains(t, view, "good/repo#3")
}

func TestView_FlashMessageVisibleAfterIgnore(t *testing.T) {
	p := setupPRPlugin(t)
	loadPRsIntoPlugin(t, p, []db.PullRequest{
		{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "Flash Test PR", Category: CategoryWaiting, LastActivityAt: recentTime()},
	})

	p.HandleKey(testutil.KeyMsg("i"))

	// Render immediately so flash message hasn't expired
	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "PR ignored: Flash Test PR")
}

// ---------------------------------------------------------------------------
// Agent Integration
// ---------------------------------------------------------------------------

func TestView_AgentStatusBadgeRendered(t *testing.T) {
	p := setupPRPlugin(t)

	// Directly set PRs with agent status since we're in the same package.
	p.prs = []db.PullRequest{
		{
			ID:          "o/r#1",
			Repo:        "o/r",
			Number:      1,
			Title:       "Agent PR",
			Category:    CategoryReview,
			AgentStatus: "running",
		},
	}

	// Switch to Review tab
	p.HandleKey(testutil.KeyMsg("3"))
	view := p.View(120, 40, 0)
	testutil.AssertViewContains(t, view, "running")
}

func TestView_HintBarContent(t *testing.T) {
	p := setupPRPlugin(t)
	view := p.View(120, 40, 0)

	testutil.AssertViewContains(t, view, "1-4 tab")
	testutil.AssertViewContains(t, view, "j/k nav")
	testutil.AssertViewContains(t, view, "enter review/respond")
	testutil.AssertViewContains(t, view, "o open")
	testutil.AssertViewContains(t, view, "i ignore")
	testutil.AssertViewContains(t, view, "r refresh")
}

// ---------------------------------------------------------------------------
// General
// ---------------------------------------------------------------------------

func TestView_RendersWithoutPanic(t *testing.T) {
	tests := []struct {
		name string
		prs  []db.PullRequest
		tab  int
	}{
		{"empty_waiting", nil, 0},
		{"empty_respond", nil, 1},
		{"empty_review", nil, 2},
		{"empty_stale", nil, 3},
		{
			"with_waiting_prs",
			[]db.PullRequest{
				{ID: "o/r#1", Repo: "o/r", Number: 1, Title: "T1", Category: CategoryWaiting, LastActivityAt: recentTime()},
			},
			0,
		},
		{
			"with_respond_prs",
			[]db.PullRequest{
				{ID: "o/r#2", Repo: "o/r", Number: 2, Title: "T2", Category: CategoryRespond, LastActivityAt: recentTime(), UnresolvedThreadCount: 5, ReviewDecision: "CHANGES_REQUESTED"},
			},
			1,
		},
		{
			"with_review_prs",
			[]db.PullRequest{
				{ID: "o/r#3", Repo: "o/r", Number: 3, Title: "T3", Category: CategoryReview, Author: "dev", LastActivityAt: recentTime(), Draft: true},
			},
			2,
		},
		{
			"with_stale_prs",
			[]db.PullRequest{
				{ID: "o/r#4", Repo: "o/r", Number: 4, Title: "T4", Category: CategoryStale, LastActivityAt: recentTime(), CIStatus: "failure", Draft: true},
			},
			3,
		},
		{
			"with_agent_statuses",
			[]db.PullRequest{
				{ID: "o/r#5", Repo: "o/r", Number: 5, Title: "T5", Category: CategoryReview, AgentStatus: "pending"},
				{ID: "o/r#6", Repo: "o/r", Number: 6, Title: "T6", Category: CategoryReview, AgentStatus: "completed"},
				{ID: "o/r#7", Repo: "o/r", Number: 7, Title: "T7", Category: CategoryReview, AgentStatus: "failed"},
			},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := setupPRPlugin(t)
			p.prs = tt.prs
			p.activeTab = tt.tab

			view := p.View(120, 40, 0)
			if view == "" {
				t.Error("expected non-empty view")
			}
		})
	}
}
