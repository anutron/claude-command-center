package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/worktree"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertViewContains(t *testing.T, view, substr string) {
	t.Helper()
	if !strings.Contains(view, substr) {
		t.Fatalf("expected view to contain %q but it did not.\nFull view:\n%s", substr, view)
	}
}

func assertViewNotContains(t *testing.T, view, substr string) {
	t.Helper()
	if strings.Contains(view, substr) {
		t.Fatalf("expected view NOT to contain %q but it did.\nFull view:\n%s", substr, view)
	}
}

// setupNewTabPlugin creates a plugin on the "new" sub-tab.
func setupNewTabPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := setupPlugin(t)
	p.subTab = "new"
	return p
}

// setupResumePlugin creates a plugin on the resume view (sessions subTab with saved filter).
func setupResumePlugin(t *testing.T) *Plugin {
	t.Helper()
	p := setupPlugin(t)
	p.NavigateTo("resume", nil)
	return p
}

func sampleSessions() []db.Session {
	return []db.Session{
		{SessionID: "s1", Project: "/home/user/alpha", Repo: "alpha", Branch: "main", Summary: "Working on alpha feature", Created: time.Now(), Type: db.SessionBookmark},
		{SessionID: "s2", Project: "/home/user/beta", Repo: "beta", Branch: "develop", Summary: "Beta bugfix session", Created: time.Now(), Type: db.SessionBookmark},
	}
}

// ---------------------------------------------------------------------------
// Tab Content
// ---------------------------------------------------------------------------

func TestView_NewTabShowsProjectList(t *testing.T) {
	p := setupNewTabPlugin(t)

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
	p := setupResumePlugin(t)
	p.unified.SetSavedSessions(sampleSessions())

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "alpha")
	assertViewContains(t, view, "beta")
}

func TestView_NewTabDoesNotShowSavedSessions(t *testing.T) {
	p := setupNewTabPlugin(t)
	p.unified.savedSessions = sampleSessions()

	_ = db.DBAddPath(p.db, "/home/user/gamma")
	p.paths = append(p.paths, "/home/user/gamma")
	p.newList.SetItems(p.buildNewItems())

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "gamma")
	assertViewNotContains(t, view, "Working on alpha feature")
}

func TestView_ResumeTabDoesNotShowProjectPaths(t *testing.T) {
	p := setupResumePlugin(t)

	_ = db.DBAddPath(p.db, "/home/user/gamma-project")
	p.paths = append(p.paths, "/home/user/gamma-project")
	p.newList.SetItems(p.buildNewItems())
	p.unified.SetSavedSessions(sampleSessions())

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "alpha")
	assertViewNotContains(t, view, "gamma-project")
}

func TestView_NewTabEmptyState(t *testing.T) {
	p := setupNewTabPlugin(t)

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Browse...")
}

func TestView_ResumeTabEmptyState(t *testing.T) {
	p := setupResumePlugin(t)
	p.unified.SetSavedSessions(nil) // explicitly empty
	view := p.View(120, 38, 0)
	// Should render without panic — empty state or no items message
	if len(view) == 0 {
		t.Fatal("expected non-empty view for empty resume tab")
	}
}

// ---------------------------------------------------------------------------
// Worktree Sub-Tab
// ---------------------------------------------------------------------------

func TestView_WorktreeSubTabSwitch(t *testing.T) {
	p := setupNewTabPlugin(t)

	// Press 't' to switch to worktrees
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	if p.subTab != "worktrees" {
		t.Fatalf("expected subTab 'worktrees', got %s", p.subTab)
	}

	view := p.View(120, 38, 0)
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

	p.worktreeItems = []worktreeItem{
		{
			info: worktree.WorktreeInfo{
				Path: "/tmp/wt-branch", Branch: "feature-x",
				RepoRoot: "/tmp/myrepo", CreatedAt: time.Now(),
			},
			project: "myrepo",
		},
	}
	p.worktreeCursor = 0

	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Delete worktree")
	assertViewContains(t, view, "Yes, delete")
}

// ---------------------------------------------------------------------------
// Session Actions
// ---------------------------------------------------------------------------

func TestView_ConfirmDeleteShowsOverlay(t *testing.T) {
	p := setupNewTabPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/removable")
	p.paths = append(p.paths, "/tmp/removable")
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(0)

	p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Remove")
	assertViewContains(t, view, "removable")
}

func TestView_ConfirmYesRemovesFromView(t *testing.T) {
	p := setupNewTabPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/gone-project")
	p.paths = append(p.paths, "/tmp/gone-project")
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(0)

	p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	view := p.View(120, 38, 0)
	assertViewNotContains(t, view, "gone-project")
}

func TestView_ResumeTabDismissOnSavedSession(t *testing.T) {
	p := setupResumePlugin(t)
	p.NavigateTo("resume", nil)
	p.HandleMessage(plugin.TabViewMsg{Route: "resume"})

	p.unified.savedSessions = []db.Session{
		{SessionID: "saved-del-001", Project: "/home/user/proj", Summary: "Deletable Session", Created: time.Now(), Type: db.SessionBookmark},
	}

	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	view := p.View(120, 38, 0)
	_ = view // Smoke test — no panic
}

func TestView_WorktreePruneConfirmation(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "worktrees"

	p.worktreeItems = []worktreeItem{
		{info: worktree.WorktreeInfo{Path: "/tmp/wt-a", Branch: "branch-a", RepoRoot: "/tmp/repo", CreatedAt: time.Now()}, project: "repo"},
		{info: worktree.WorktreeInfo{Path: "/tmp/wt-b", Branch: "branch-b", RepoRoot: "/tmp/repo", CreatedAt: time.Now()}, project: "repo"},
	}
	p.worktreeCursor = 0

	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Remove all worktrees for")
	assertViewContains(t, view, "repo")
}

// ---------------------------------------------------------------------------
// Navigation Integrity
// ---------------------------------------------------------------------------

func TestView_TabSwitchPreservesContent(t *testing.T) {
	p := setupNewTabPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/stable-project")
	p.paths = append(p.paths, "/tmp/stable-project")
	p.newList.SetItems(p.buildNewItems())
	p.unified.SetSavedSessions(sampleSessions())

	// New tab shows project
	view1 := p.View(120, 38, 0)
	assertViewContains(t, view1, "stable-project")

	// Switch to resume via NavigateTo (key 'r' is captured as filter input on new tab)
	p.NavigateTo("resume", nil)
	p.HandleMessage(plugin.TabViewMsg{Route: "resume"})
	view2 := p.View(120, 38, 0)
	assertViewContains(t, view2, "alpha")

	// Switch back to new
	p.NavigateTo("new", nil)
	p.HandleMessage(plugin.TabViewMsg{Route: "new"})
	view3 := p.View(120, 38, 0)
	assertViewContains(t, view3, "stable-project")
}

func TestView_HintBarUpdatesPerSubTab(t *testing.T) {
	p := setupNewTabPlugin(t)

	// New tab hints
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "enter launch")
	assertViewContains(t, view, "t worktrees")

	// Worktrees tab hints
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "d delete")
	assertViewContains(t, view, "p prune")
}

func TestView_HintBarHidesSessionsKeyOnSessionsTab(t *testing.T) {
	p := setupNewTabPlugin(t)

	// New tab should show "s sessions" (it's useful for switching)
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "s sessions")

	// Switch to sessions sub-tab
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	view = p.View(120, 38, 0)

	// Sessions tab should NOT show "s sessions" — already there
	assertViewNotContains(t, view, "s sessions")
	// But should still show other navigation hints
	assertViewContains(t, view, "n new")
	assertViewContains(t, view, "t worktrees")
}

func TestView_FilterTypingInNewTab(t *testing.T) {
	p := setupNewTabPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/alpha-project")
	_ = db.DBAddPath(p.db, "/tmp/beta-project")
	p.paths = append(p.paths, "/tmp/alpha-project", "/tmp/beta-project")
	p.newList.SetItems(p.buildNewItems())

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "alpha-project")
	assertViewContains(t, view, "beta-project")

	// Type filter
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	view = p.View(120, 38, 0)
	assertViewContains(t, view, "alpha-project")
	assertViewContains(t, view, "filter: alp")
}

func TestView_FilterHintBarShowsFilterMode(t *testing.T) {
	p := setupNewTabPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/test-proj")
	p.paths = append(p.paths, "/tmp/test-proj")
	p.newList.SetItems(p.buildNewItems())

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "type to filter")

	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	view = p.View(120, 38, 0)
	assertViewContains(t, view, "filter: x")
	assertViewContains(t, view, "esc clear")
}

// ---------------------------------------------------------------------------
// Overlays and Banners
// ---------------------------------------------------------------------------

func TestView_WorktreeWarningOverlay(t *testing.T) {
	p := setupPlugin(t)
	p.worktreeWarning = "/tmp/not-a-repo"

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Not a git repository")
	assertViewContains(t, view, "Launch directly")
}

func TestView_PendingTodoBanner(t *testing.T) {
	p := setupNewTabPlugin(t)
	p.pendingLaunchTodo = &db.Todo{Title: "Fix critical bug"}

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Select project for:")
	assertViewContains(t, view, "Fix critical bug")
}
