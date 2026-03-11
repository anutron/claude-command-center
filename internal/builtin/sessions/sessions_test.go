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

	// New list should have: home + 2 paths + Browse = 4 items
	items := p.newList.Items()
	if len(items) != 4 {
		t.Fatalf("expected 4 new list items, got %d", len(items))
	}
}

func TestHandleKeyEnterOnPathReturnsLaunch(t *testing.T) {
	p := setupPlugin(t)

	// Add a path so there's something beyond home
	_ = db.DBAddPath(p.db, "/tmp/myproject")
	p.paths = append(p.paths, "/tmp/myproject")
	p.newList.SetItems(p.buildNewItems())

	// Select the second item (the path we just added)
	p.newList.Select(1)

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

	// Select the path item (index 1, since 0 is home)
	p.newList.Select(1)

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

func TestHandleKeyDeleteOnHomeEntersConfirming(t *testing.T) {
	p := setupPlugin(t)

	// Home item is at index 0 — should be deletable
	p.newList.Select(0)

	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})
	if action.Type != "noop" {
		t.Fatalf("expected noop action type, got %s", action.Type)
	}
	if !p.confirming {
		t.Fatal("should enter confirming for home item")
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
