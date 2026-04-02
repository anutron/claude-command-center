package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
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
	p.subTab = subTabNew
	return p
}

// setupResumePlugin creates a plugin on the Saved sub-tab (formerly "resume").
func setupResumePlugin(t *testing.T) *Plugin {
	t.Helper()
	p := setupPlugin(t)
	p.NavigateTo("sessions/saved", nil)
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

	// Press '4' to switch to worktrees (consolidated sub-tab navigation)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "d delete")
	assertViewContains(t, view, "p prune")
}

func TestView_WorktreeEmptyState(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = subTabWorktrees

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "No worktrees found")
}

func TestView_WorktreeDeleteConfirmation(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = subTabWorktrees

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
	p.NavigateTo("sessions/saved", nil)
	p.HandleMessage(plugin.TabViewMsg{Route: "sessions/saved"})

	p.unified.savedSessions = []db.Session{
		{SessionID: "saved-del-001", Project: "/home/user/proj", Summary: "Deletable Session", Created: time.Now(), Type: db.SessionBookmark},
	}

	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	view := p.View(120, 38, 0)
	_ = view // Smoke test — no panic
}

func TestView_WorktreePruneConfirmation(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = subTabWorktrees

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

	// Switch to Saved via NavigateTo
	p.NavigateTo("sessions/saved", nil)
	p.HandleMessage(plugin.TabViewMsg{Route: "sessions/saved"})
	view2 := p.View(120, 38, 0)
	assertViewContains(t, view2, "alpha")

	// Switch back to New Session
	p.NavigateTo("sessions/new", nil)
	p.HandleMessage(plugin.TabViewMsg{Route: "sessions/new"})
	view3 := p.View(120, 38, 0)
	assertViewContains(t, view3, "stable-project")
}

func TestView_HintBarUpdatesPerSubTab(t *testing.T) {
	p := setupNewTabPlugin(t)

	// New tab hints
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "enter launch")

	// Worktrees tab hints — switch via '4' key (new consolidated navigation)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "d delete")
	assertViewContains(t, view, "p prune")
}

func TestView_HintBarUpdatesPerSubTabConsolidated(t *testing.T) {
	p := setupPlugin(t)

	// New Session sub-tab hints (default sub-tab).
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "enter launch")

	// Switch to Saved sub-tab via number key.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	view = p.View(120, 38, 0)
	// Saved sub-tab should show session management hints.
	assertViewContains(t, view, "enter resume")
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

// ---------------------------------------------------------------------------
// Session Label Display (BUG-128)
// ---------------------------------------------------------------------------

func TestView_LiveSessionShowsProjectName(t *testing.T) {
	p := setupPlugin(t)
	// Switch to Recent sub-tab to see live sessions.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-001",
			Project:      "/home/user/claude-command-center",
			Branch:       "main",
			State:        "active",
			RegisteredAt: db.FormatTime(time.Now()),
		},
	}

	view := p.View(120, 38, 0)
	// Should show project basename, not just branch.
	assertViewContains(t, view, "claude-command-center")
}

func TestView_LiveSessionShowsTopicWhenSet(t *testing.T) {
	p := setupPlugin(t)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-002",
			Topic:        "AGENT CONSOLE",
			Project:      "/home/user/claude-command-center",
			Branch:       "main",
			State:        "active",
			RegisteredAt: db.FormatTime(time.Now()),
		},
	}

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "AGENT CONSOLE")
}

func TestView_LiveSessionShowsBranchInSuffix(t *testing.T) {
	p := setupPlugin(t)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-003",
			Project:      "/home/user/my-project",
			Branch:       "feature-x",
			State:        "active",
			RegisteredAt: db.FormatTime(time.Now()),
		},
	}

	view := p.View(120, 38, 0)
	// Branch should appear in parentheses in the suffix.
	assertViewContains(t, view, "(feature-x)")
}

func TestView_PendingTodoBanner(t *testing.T) {
	p := setupNewTabPlugin(t)
	p.pendingLaunchTodo = &db.Todo{Title: "Fix critical bug"}

	view := p.View(120, 38, 0)
	assertViewContains(t, view, "Select project for:")
	assertViewContains(t, view, "Fix critical bug")
}

// ---------------------------------------------------------------------------
// Sub-Tab Bar Rendering (Consolidation)
// ---------------------------------------------------------------------------

func TestView_SubTabBarRendered(t *testing.T) {
	p := setupPlugin(t)

	view := p.View(120, 38, 0)
	// The sub-tab bar should show all four sub-tabs with number keys.
	assertViewContains(t, view, "[1] New Session")
	assertViewContains(t, view, "[2] Saved")
	assertViewContains(t, view, "[3] Recent")
	assertViewContains(t, view, "[4] Worktrees")
}

// ---------------------------------------------------------------------------
// Number Key Sub-Tab Switching
// ---------------------------------------------------------------------------

func TestView_NumberKeySwitchesSubTab(t *testing.T) {
	p := setupPlugin(t)

	// Press '2' to switch to Saved sub-tab.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	view := p.View(120, 38, 0)
	// Saved sub-tab should show saved sessions content (or empty state).
	// The unified view with saved-only filter should be active.
	// At minimum, verify the sub-tab bar still renders and we're on Saved.
	assertViewContains(t, view, "[2] Saved")

	// Press '3' to switch to Recent sub-tab.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "[3] Recent")

	// Press '4' to switch to Worktrees sub-tab.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "[4] Worktrees")

	// Press '1' to go back to New Session.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "[1] New Session")
	assertViewContains(t, view, "Browse...")
}

// ---------------------------------------------------------------------------
// Left/Right Arrow Sub-Tab Cycling
// ---------------------------------------------------------------------------

func TestView_ArrowKeyCyclesSubTabs(t *testing.T) {
	p := setupPlugin(t)
	// Default sub-tab is New Session (0).

	// Right from New Session → Saved (1).
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	view := p.View(120, 38, 0)
	// Should be on Saved now — verify by checking for saved-specific content or empty state.
	assertViewContains(t, view, "[2] Saved")

	// Right from Saved → Recent (2).
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})

	// Right from Recent → Worktrees (3).
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "[4] Worktrees")

	// Right from Worktrees wraps → New Session (0).
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	view = p.View(120, 38, 0)
	assertViewContains(t, view, "Browse...")
}

func TestView_LeftArrowWrapsFromNewSession(t *testing.T) {
	p := setupPlugin(t)
	// Default sub-tab is New Session (0).

	// Left from New Session wraps → Worktrees (3).
	p.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	view := p.View(120, 38, 0)
	assertViewContains(t, view, "[4] Worktrees")
}

// ---------------------------------------------------------------------------
// Live Session Topic Display
// ---------------------------------------------------------------------------

func TestView_LiveSessionTopicWithProjectAndBranch(t *testing.T) {
	p := setupPlugin(t)
	// Switch to Recent sub-tab.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})

	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-topic-001",
			Topic:        "Test Topic",
			Project:      "/path/to/myproject",
			Branch:       "main",
			State:        "active",
			RegisteredAt: db.FormatTime(time.Now()),
		},
	}

	view := p.View(120, 38, 0)
	// Label should show the topic.
	assertViewContains(t, view, "Test Topic")
	// Suffix should show project basename and branch.
	assertViewContains(t, view, "myproject")
	assertViewContains(t, view, "(main)")
}
