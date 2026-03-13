package commandcenter

import (
	"database/sql"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// testDB opens an in-memory SQLite database for testing.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.OpenDB(":memory:")
	if err != nil {
		// Fallback: try without db.OpenDB if it doesn't support :memory:
		t.Fatalf("failed to open test db: %v", err)
	}
	return database
}

func testPlugin(t *testing.T) *Plugin {
	t.Helper()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	p := New()
	database := testDB(t)
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	ctx := plugin.Context{
		DB:     database,
		Config: cfg,
	}
	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return p
}

func testPluginWithCC(t *testing.T) *Plugin {
	t.Helper()
	p := testPlugin(t)
	p.cc = &db.CommandCenter{
		GeneratedAt: time.Now(),
		Todos: []db.Todo{
			{ID: "t1", Title: "First todo", Status: "active", Source: "manual", CreatedAt: time.Now()},
			{ID: "t2", Title: "Second todo", Status: "active", Source: "manual", CreatedAt: time.Now()},
			{ID: "t3", Title: "Third todo", Status: "active", Source: "manual", CreatedAt: time.Now()},
		},
		Threads: []db.Thread{
			{ID: "th1", Title: "Thread one", Status: "active", Type: "manual", CreatedAt: time.Now()},
			{ID: "th2", Title: "Thread two", Status: "paused", Type: "manual", CreatedAt: time.Now()},
		},
	}
	p.width = 120
	p.height = 40
	return p
}

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func specialKeyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func TestSlugAndTabName(t *testing.T) {
	p := New()
	if p.Slug() != "commandcenter" {
		t.Errorf("Slug() = %q, want %q", p.Slug(), "commandcenter")
	}
	if p.TabName() != "Command Center" {
		t.Errorf("TabName() = %q, want %q", p.TabName(), "Command Center")
	}
}

func TestRoutes(t *testing.T) {
	p := testPlugin(t)
	routes := p.Routes()
	if len(routes) != 2 {
		t.Fatalf("Routes() returned %d routes, want 2", len(routes))
	}
	if routes[0].Slug != "commandcenter" {
		t.Errorf("routes[0].Slug = %q, want %q", routes[0].Slug, "commandcenter")
	}
	if routes[1].Slug != "commandcenter/threads" {
		t.Errorf("routes[1].Slug = %q, want %q", routes[1].Slug, "commandcenter/threads")
	}
}

func TestInitLoadsCC(t *testing.T) {
	p := testPlugin(t)
	// With an empty DB, cc may be nil or empty
	// The important thing is that Init doesn't error
	if p.database == nil {
		t.Error("database should be set after Init")
	}
	if p.cfg == nil {
		t.Error("cfg should be set after Init")
	}
}

func TestNavigationUpDown(t *testing.T) {
	p := testPluginWithCC(t)

	// Start at cursor 0
	if p.ccCursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", p.ccCursor)
	}

	// Move down
	p.HandleKey(keyMsg("j"))
	if p.ccCursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", p.ccCursor)
	}

	p.HandleKey(keyMsg("j"))
	if p.ccCursor != 2 {
		t.Errorf("after j j: cursor = %d, want 2", p.ccCursor)
	}

	// Move up
	p.HandleKey(keyMsg("k"))
	if p.ccCursor != 1 {
		t.Errorf("after k: cursor = %d, want 1", p.ccCursor)
	}

	// Move up with "up"
	p.HandleKey(specialKeyMsg(tea.KeyUp))
	if p.ccCursor != 0 {
		t.Errorf("after up: cursor = %d, want 0", p.ccCursor)
	}

	// Don't go below 0
	p.HandleKey(keyMsg("k"))
	if p.ccCursor != 0 {
		t.Errorf("after k at 0: cursor = %d, want 0", p.ccCursor)
	}
}

func TestCompleteTodo(t *testing.T) {
	p := testPluginWithCC(t)

	activeBefore := len(p.cc.ActiveTodos())
	action := p.HandleKey(keyMsg("x"))
	activeAfter := len(p.cc.ActiveTodos())

	if activeAfter != activeBefore-1 {
		t.Errorf("after x: active todos = %d, want %d", activeAfter, activeBefore-1)
	}
	if action.TeaCmd == nil {
		t.Error("x should return a TeaCmd for DB write")
	}
	if len(p.undoStack) != 1 {
		t.Errorf("undo stack len = %d, want 1", len(p.undoStack))
	}
}

func TestDismissTodo(t *testing.T) {
	p := testPluginWithCC(t)

	activeBefore := len(p.cc.ActiveTodos())
	action := p.HandleKey(keyMsg("X"))
	activeAfter := len(p.cc.ActiveTodos())

	if activeAfter != activeBefore-1 {
		t.Errorf("after X: active todos = %d, want %d", activeAfter, activeBefore-1)
	}
	if action.TeaCmd == nil {
		t.Error("X should return a TeaCmd for DB write")
	}
}

func TestUndoCompletion(t *testing.T) {
	p := testPluginWithCC(t)

	activeBefore := len(p.cc.ActiveTodos())

	// Complete first todo
	p.HandleKey(keyMsg("x"))
	if len(p.cc.ActiveTodos()) != activeBefore-1 {
		t.Fatal("todo should be completed")
	}

	// Undo
	action := p.HandleKey(keyMsg("u"))
	if len(p.cc.ActiveTodos()) != activeBefore {
		t.Errorf("after undo: active todos = %d, want %d", len(p.cc.ActiveTodos()), activeBefore)
	}
	if p.flashMessage != "Undid last action" {
		t.Errorf("flash message = %q, want %q", p.flashMessage, "Undid last action")
	}
	if action.TeaCmd == nil {
		t.Error("undo should return a TeaCmd for DB write")
	}
}

func TestCreateTodoEntersRichMode(t *testing.T) {
	p := testPluginWithCC(t)

	action := p.HandleKey(keyMsg("c"))
	if !p.addingTodoRich {
		t.Error("c should enter addingTodoRich mode")
	}
	if action.TeaCmd == nil {
		t.Error("c should return a TeaCmd (textarea focus)")
	}
}

func TestEnterOnTodoWithProjectDir(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	action := p.HandleKey(keyMsg("enter"))
	if action.Type != "launch" {
		t.Errorf("enter on todo with project dir: type = %q, want %q", action.Type, "launch")
	}
	if action.Args["dir"] != "/tmp/myproject" {
		t.Errorf("launch dir = %q, want %q", action.Args["dir"], "/tmp/myproject")
	}
}

func TestEnterOnTodoWithSessionID(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].SessionID = "abc12345-session-id"
	p.cc.Todos[0].ProjectDir = "/tmp/proj"

	action := p.HandleKey(keyMsg("enter"))
	if action.Type != "launch" {
		t.Errorf("enter on todo with session: type = %q, want %q", action.Type, "launch")
	}
	if action.Args["resume_id"] != "abc12345-session-id" {
		t.Errorf("resume_id = %q, want %q", action.Args["resume_id"], "abc12345-session-id")
	}
}

func TestEnterOnTodoWithoutProjectDir(t *testing.T) {
	p := testPluginWithCC(t)
	// No project dir, no session ID

	action := p.HandleKey(keyMsg("enter"))
	if action.Type != "navigate" {
		t.Errorf("enter on todo without project dir: type = %q, want %q", action.Type, "navigate")
	}
	if action.Payload != "sessions" {
		t.Errorf("navigate payload = %q, want %q", action.Payload, "sessions")
	}
	if p.pendingLaunchTodo == nil {
		t.Error("pendingLaunchTodo should be set")
	}
}

func TestSubViewSwitching(t *testing.T) {
	p := testPluginWithCC(t)

	if p.subView != "command" {
		t.Fatalf("initial subView = %q, want %q", p.subView, "command")
	}

	p.NavigateTo("commandcenter/threads", nil)
	if p.subView != "threads" {
		t.Errorf("after NavigateTo threads: subView = %q, want %q", p.subView, "threads")
	}

	p.NavigateTo("commandcenter", nil)
	if p.subView != "command" {
		t.Errorf("after NavigateTo command: subView = %q, want %q", p.subView, "command")
	}
}

func TestThreadsNavigation(t *testing.T) {
	p := testPluginWithCC(t)
	p.NavigateTo("commandcenter/threads", nil)

	if p.threadCursor != 0 {
		t.Fatalf("initial thread cursor = %d, want 0", p.threadCursor)
	}

	p.HandleKey(keyMsg("j"))
	if p.threadCursor != 1 {
		t.Errorf("after j: thread cursor = %d, want 1", p.threadCursor)
	}

	p.HandleKey(keyMsg("k"))
	if p.threadCursor != 0 {
		t.Errorf("after k: thread cursor = %d, want 0", p.threadCursor)
	}
}

func TestDeferTodo(t *testing.T) {
	p := testPluginWithCC(t)
	firstID := p.cc.ActiveTodos()[0].ID

	action := p.HandleKey(keyMsg("d"))
	activeTodos := p.cc.ActiveTodos()
	lastActive := activeTodos[len(activeTodos)-1]
	if lastActive.ID != firstID {
		t.Errorf("deferred todo should be at end, got %q at end", lastActive.ID)
	}
	if action.TeaCmd == nil {
		t.Error("d should return a TeaCmd for DB write")
	}
}

func TestPromoteTodo(t *testing.T) {
	p := testPluginWithCC(t)
	p.ccCursor = 2
	lastID := p.cc.ActiveTodos()[2].ID

	action := p.HandleKey(keyMsg("p"))
	firstActive := p.cc.ActiveTodos()[0]
	if firstActive.ID != lastID {
		t.Errorf("promoted todo should be at top, got %q at top", firstActive.ID)
	}
	if p.ccCursor != 0 {
		t.Errorf("cursor should be 0 after promote, got %d", p.ccCursor)
	}
	if action.TeaCmd == nil {
		t.Error("p should return a TeaCmd for DB write")
	}
}

func TestToggleBacklog(t *testing.T) {
	p := testPluginWithCC(t)

	if p.showBacklog {
		t.Fatal("initial showBacklog should be false")
	}

	p.HandleKey(keyMsg("b"))
	if !p.showBacklog {
		t.Error("after b: showBacklog should be true")
	}

	p.HandleKey(keyMsg("b"))
	if p.showBacklog {
		t.Error("after b b: showBacklog should be false")
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	p := testPluginWithCC(t)

	// Command view
	output := p.View(120, 40, 0)
	if output == "" {
		t.Error("command view should not be empty")
	}

	// Threads view
	p.NavigateTo("commandcenter/threads", nil)
	output = p.View(120, 40, 0)
	if output == "" {
		t.Error("threads view should not be empty")
	}
}

func TestViewWithNilCC(t *testing.T) {
	p := testPlugin(t)
	p.cc = nil

	output := p.View(120, 40, 0)
	if output == "" {
		t.Error("view with nil CC should not be empty")
	}
}

func TestHelpOverlay(t *testing.T) {
	p := testPluginWithCC(t)

	p.HandleKey(keyMsg("?"))
	if !p.showHelp {
		t.Error("? should toggle help on")
	}

	output := p.View(120, 40, 0)
	if output == "" {
		t.Error("help overlay should not be empty")
	}

	// Any key dismisses
	p.HandleKey(keyMsg("q"))
	if p.showHelp {
		t.Error("any key should dismiss help")
	}
}

func TestBookingMode(t *testing.T) {
	p := testPluginWithCC(t)

	p.HandleKey(keyMsg("s"))
	if !p.bookingMode {
		t.Error("s should enter booking mode")
	}
	if p.bookingCursor != 2 {
		t.Errorf("initial booking cursor = %d, want 2", p.bookingCursor)
	}

	// Navigate booking
	p.HandleKey(keyMsg("l"))
	if p.bookingCursor != 3 {
		t.Errorf("after l: booking cursor = %d, want 3", p.bookingCursor)
	}

	p.HandleKey(keyMsg("h"))
	if p.bookingCursor != 2 {
		t.Errorf("after h: booking cursor = %d, want 2", p.bookingCursor)
	}

	// Esc cancels
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.bookingMode {
		t.Error("esc should cancel booking mode")
	}
}

func TestHandleMessageCCLoaded(t *testing.T) {
	p := testPlugin(t)
	newCC := &db.CommandCenter{
		GeneratedAt: time.Now(),
		Todos: []db.Todo{
			{ID: "new1", Title: "New todo", Status: "active"},
		},
	}

	handled, _ := p.HandleMessage(ccLoadedMsg{cc: newCC})
	if !handled {
		t.Error("ccLoadedMsg should be handled")
	}
	if p.cc == nil {
		t.Fatal("cc should be set after ccLoadedMsg")
	}
	if len(p.cc.Todos) != 1 {
		t.Errorf("cc.Todos len = %d, want 1", len(p.cc.Todos))
	}
}

func TestHandleMessageRefreshFinished(t *testing.T) {
	p := testPlugin(t)
	p.ccRefreshing = true

	handled, _ := p.HandleMessage(ccRefreshFinishedMsg{err: nil})
	if !handled {
		t.Error("ccRefreshFinishedMsg should be handled")
	}
	if p.ccRefreshing {
		t.Error("ccRefreshing should be false after refresh finished")
	}
}

func TestAddThread(t *testing.T) {
	p := testPluginWithCC(t)
	p.NavigateTo("commandcenter/threads", nil)

	// Enter add mode
	action := p.HandleKey(keyMsg("a"))
	if !p.addingThread {
		t.Error("a should enter addingThread mode")
	}
	if action.TeaCmd == nil {
		t.Error("a should return a TeaCmd (textinput blink)")
	}
}

func TestCloseThread(t *testing.T) {
	p := testPluginWithCC(t)
	p.NavigateTo("commandcenter/threads", nil)

	threadsBefore := len(p.cc.ActiveThreads()) + len(p.cc.PausedThreads())
	action := p.HandleKey(keyMsg("x"))
	threadsAfter := len(p.cc.ActiveThreads()) + len(p.cc.PausedThreads())

	if threadsAfter != threadsBefore-1 {
		t.Errorf("after x: total threads = %d, want %d", threadsAfter, threadsBefore-1)
	}
	if action.TeaCmd == nil {
		t.Error("x should return a TeaCmd for DB write")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"key": "value"}`, `{"key": "value"}`},
		{"```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{`some text {"key": "value"} more text`, `{"key": "value"}`},
	}

	for _, tt := range tests {
		got := extractJSON(tt.input)
		if got != tt.want {
			t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
