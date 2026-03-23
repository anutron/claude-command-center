package sessions

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
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
	p := setupPlugin(t)

	// Switch to resume tab and load a session
	p.subTab = "resume"
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
	p.loading = false
	p.resumeList.SetItems(buildSessionItems(sessions))
	p.resumeList.Select(0)

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

	if p.subTab != "new" {
		t.Fatalf("expected initial subTab 'new', got %s", p.subTab)
	}

	// Switch to resume
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if p.subTab != "resume" {
		t.Fatalf("expected subTab 'resume', got %s", p.subTab)
	}

	// Switch back to new
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if p.subTab != "new" {
		t.Fatalf("expected subTab 'new', got %s", p.subTab)
	}
}

func TestHandleKeyDeleteOnFirstPathEntersConfirming(t *testing.T) {
	p := setupPlugin(t)

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

	// Should not panic for either sub-tab
	output := p.View(120, 40, 0)
	if output == "" {
		t.Fatal("expected non-empty view for new tab")
	}

	p.subTab = "resume"
	output = p.View(120, 40, 0)
	if output == "" {
		t.Fatal("expected non-empty view for resume tab")
	}
}

func TestSessionsLoadedMsg(t *testing.T) {
	p := setupPlugin(t)

	if !p.loading {
		t.Fatal("expected loading to be true initially")
	}

	sessions := []db.Session{
		{
			SessionID: "s1",
			Repo:      "repo1",
			Branch:    "main",
			Created:   time.Now(),
			Type:      db.SessionBookmark,
		},
	}
	handled, _ := p.HandleMessage(sessionsLoadedMsg{sessions: sessions})
	if !handled {
		t.Fatal("expected sessionsLoadedMsg to be handled")
	}
	if p.loading {
		t.Fatal("expected loading to be false after sessionsLoadedMsg")
	}
	if len(p.resumeList.Items()) != 1 {
		t.Fatalf("expected 1 resume item, got %d", len(p.resumeList.Items()))
	}
}

func TestNavigateTo(t *testing.T) {
	p := setupPlugin(t)

	p.NavigateTo("resume", nil)
	if p.subTab != "resume" {
		t.Fatalf("expected subTab 'resume', got %s", p.subTab)
	}

	p.NavigateTo("new", nil)
	if p.subTab != "new" {
		t.Fatalf("expected subTab 'new', got %s", p.subTab)
	}
}

func TestEscWithPendingTodoNavigatesToCommand(t *testing.T) {
	p := setupPlugin(t)
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
	p := setupPlugin(t)

	// Switch to resume tab with 3 sessions whose summaries contain
	// scattered c/l/a characters (which would fuzzy-match "cla" but
	// should NOT substring-match it).
	p.subTab = "resume"
	sessions := []db.Session{
		{SessionID: "s1", Project: "/home/user/claude-command-center", Repo: "claude-command-center", Branch: "main", Summary: "Working on the command center dashboard", Created: time.Now(), Type: db.SessionBookmark},
		{SessionID: "s2", Project: "/home/user/sherlock", Repo: "sherlock", Branch: "main", Summary: "Building the investigation dashboard with complex queries", Created: time.Now(), Type: db.SessionBookmark},
		{SessionID: "s3", Project: "/home/user/merchant-ui", Repo: "merchant-ui", Branch: "main", Summary: "Merchant portal UI with Tailwind CSS layout improvements", Created: time.Now(), Type: db.SessionBookmark},
	}
	p.loading = false
	p.resumeList.SetItems(buildSessionItems(sessions))

	// Verify 3 items visible before filtering
	if len(p.resumeList.VisibleItems()) != 3 {
		t.Fatalf("expected 3 visible items before filtering, got %d", len(p.resumeList.VisibleItems()))
	}

	// Test progressive filtering: each additional character should narrow results
	tests := []struct {
		term     string
		expected int
	}{
		{"c", 3}, // all three contain "c" somewhere in their FilterValue
		{"cl", 1}, // only "claude-command-center" contains "cl" as substring
		{"cla", 1},
		{"clau", 1},
	}

	for _, tc := range tests {
		p.resumeList.SetFilterText(tc.term)
		visible := p.resumeList.VisibleItems()
		if len(visible) != tc.expected {
			var names []string
			for _, item := range visible {
				if si, ok := item.(sessionItem); ok {
					names = append(names, si.session.Repo)
				}
			}
			t.Errorf("filter %q: expected %d visible items, got %d %v",
				tc.term, tc.expected, len(visible), names)
		}
	}
}

func TestTypeToFilterNewTab(t *testing.T) {
	p := setupPlugin(t)

	// Add paths so we have items to filter
	_ = db.DBAddPath(p.db, "/tmp/alpha-project")
	_ = db.DBAddPath(p.db, "/tmp/beta-project")
	p.paths = append(p.paths, "/tmp/alpha-project", "/tmp/beta-project")
	p.newList.SetItems(p.buildNewItems())

	// Typing a character should immediately start filtering (no '/' needed)
	// Note: 'a' is a sub-tab shortcut, so use 'b' which is not a shortcut.
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

	// Start filtering
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if p.filterText != "c" {
		t.Fatalf("expected filterText 'c', got %q", p.filterText)
	}

	// Pressing 'r' while filtering should append to filter, not switch to resume tab
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if p.subTab != "new" {
		t.Fatalf("expected subTab 'new' while filtering, got %s", p.subTab)
	}
	if p.filterText != "cr" {
		t.Fatalf("expected filterText 'cr', got %q", p.filterText)
	}

	// Same for 'n' and 't'
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if p.filterText != "crn" {
		t.Fatalf("expected filterText 'crn', got %q", p.filterText)
	}
}

func TestEnterDirectlyLaunchesOnNewTab(t *testing.T) {
	p := setupPlugin(t)

	// Add a path
	_ = db.DBAddPath(p.db, "/tmp/myproject")
	p.paths = append(p.paths, "/tmp/myproject")
	p.newList.SetItems(p.buildNewItems())
	p.newList.Select(0)

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
		{"c", 3},  // all three contain "c" somewhere
		{"cl", 1}, // only claude-command-center
		{"sh", 1}, // only sherlock
		{"main", 3}, // all contain "main"
		{"xyz", 0},  // nothing matches
		{"CLAUDE", 1}, // case-insensitive
	}

	for _, tc := range tests {
		ranks := substringFilter(tc.term, targets)
		if len(ranks) != tc.expected {
			t.Errorf("substringFilter(%q): expected %d matches, got %d", tc.term, tc.expected, len(ranks))
		}
	}
}
