package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/worktree"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// assertViewContains fails with the full view dump if substr is not found.
func assertViewContains(t *testing.T, view, substr string) {
	t.Helper()
	if !strings.Contains(view, substr) {
		t.Fatalf("expected view to contain %q but it did not.\nFull view:\n%s", substr, view)
	}
}

// assertViewNotContains fails with the full view dump if substr IS found.
func assertViewNotContains(t *testing.T, view, substr string) {
	t.Helper()
	if strings.Contains(view, substr) {
		t.Fatalf("expected view NOT to contain %q but it did.\nFull view:\n%s", substr, view)
	}
}

// setupSessionsPlugin creates a plugin initialised on the "resume" sub-tab
// with loading=false so session items can be injected directly.
func setupSessionsPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := setupPlugin(t)
	p.subTab = "resume"
	p.loading = false
	return p
}

// sampleSessions returns a small set of bookmark sessions for testing.
func sampleSessions() []db.Session {
	return []db.Session{
		{
			SessionID: "s1",
			Project:   "/home/user/alpha",
			Repo:      "alpha",
			Branch:    "main",
			Summary:   "Working on alpha feature",
			Created:   time.Now(),
			Type:      db.SessionBookmark,
		},
		{
			SessionID: "s2",
			Project:   "/home/user/beta",
			Repo:      "beta",
			Branch:    "develop",
			Summary:   "Beta bugfix session",
			Created:   time.Now(),
			Type:      db.SessionBookmark,
		},
	}
}

// ---------------------------------------------------------------------------
// Tab Content
// ---------------------------------------------------------------------------

func TestView_NewTabShowsProjectList(t *testing.T) {
	p := setupPlugin(t)

	_ = db.DBAddPath(p.db, "/home/user/project-alpha")
	_ = db.DBAddPath(p.db, "/home/user/project-beta")
	p.paths = append(p.paths, "/home/user/project-alpha", "/home/user/project-beta")
	p.newList.SetItems(p.buildNewItems())

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "project-alpha")
	assertViewContains(t, view, "project-beta")
	assertViewContains(t, view, "Browse...")
}

func TestView_ResumeTabShowsSavedSessions(t *testing.T) {
	p := setupSessionsPlugin(t)
	sessions := sampleSessions()
	p.resumeList.SetItems(buildSessionItems(sessions))

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "alpha")
	assertViewContains(t, view, "beta")
	assertViewContains(t, view, "main")
	assertViewContains(t, view, "develop")
}

func TestView_NewTabDoesNotShowSavedSessions(t *testing.T) {
	p := setupPlugin(t)

	// Inject sessions into resume list
	sessions := sampleSessions()
	p.resumeList.SetItems(buildSessionItems(sessions))

	// Add a project path so new tab has something
	_ = db.DBAddPath(p.db, "/home/user/gamma")
	p.paths = append(p.paths, "/home/user/gamma")
	p.newList.SetItems(p.buildNewItems())

	// New tab should show project paths, not session summaries
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "gamma")
	assertViewNotContains(t, view, "Working on alpha feature")
	assertViewNotContains(t, view, "Beta bugfix session")
}

func TestView_ResumeTabDoesNotShowProjectPaths(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Add project paths to new list
	_ = db.DBAddPath(p.db, "/home/user/gamma-project")
	p.paths = append(p.paths, "/home/user/gamma-project")
	p.newList.SetItems(p.buildNewItems())

	// Inject sessions
	sessions := sampleSessions()
	p.resumeList.SetItems(buildSessionItems(sessions))

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "alpha")
	assertViewNotContains(t, view, "gamma-project")
}

func TestView_NewTabEmptyState(t *testing.T) {
	p := setupPlugin(t)

	// Default new tab has "Browse..." even with no paths
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Browse...")
}

func TestView_ResumeTabLoadingState(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "resume"
	// loading is true by default after init

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Loading sessions...")
}

func TestView_ResumeTabEmptyAfterLoad(t *testing.T) {
	p := setupSessionsPlugin(t)
	// No sessions injected — resume list is empty

	view := p.View(120, 38, 0)
	// The view should render without panic; the bubbles list renders
	// an empty state with "No items." or similar
	if view == "" {
		t.Fatal("expected non-empty view for empty resume tab")
	}
}

// ---------------------------------------------------------------------------
// Worktree Sub-Tab
// ---------------------------------------------------------------------------

func TestView_WorktreeSubTabSwitch(t *testing.T) {
	p := setupPlugin(t)

	// Verify initial tab is "new"
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "enter launch")

	// Press 't' to switch to worktrees
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	if p.subTab != "worktrees" {
		t.Fatalf("expected subTab 'worktrees', got %s", p.subTab)
	}

	view = p.View(120, 38, 0)
	assertViewContains(t, view, "enter launch")
	assertViewContains(t, view, "d delete")
	assertViewContains(t, view, "p prune")
}

func TestView_WorktreeEmptyState(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "worktrees"

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "No worktrees found")
}

func TestView_WorktreeDeleteConfirmation(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "worktrees"

	// Inject a fake worktree item
	p.worktreeItems = []worktreeItem{
		{
			info: worktree.WorktreeInfo{
				Path:      "/tmp/wt-branch",
				Branch:    "feature-x",
				RepoRoot:  "/tmp/myrepo",
				CreatedAt: time.Now(),
			},
			project: "myrepo",
		},
	}
	p.worktreeCursor = 0

	// Press 'd' to trigger delete confirmation
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Delete worktree")
	assertViewContains(t, view, "Yes, delete")
}

// ---------------------------------------------------------------------------
// Session Actions
// ---------------------------------------------------------------------------

func TestView_ConfirmDeleteShowsOverlay(t *testing.T) {
	p := setupPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/removable")
	p.paths = append(p.paths, "/tmp/removable")
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(0)

	// Trigger delete confirmation
	p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Remove")
	assertViewContains(t, view, "removable")
	assertViewContains(t, view, "yes")
	assertViewContains(t, view, "no")
}

func TestView_ConfirmYesRemovesFromView(t *testing.T) {
	p := setupPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/gone-project")
	p.paths = append(p.paths, "/tmp/gone-project")
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(0)

	// Enter confirming, then press 'y'
	p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	view := p.View(120, 38, 0)
	assertViewNotContains(t, view, "gone-project")
}

func TestView_ResumeConfirmDeleteShowsOverlay(t *testing.T) {
	p := setupSessionsPlugin(t)
	sessions := sampleSessions()
	p.resumeList.SetItems(buildSessionItems(sessions))
	p.resumeList.Select(0)

	// Trigger delete confirmation on resume tab
	p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Remove")
	assertViewContains(t, view, "alpha")
	assertViewContains(t, view, "yes")
	assertViewContains(t, view, "no")
}

func TestView_WorktreePruneConfirmation(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "worktrees"

	// Inject worktree items
	p.worktreeItems = []worktreeItem{
		{
			info: worktree.WorktreeInfo{
				Path:      "/tmp/wt-a",
				Branch:    "branch-a",
				RepoRoot:  "/tmp/repo",
				CreatedAt: time.Now(),
			},
			project: "repo",
		},
		{
			info: worktree.WorktreeInfo{
				Path:      "/tmp/wt-b",
				Branch:    "branch-b",
				RepoRoot:  "/tmp/repo",
				CreatedAt: time.Now(),
			},
			project: "repo",
		},
	}
	p.worktreeCursor = 0

	// Press 'p' to trigger prune confirmation
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Remove all worktrees for")
	assertViewContains(t, view, "repo")
	assertViewContains(t, view, "2 worktrees")
	assertViewContains(t, view, "Yes, prune all")
}

// ---------------------------------------------------------------------------
// Navigation Integrity
// ---------------------------------------------------------------------------

func TestView_TabSwitchPreservesContent(t *testing.T) {
	p := setupPlugin(t)

	// Add path and sessions
	_ = db.DBAddPath(p.db, "/tmp/stable-project")
	p.paths = append(p.paths, "/tmp/stable-project")
	p.newList.SetItems(p.buildNewItems())
	p.loading = false
	sessions := sampleSessions()
	p.resumeList.SetItems(buildSessionItems(sessions))

	// Check new tab
	view1 := p.View(120, 38, 0)
	assertViewContains(t, view1, "stable-project")

	// Switch to resume
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	view2 := p.View(120, 38, 0)
	assertViewContains(t, view2, "alpha")
	assertViewNotContains(t, view2, "stable-project")

	// Switch back to new
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	view3 := p.View(120, 38, 0)
	assertViewContains(t, view3, "stable-project")
	assertViewNotContains(t, view3, "Working on alpha feature")
}

func TestView_HintBarUpdatesPerSubTab(t *testing.T) {
	p := setupPlugin(t)
	p.loading = false

	// New tab hints
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "enter launch")
	assertViewContains(t, view, "w worktree")
	assertViewContains(t, view, "r resume")
	assertViewContains(t, view, "t worktrees")

	// Resume tab hints
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "enter resume")
	assertViewContains(t, view, "n new")
	assertViewContains(t, view, "t worktrees")

	// Worktrees tab hints
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "enter launch")
	assertViewContains(t, view, "d delete")
	assertViewContains(t, view, "p prune")
}

func TestView_FilterTypingInNewTab(t *testing.T) {
	p := setupPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/alpha-project")
	_ = db.DBAddPath(p.db, "/tmp/beta-project")
	p.paths = append(p.paths, "/tmp/alpha-project", "/tmp/beta-project")
	p.newList.SetItems(p.buildNewItems())

	// Verify both visible initially
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "alpha-project")
	assertViewContains(t, view, "beta-project")

	// Type filter characters
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	view = p.View(120, 38, 0)
	assertViewContains(t, view, "alpha-project")
	assertViewContains(t, view, "filter: alp")
	// beta should be filtered out of the visible items list
	if len(p.newList.VisibleItems()) != 1 {
		t.Fatalf("expected 1 visible item after filtering 'alp', got %d", len(p.newList.VisibleItems()))
	}
}

func TestView_FilterHintBarShowsFilterMode(t *testing.T) {
	p := setupPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/test-proj")
	p.paths = append(p.paths, "/tmp/test-proj")
	p.newList.SetItems(p.buildNewItems())

	// Before filtering — normal hints
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "type to filter")

	// Start filtering
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	view = p.View(120, 38, 0)
	assertViewContains(t, view, "filter: x")
	assertViewContains(t, view, "esc clear")
	assertViewContains(t, view, "backspace edit")
}

func TestView_ResumeFilterHintBar(t *testing.T) {
	p := setupSessionsPlugin(t)
	sessions := sampleSessions()
	p.resumeList.SetItems(buildSessionItems(sessions))

	// Before filtering
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "type to filter")

	// Start filtering
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	view = p.View(120, 38, 0)
	assertViewContains(t, view, "filter: a")
	assertViewContains(t, view, "enter resume")
}

// ---------------------------------------------------------------------------
// Warning / Not-a-git-repo overlay
// ---------------------------------------------------------------------------

func TestView_WorktreeWarningOverlay(t *testing.T) {
	p := setupPlugin(t)
	p.worktreeWarning = "/tmp/not-a-repo"

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Not a git repository")
	assertViewContains(t, view, "Launch directly")
}

func TestView_PendingTodoBanner(t *testing.T) {
	p := setupPlugin(t)
	p.pendingLaunchTodo = &db.Todo{Title: "Fix critical bug"}

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Select project for:")
	assertViewContains(t, view, "Fix critical bug")
	assertViewContains(t, view, "esc to cancel")
}
