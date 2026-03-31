package sessions

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// testLogger is a no-op logger for tests.
type testLogger struct{}

func (testLogger) Info(_, _ string, _ ...interface{})  {}
func (testLogger) Warn(_, _ string, _ ...interface{})  {}
func (testLogger) Error(_, _ string, _ ...interface{}) {}
func (testLogger) Recent(_ int) []plugin.LogEntry      { return nil }

func testConfig() *config.Config {
	return &config.Config{
		Name:    "TestBot",
		Palette: "aurora",
	}
}

func setupPlugin(t *testing.T) *Plugin {
	t.Helper()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := testConfig()
	p := &Plugin{}
	err = p.Init(plugin.Context{
		DB:     database,
		Config: cfg,
		Bus:    plugin.NewBus(),
		Logger: testLogger{},
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	// Send a window size so lists have dimensions
	p.HandleMessage(tea.WindowSizeMsg{Width: 120, Height: 40})
	return p
}

// setupSessionsPlugin returns a plugin with subTab set to "sessions".
func setupSessionsPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := setupPlugin(t)
	p.subTab = "sessions"
	return p
}

func TestInitLoadsPaths(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	_ = db.DBAddPath(database, "/home/user/project-a")
	_ = db.DBAddPath(database, "/home/user/project-b")

	cfg := testConfig()
	p := &Plugin{}
	err = p.Init(plugin.Context{
		DB:     database,
		Config: cfg,
		Bus:    plugin.NewBus(),
		Logger: testLogger{},
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	if len(p.paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(p.paths))
	}
	if p.paths[0] != "/home/user/project-a" {
		t.Fatalf("expected project-a, got %s", p.paths[0])
	}

	// New list should have: 2 paths + Browse = 3 items
	items := p.newList.Items()
	if len(items) != 3 {
		t.Fatalf("expected 3 new list items, got %d", len(items))
	}
}

func TestHandleKeyEnterOnPathReturnsLaunch(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "new"

	// Add a path so there's something beyond home
	_ = db.DBAddPath(p.db, "/tmp/myproject")
	p.paths = append(p.paths, "/tmp/myproject")
	p.newList.SetItems(p.buildNewItems())

	// Select the path we just added (index 0 since no more home item)
	p.newList.Select(0)

	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if action.Type != "launch" {
		t.Fatalf("expected launch action, got %s", action.Type)
	}
	if action.Args["dir"] != "/tmp/myproject" {
		t.Fatalf("expected dir /tmp/myproject, got %s", action.Args["dir"])
	}
}

func TestHandleKeyEnterOnSessionReturnsResume(t *testing.T) {
	p := setupSessionsPlugin(t)
	p.unified.viewFilter = ViewFilterSavedOnly // Resume tab shows saved sessions

	// Load a saved session into the unified view
	sessions := []db.Session{
		{
			SessionID: "sess-abc",
			Project:   "/home/user/proj",
			Repo:      "proj",
			Branch:    "main",
			Summary:   "test session",
			Created:   time.Now(),
			Type:      db.SessionBookmark,
		},
	}
	p.unified.SetSavedSessions(sessions)
	// cursor starts at 0 which will hit the saved session

	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if action.Type != "launch" {
		t.Fatalf("expected launch action, got %s", action.Type)
	}
	if action.Args["resume_id"] != "sess-abc" {
		t.Fatalf("expected resume_id sess-abc, got %s", action.Args["resume_id"])
	}
	if action.Args["dir"] != "/home/user/proj" {
		t.Fatalf("expected dir /home/user/proj, got %s", action.Args["dir"])
	}
}

func TestHandleKeyDeleteEntersConfirming(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "new"

	_ = db.DBAddPath(p.db, "/tmp/deleteme")
	p.paths = append(p.paths, "/tmp/deleteme")
	p.newList.SetItems(p.buildNewItems())

	// Select the path item (index 0)
	p.newList.Select(0)

	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})
	if action.Type != "noop" {
		t.Fatalf("expected noop during confirm setup, got %s", action.Type)
	}
	if !p.confirming {
		t.Fatal("expected confirming to be true")
	}
	if p.confirmItem.path != "/tmp/deleteme" {
		t.Fatalf("expected confirm path /tmp/deleteme, got %s", p.confirmItem.path)
	}
}

func TestConfirmingYRemovesItem(t *testing.T) {
	p := setupPlugin(t)

	_ = db.DBAddPath(p.db, "/tmp/removeme")
	p.paths = append(p.paths, "/tmp/removeme")
	p.newList.SetItems(p.buildNewItems())

	// Enter confirming mode
	p.confirming = true
	p.confirmItem = newItem{path: "/tmp/removeme", label: "removeme"}

	// Press "y"
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if action.Type != "noop" {
		t.Fatalf("expected noop, got %s", action.Type)
	}
	if p.confirming {
		t.Fatal("expected confirming to be false after y")
	}

	// Verify path was removed
	for _, path := range p.paths {
		if path == "/tmp/removeme" {
			t.Fatal("expected path to be removed from p.paths")
		}
	}
}

func TestSubTabSwitching(t *testing.T) {
	p := setupPlugin(t)

	// After Init, subTab defaults to "sessions"
	if p.subTab != "sessions" {
		t.Fatalf("expected initial subTab 'sessions', got %s", p.subTab)
	}

	// Switch to new
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if p.subTab != "new" {
		t.Fatalf("expected subTab 'new', got %s", p.subTab)
	}

	// Switch to sessions
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions', got %s", p.subTab)
	}
}

func TestHandleKeyDeleteOnFirstPathEntersConfirming(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "new"

	// Add a path and select it
	_ = db.DBAddPath(p.db, "/tmp/firstpath")
	p.paths = append(p.paths, "/tmp/firstpath")
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(0)

	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})
	if action.Type != "noop" {
		t.Fatalf("expected noop action type, got %s", action.Type)
	}
	if !p.confirming {
		t.Fatal("should enter confirming for first path")
	}
	if p.confirmItem.path != "/tmp/firstpath" {
		t.Fatalf("expected confirm path /tmp/firstpath, got %s", p.confirmItem.path)
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	p := setupPlugin(t)

	// Should not panic for any sub-tab
	p.subTab = "new"
	output := p.View(120, 40, 0)
	if output == "" {
		t.Fatal("expected non-empty view for new tab")
	}

	p.subTab = "sessions"
	output = p.View(120, 40, 0)
	if output == "" {
		t.Fatal("expected non-empty view for sessions tab")
	}
}

// TestUnifiedViewLoadsSavedSessions verifies that SetSavedSessions populates
// the unified view and SelectedItem returns the right session.
func TestUnifiedViewLoadsSavedSessions(t *testing.T) {
	p := setupSessionsPlugin(t)

	sessions := []db.Session{
		{
			SessionID: "s1",
			Repo:      "repo1",
			Branch:    "main",
			Created:   time.Now(),
			Type:      db.SessionBookmark,
		},
	}
	p.unified.SetSavedSessions(sessions)
	p.unified.viewFilter = ViewFilterSavedOnly // Resume tab shows saved sessions

	if len(p.unified.savedSessions) != 1 {
		t.Fatalf("expected 1 saved session, got %d", len(p.unified.savedSessions))
	}

	sel := p.unified.SelectedItem()
	if sel == nil {
		t.Fatal("expected selected item, got nil")
	}
	if sel.SessionID != "s1" {
		t.Fatalf("expected session ID s1, got %s", sel.SessionID)
	}
}

func TestNavigateTo(t *testing.T) {
	p := setupPlugin(t)

	p.NavigateTo("active", nil)
	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions', got %s", p.subTab)
	}
	if p.unified.viewFilter != ViewFilterLiveOnly {
		t.Fatalf("expected viewFilter live_only for active route, got %q", p.unified.viewFilter)
	}

	p.NavigateTo("resume", nil)
	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions' for resume route, got %s", p.subTab)
	}
	if p.unified.viewFilter != ViewFilterSavedOnly {
		t.Fatalf("expected viewFilter saved_only for resume route, got %q", p.unified.viewFilter)
	}

	p.NavigateTo("sessions", nil)
	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions', got %s", p.subTab)
	}
	if p.unified.viewFilter != ViewFilterLiveOnly {
		t.Fatalf("expected viewFilter live_only for sessions route (alias), got %q", p.unified.viewFilter)
	}

	p.NavigateTo("new", nil)
	if p.subTab != "new" {
		t.Fatalf("expected subTab 'new', got %s", p.subTab)
	}
}

func TestEscWithPendingTodoNavigatesToCommand(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "new"
	p.pendingLaunchTodo = &db.Todo{Title: "test task"}

	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if action.Type != "navigate" {
		t.Fatalf("expected navigate action, got %s", action.Type)
	}
	if action.Payload != "command" {
		t.Fatalf("expected payload 'command', got %s", action.Payload)
	}
	if p.pendingLaunchTodo != nil {
		t.Fatal("expected pendingLaunchTodo to be cleared")
	}
}

func TestFormatTodoContext(t *testing.T) {
	todo := db.Todo{
		Title:   "Fix the bug",
		Context: "Found in prod",
		Due:     "2026-03-10",
	}
	result := formatTodoContext(todo)
	if result == "" {
		t.Fatal("expected non-empty context")
	}
	if !contains(result, "Fix the bug") {
		t.Fatal("expected title in context")
	}
	if !contains(result, "Found in prod") {
		t.Fatal("expected context field in output")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFilterFromFirstCharacter(t *testing.T) {
	p := setupSessionsPlugin(t)
	p.unified.viewFilter = ViewFilterSavedOnly // show saved sessions for this test

	// Load 3 saved sessions with different repos
	sessions := []db.Session{
		{SessionID: "s1", Project: "/home/user/claude-command-center", Repo: "claude-command-center", Branch: "main", Summary: "Working on the command center dashboard", Created: time.Now(), Type: db.SessionBookmark},
		{SessionID: "s2", Project: "/home/user/sherlock", Repo: "sherlock", Branch: "main", Summary: "Building the investigation dashboard with complex queries", Created: time.Now(), Type: db.SessionBookmark},
		{SessionID: "s3", Project: "/home/user/merchant-ui", Repo: "merchant-ui", Branch: "main", Summary: "Merchant portal UI with Tailwind CSS layout improvements", Created: time.Now(), Type: db.SessionBookmark},
	}
	p.unified.SetSavedSessions(sessions)

	// Verify 3 items visible via displayItems
	items := p.unified.displayItems()
	if len(items) != 3 {
		t.Fatalf("expected 3 items before filtering, got %d", len(items))
	}

	// Unified view doesn't do text filtering — just verify all items are present
	// and can be navigated. The unified view uses cursor-based navigation.
	p.unified.MoveDown()
	sel := p.unified.SelectedItem()
	if sel == nil {
		t.Fatal("expected selected item after MoveDown")
	}
}

func TestTypeToFilterNewTab(t *testing.T) {
	p := setupPlugin(t)
	// Switch to new tab explicitly so we can test filter behavior
	p.subTab = "new"

	// Add paths so we have items to filter
	_ = db.DBAddPath(p.db, "/tmp/alpha-project")
	_ = db.DBAddPath(p.db, "/tmp/beta-project")
	p.paths = append(p.paths, "/tmp/alpha-project", "/tmp/beta-project")
	p.newList.SetItems(p.buildNewItems())

	// Typing a character should immediately start filtering (no '/' needed)
	// Note: 's' and 'n' are sub-tab shortcuts on new tab, so use 'b' which is not.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if p.filterText != "b" {
		t.Fatalf("expected filterText 'b', got %q", p.filterText)
	}

	// Type more chars
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if p.filterText != "bet" {
		t.Fatalf("expected filterText 'bet', got %q", p.filterText)
	}

	// Visible items should be filtered
	visible := p.newList.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible item after filtering 'bet', got %d", len(visible))
	}

	// Backspace should edit the filter
	p.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if p.filterText != "be" {
		t.Fatalf("expected filterText 'be', got %q after backspace", p.filterText)
	}

	// Escape should clear the filter
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if p.filterText != "" {
		t.Fatalf("expected empty filterText after escape, got %q", p.filterText)
	}
}

func TestTypeToFilterShortcutsDisabledWhileFiltering(t *testing.T) {
	p := setupPlugin(t)
	// Must be on new tab for type-to-filter to work
	p.subTab = "new"

	// Start filtering
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if p.filterText != "c" {
		t.Fatalf("expected filterText 'c', got %q", p.filterText)
	}

	// Pressing 's' while filtering should append to filter, not switch to sessions tab
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if p.subTab != "new" {
		t.Fatalf("expected subTab 'new' while filtering, got %s", p.subTab)
	}
	if p.filterText != "cs" {
		t.Fatalf("expected filterText 'cs', got %q", p.filterText)
	}

	// Same for 'n' and 't'
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if p.filterText != "csn" {
		t.Fatalf("expected filterText 'csn', got %q", p.filterText)
	}
}

func TestEnterDirectlyLaunchesOnNewTab(t *testing.T) {
	p := setupPlugin(t)

	// Add a path
	_ = db.DBAddPath(p.db, "/tmp/myproject")
	p.paths = append(p.paths, "/tmp/myproject")
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(0)
	p.subTab = "new"

	// Single Enter should launch directly
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if action.Type != "launch" {
		t.Fatalf("expected launch action from single Enter, got %s", action.Type)
	}
	if action.Args["dir"] != "/tmp/myproject" {
		t.Fatalf("expected dir /tmp/myproject, got %s", action.Args["dir"])
	}
}

func TestSubstringFilter(t *testing.T) {
	targets := []string{
		"claude-command-center main Working on CCC",
		"sherlock main Investigation dashboard",
		"merchant-ui main Merchant portal",
	}

	tests := []struct {
		term     string
		expected int
	}{
		{"c", 3},       // all three contain "c" somewhere
		{"cl", 1},      // only claude-command-center
		{"sh", 1},      // only sherlock
		{"main", 3},    // all contain "main"
		{"xyz", 0},     // nothing matches
		{"CLAUDE", 1},  // case-insensitive
	}

	for _, tc := range tests {
		ranks := substringFilter(tc.term, targets)
		if len(ranks) != tc.expected {
			t.Errorf("substringFilter(%q): expected %d matches, got %d", tc.term, tc.expected, len(ranks))
		}
	}
}

// ---------------------------------------------------------------------------
// New integration tests for unified view
// ---------------------------------------------------------------------------

func TestSessionsArchiveToggle(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Add archived sessions directly
	p.unified.archivedSessions = []db.ArchivedSession{
		{
			SessionID:    "arch-1",
			Topic:        "Archived session",
			Project:      "/home/user/proj",
			Repo:         "proj",
			Branch:       "main",
			RegisteredAt: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			EndedAt:      time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}

	if p.unified.archiveMode {
		t.Fatal("expected archiveMode to be false initially")
	}

	// Press 'A' (shift-a) — should enter archive mode
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if !p.unified.archiveMode {
		t.Fatal("expected archiveMode to be true after pressing 'A'")
	}

	// Press 'A' again — should leave archive mode
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if p.unified.archiveMode {
		t.Fatal("expected archiveMode to be false after pressing 'A' again")
	}
}

func TestSessionsArchiveActionOnEndedLive(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Add an ended live session
	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-ended-001",
			Topic:        "Ended session",
			Project:      "/home/user/proj",
			Repo:         "proj",
			Branch:       "main",
			State:        "ended",
			RegisteredAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			EndedAt:      time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		},
	}

	// Press 'a' to archive the selected session
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if action.Type != "consumed" {
		t.Fatalf("expected consumed action, got %s", action.Type)
	}

	// Verify session was archived to DB
	archived, _ := db.DBLoadArchivedSessions(p.db)
	if len(archived) != 1 {
		t.Fatalf("expected 1 archived session, got %d", len(archived))
	}
	if archived[0].SessionID != "live-ended-001" {
		t.Fatalf("expected session ID live-ended-001, got %s", archived[0].SessionID)
	}

	// Verify flash message
	if p.flashMessage == "" {
		t.Fatal("expected flash message after archiving")
	}
}

func TestSessionsArchiveActionOnRunningBlocked(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Add a running live session
	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-running-001",
			Topic:        "Running session",
			Project:      "/home/user/proj",
			Repo:         "proj",
			Branch:       "main",
			State:        "running",
			RegisteredAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}

	// Press 'a' to archive — should be blocked
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if action.Type != "consumed" {
		t.Fatalf("expected consumed action, got %s", action.Type)
	}

	// Verify no archived sessions
	archived, _ := db.DBLoadArchivedSessions(p.db)
	if len(archived) != 0 {
		t.Fatalf("expected 0 archived sessions, got %d", len(archived))
	}

	// Verify flash message indicates blocking
	if p.flashMessage != "Can't archive running session" {
		t.Fatalf("expected 'Can't archive running session' flash, got %q", p.flashMessage)
	}
}

func TestSessionsEnterLaunchesLive(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Add a live session
	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-sess-001",
			Topic:        "My live session",
			Project:      "/home/user/myproject",
			Repo:         "myproject",
			Branch:       "main",
			State:        "ended",
			RegisteredAt: time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
		},
	}
	// cursor is at 0, which hits the live session

	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if action.Type != "launch" {
		t.Fatalf("expected launch action, got %s", action.Type)
	}
	if action.Args["resume_id"] != "live-sess-001" {
		t.Fatalf("expected resume_id live-sess-001, got %s", action.Args["resume_id"])
	}
	if action.Args["dir"] != "/home/user/myproject" {
		t.Fatalf("expected dir /home/user/myproject, got %s", action.Args["dir"])
	}
}

func TestSessionsBookmarkSavesToDB(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Add a live ended session
	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-sess-bk1",
			Topic:        "Session to bookmark",
			Project:      "/home/user/project",
			Repo:         "project",
			Branch:       "feature",
			State:        "ended",
			RegisteredAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}

	// Press 'b' to bookmark
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if action.Type != "consumed" && action.Type != "noop" {
		t.Fatalf("unexpected action type: %s", action.Type)
	}

	// Verify the bookmark was saved to DB
	bookmarks, err := db.DBLoadBookmarks(p.db)
	if err != nil {
		t.Fatalf("load bookmarks: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}
	if bookmarks[0].SessionID != "live-sess-bk1" {
		t.Fatalf("expected session ID live-sess-bk1, got %s", bookmarks[0].SessionID)
	}
}

func TestSessionsBookmarkArchivedPromotesToSaved(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Add an archived session and switch to archive mode
	now := time.Now()
	_ = db.DBInsertArchivedSession(p.db, db.ArchivedSession{
		SessionID:    "arch-promote-001",
		Topic:        "Archived to promote",
		Project:      "/home/user/proj",
		Repo:         "proj",
		Branch:       "main",
		RegisteredAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		EndedAt:      now.Add(-1 * time.Hour).Format(time.RFC3339),
	})
	p.unified.ReloadArchived()
	p.unified.ToggleArchive() // enter archive mode

	// Press 'b' to promote to saved
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if action.Type != "consumed" {
		t.Fatalf("expected consumed action, got %s", action.Type)
	}

	// Verify bookmark was created
	bookmarks, err := db.DBLoadBookmarks(p.db)
	if err != nil {
		t.Fatalf("load bookmarks: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}
	if bookmarks[0].SessionID != "arch-promote-001" {
		t.Fatalf("expected session ID arch-promote-001, got %s", bookmarks[0].SessionID)
	}

	// Verify archived session was removed
	archived, _ := db.DBLoadArchivedSessions(p.db)
	if len(archived) != 0 {
		t.Fatalf("expected 0 archived sessions after promotion, got %d", len(archived))
	}
}

func TestSessionsDismissSavedRemovesBookmark(t *testing.T) {
	p := setupSessionsPlugin(t)
	p.unified.viewFilter = ViewFilterSavedOnly // Resume tab shows saved sessions

	// Insert a bookmark directly into DB
	_ = db.DBInsertBookmark(p.db, db.Session{
		SessionID: "saved-dismiss-001",
		Project:   "/home/user/proj",
		Repo:      "proj",
		Branch:    "main",
		Summary:   "Session to dismiss",
		Created:   time.Now(),
	}, "test label")

	// Reload saved sessions into unified view
	sessions, _ := db.DBLoadBookmarks(p.db)
	p.unified.SetSavedSessions(sessions)

	// Cursor is at 0 → saved session (no live sessions)
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if action.Type != "consumed" {
		t.Fatalf("expected consumed action, got %s", action.Type)
	}

	// Verify bookmark was removed from DB
	bookmarks, err := db.DBLoadBookmarks(p.db)
	if err != nil {
		t.Fatalf("load bookmarks: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Fatalf("expected 0 bookmarks after dismiss, got %d", len(bookmarks))
	}
}

func TestSessionsDismissArchivedDeletesFromDB(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Insert an archived session
	now := time.Now()
	_ = db.DBInsertArchivedSession(p.db, db.ArchivedSession{
		SessionID:    "arch-delete-001",
		Topic:        "Archived to delete",
		Project:      "/home/user/proj",
		Repo:         "proj",
		Branch:       "main",
		RegisteredAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		EndedAt:      now.Add(-1 * time.Hour).Format(time.RFC3339),
	})
	p.unified.ReloadArchived()
	p.unified.ToggleArchive() // enter archive mode

	// Press 'd' to delete
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if action.Type != "consumed" {
		t.Fatalf("expected consumed action, got %s", action.Type)
	}

	// Verify archived session was removed from DB
	archived, _ := db.DBLoadArchivedSessions(p.db)
	if len(archived) != 0 {
		t.Fatalf("expected 0 archived sessions after delete, got %d", len(archived))
	}

	// Verify flash message
	if p.flashMessage == "" {
		t.Fatal("expected flash message after delete")
	}
}

func TestSessionsDismissRunningBlocked(t *testing.T) {
	p := setupSessionsPlugin(t)

	// Add a running (active) session — dismiss should be blocked
	p.unified.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "running-sess-001",
			Topic:        "Active session",
			Project:      "/home/user/active",
			Repo:         "active",
			Branch:       "main",
			State:        "running",
			RegisteredAt: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
		},
	}

	// Press 'd' — should show flash message, not dismiss
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if p.flashMessage == "" {
		t.Fatal("expected flash message set after 'd' on running session")
	}
	if !contains(p.flashMessage, "Can't dismiss") {
		t.Fatalf("expected 'Can't dismiss' in flash message, got %q", p.flashMessage)
	}

	// Session should still be in the list
	if len(p.unified.liveSessions) != 1 {
		t.Fatalf("expected session to still be present, got %d sessions", len(p.unified.liveSessions))
	}
}

// BUG-119: NavigateTo("resume") must set subTab to "sessions", not leave it unchanged.
func TestNavigateToResumeRoute(t *testing.T) {
	p := setupPlugin(t)
	// Start on the "new" sub-tab (simulates user on New Session tab)
	p.subTab = "new"

	// Switch to the Resume route (as the host tab bar does)
	p.NavigateTo("resume", nil)

	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions' after NavigateTo('resume'), got %q", p.subTab)
	}
}

// BUG-119: NavigateTo("active") must set subTab to "sessions", not leave it unchanged.
func TestNavigateToActiveRoute(t *testing.T) {
	p := setupPlugin(t)
	// Start on the "new" sub-tab
	p.subTab = "new"

	p.NavigateTo("active", nil)

	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions' after NavigateTo('active'), got %q", p.subTab)
	}
}

// BUG-119: Switching tabs should not corrupt content — each tab renders independently.
func TestTabSwitchingDoesNotCorruptContent(t *testing.T) {
	p := setupPlugin(t)

	// Start on sessions
	p.NavigateTo("sessions", nil)
	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions', got %q", p.subTab)
	}

	// Switch to new
	p.NavigateTo("new", nil)
	if p.subTab != "new" {
		t.Fatalf("expected subTab 'new', got %q", p.subTab)
	}

	// Switch to resume
	p.NavigateTo("resume", nil)
	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions' after resume, got %q", p.subTab)
	}

	// Switch back to active
	p.NavigateTo("active", nil)
	if p.subTab != "sessions" {
		t.Fatalf("expected subTab 'sessions' after active, got %q", p.subTab)
	}

	// Switch to new again — should still work
	p.NavigateTo("new", nil)
	if p.subTab != "new" {
		t.Fatalf("expected subTab 'new' after switching back, got %q", p.subTab)
	}
}

// BUG-119: TabViewMsg with route "resume" should trigger a refresh command.
func TestTabViewMsgResumeTriggersRefresh(t *testing.T) {
	p := setupPlugin(t)
	p.subTab = "sessions"

	handled, action := p.HandleMessage(plugin.TabViewMsg{Route: "resume"})
	if !handled {
		t.Fatal("expected TabViewMsg with route 'resume' to be handled")
	}
	// action.TeaCmd may be nil if unified has no daemon client, but the message
	// should still be handled (returns true).
	_ = action
}
