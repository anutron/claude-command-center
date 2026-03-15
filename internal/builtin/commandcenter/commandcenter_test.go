package commandcenter

import (
	"database/sql"
	"strings"
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
			{ID: "t1", Title: "First todo", Status: "active", Source: "manual", TriageStatus: "accepted", CreatedAt: time.Now()},
			{ID: "t2", Title: "Second todo", Status: "active", Source: "manual", TriageStatus: "accepted", CreatedAt: time.Now()},
			{ID: "t3", Title: "Third todo", Status: "active", Source: "manual", TriageStatus: "accepted", CreatedAt: time.Now()},
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

func TestEnterOpensDetailView(t *testing.T) {
	p := testPluginWithCC(t)

	_ = p.HandleKey(keyMsg("enter"))
	if !p.detailView {
		t.Error("enter should open detail view")
	}
	if p.detailTodoID != p.cc.ActiveTodos()[0].ID {
		t.Errorf("detailTodoID = %q, want first active todo ID", p.detailTodoID)
	}
	if p.detailMode != "viewing" {
		t.Errorf("detailMode = %q, want %q", p.detailMode, "viewing")
	}
	if p.detailSelectedField != 0 {
		t.Errorf("detailSelectedField = %d, want 0", p.detailSelectedField)
	}
}

func TestOpenLaunchOnTodoWithProjectDir(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	action := p.HandleKey(keyMsg("o"))
	// Should enter detail view + task runner, NOT launch directly
	if action.Type != "noop" {
		t.Errorf("o on todo with project dir: type = %q, want %q", action.Type, "noop")
	}
	if !p.detailView {
		t.Error("detailView should be true")
	}
	if !p.taskRunnerView {
		t.Error("taskRunnerView should be true")
	}
}

func TestOpenLaunchOnTodoWithSessionID(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].SessionID = "abc12345-session-id"
	p.cc.Todos[0].ProjectDir = "/tmp/proj"

	action := p.HandleKey(keyMsg("o"))
	if action.Type != "launch" {
		t.Errorf("o on todo with session: type = %q, want %q", action.Type, "launch")
	}
	if action.Args["resume_id"] != "abc12345-session-id" {
		t.Errorf("resume_id = %q, want %q", action.Args["resume_id"], "abc12345-session-id")
	}
}

func TestOpenLaunchOnTodoWithoutProjectDir(t *testing.T) {
	p := testPluginWithCC(t)
	// No project dir, no session ID

	action := p.HandleKey(keyMsg("o"))
	// Should enter detail view + task runner, NOT navigate to sessions
	if action.Type != "noop" {
		t.Errorf("o on todo without project dir: type = %q, want %q", action.Type, "noop")
	}
	if !p.detailView {
		t.Error("detailView should be true")
	}
	if !p.taskRunnerView {
		t.Error("taskRunnerView should be true")
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

func TestDisplayContext(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"some plain context", "some plain context"},
		{"https://thanx.slack.com/archives/G01MA920F9/p1773165648549789?thread_ts=1771862390.043209&cid=G01MA920F9", "Slack"},
		{"https://mycompany.slack.com/archives/C01ABC/p123456", "Slack"},
		{"https://workspace.slack.com/messages/general", "Slack"},
		{"https://github.com/owner/repo/issues/42", "GitHub"},
		// Slack channel with description (BUG-074)
		{"#proj-dashboard-permissions-via-rbac – RBAC feature is in QA/bug bash phase, these items are non-blocking but needed in parallel", "Slack: #proj-dashboard-permissions-vi..."},
		{"#general – Company announcements", "Slack: #general"},
		{"#general - Company announcements", "Slack: #general"},
		{"#my-channel", "Slack: #my-channel"},
		// Long plain text gets truncated
		{"this is a very long context string that should be truncated to forty chars", "this is a very long context string th..."},
		// Short plain text passes through
		{"short", "short"},
	}

	for _, tt := range tests {
		got := displayContext(tt.input)
		if got != tt.want {
			t.Errorf("displayContext(%q) = %q, want %q", tt.input, got, tt.want)
		}
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

func TestDetailViewCommandInput(t *testing.T) {
	p := testPluginWithCC(t)

	// Enter detail view
	_ = p.HandleKey(keyMsg("enter"))
	if !p.detailView {
		t.Fatal("enter should open detail view")
	}
	if p.detailMode != "viewing" {
		t.Fatalf("detailMode = %q, want viewing", p.detailMode)
	}

	// Press c for command input
	action := p.HandleKey(keyMsg("c"))
	if p.detailMode != "commandInput" {
		t.Errorf("after c: detailMode = %q, want commandInput", p.detailMode)
	}
	if action.TeaCmd == nil {
		t.Error("c should return a TeaCmd (blink)")
	}

	// Verify the view renders the command input section
	view := p.View(120, 40, 0)
	if !strings.Contains(view, "Tell me what changed") {
		t.Error("detail view in commandInput mode should show 'Tell me what changed' label")
	}
	if !strings.Contains(view, "submit to AI") {
		t.Error("detail view in commandInput mode should show 'submit to AI' hint")
	}
}

func TestTaskRunnerStepNavigation(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	// Enter task runner via detail view
	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerView {
		t.Fatal("taskRunnerView should be true after 'o'")
	}
	if p.taskRunnerStep != 1 {
		t.Fatalf("initial step = %d, want 1", p.taskRunnerStep)
	}

	// Enter advances step 1 -> 2
	p.HandleKey(keyMsg("enter"))
	if p.taskRunnerStep != 2 {
		t.Errorf("after enter at step 1: step = %d, want 2", p.taskRunnerStep)
	}

	// Enter advances step 2 -> 3
	p.HandleKey(keyMsg("enter"))
	if p.taskRunnerStep != 3 {
		t.Errorf("after enter at step 2: step = %d, want 3", p.taskRunnerStep)
	}

	// Esc goes back step 3 -> 2
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.taskRunnerStep != 2 {
		t.Errorf("after esc at step 3: step = %d, want 2", p.taskRunnerStep)
	}

	// Esc goes back step 2 -> 1
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.taskRunnerStep != 1 {
		t.Errorf("after esc at step 2: step = %d, want 1", p.taskRunnerStep)
	}

	// Esc at step 1 exits task runner view
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.taskRunnerView {
		t.Error("esc at step 1 should exit taskRunnerView")
	}
}

func TestTaskRunnerModeCycling(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	// Enter task runner and advance to step 2
	p.HandleKey(keyMsg("o"))
	p.HandleKey(keyMsg("enter"))
	if p.taskRunnerStep != 2 {
		t.Fatalf("step = %d, want 2", p.taskRunnerStep)
	}

	// Default mode is "normal"
	if p.taskRunnerMode != "normal" {
		t.Fatalf("initial mode = %q, want %q", p.taskRunnerMode, "normal")
	}

	// Right arrow cycles normal -> worktree
	p.HandleKey(keyMsg("right"))
	if p.taskRunnerMode != "worktree" {
		t.Errorf("after right: mode = %q, want %q", p.taskRunnerMode, "worktree")
	}

	// Right again: worktree -> sandbox
	p.HandleKey(keyMsg("right"))
	if p.taskRunnerMode != "sandbox" {
		t.Errorf("after right right: mode = %q, want %q", p.taskRunnerMode, "sandbox")
	}

	// Right wraps: sandbox -> normal
	p.HandleKey(keyMsg("right"))
	if p.taskRunnerMode != "normal" {
		t.Errorf("after right wrap: mode = %q, want %q", p.taskRunnerMode, "normal")
	}

	// Left wraps: normal -> sandbox
	p.HandleKey(keyMsg("left"))
	if p.taskRunnerMode != "sandbox" {
		t.Errorf("after left wrap: mode = %q, want %q", p.taskRunnerMode, "sandbox")
	}
}

func TestTaskRunnerLaunchQueue(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	// Enter task runner, advance to step 3
	p.HandleKey(keyMsg("o"))
	p.HandleKey(keyMsg("enter")) // step 1 -> 2
	p.HandleKey(keyMsg("enter")) // step 2 -> 3

	// Default launch cursor is 0 (Queue)
	if p.taskRunnerLaunchCursor != 0 {
		t.Fatalf("initial launch cursor = %d, want 0", p.taskRunnerLaunchCursor)
	}

	// Enter at cursor 0 should queue (not immediate)
	action := p.HandleKey(keyMsg("enter"))
	if p.taskRunnerView {
		t.Error("task runner should be closed after launch")
	}
	if action.TeaCmd == nil {
		t.Error("launch should return a TeaCmd")
	}
	if !strings.Contains(p.flashMessage, "queued") && !strings.Contains(p.flashMessage, "launched") {
		t.Errorf("flash message = %q, want to contain 'queued' or 'launched'", p.flashMessage)
	}
}

func TestTaskRunnerLaunchRunNow(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	// Enter task runner, advance to step 3
	p.HandleKey(keyMsg("o"))
	p.HandleKey(keyMsg("enter")) // step 1 -> 2
	p.HandleKey(keyMsg("enter")) // step 2 -> 3

	// Move launch cursor to 1 (Run Now)
	p.HandleKey(keyMsg("right"))
	if p.taskRunnerLaunchCursor != 1 {
		t.Fatalf("launch cursor = %d, want 1", p.taskRunnerLaunchCursor)
	}

	// Enter at cursor 1 should launch immediately
	action := p.HandleKey(keyMsg("enter"))
	if p.taskRunnerView {
		t.Error("task runner should be closed after launch")
	}
	if action.TeaCmd == nil {
		t.Error("launch should return a TeaCmd")
	}
}

func TestTaskRunnerRefineKey(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	// Enter task runner, advance to step 3
	p.HandleKey(keyMsg("o"))
	p.HandleKey(keyMsg("enter")) // step 1 -> 2
	p.HandleKey(keyMsg("enter")) // step 2 -> 3

	// Press 'c' to refine
	p.HandleKey(keyMsg("c"))
	if !p.taskRunnerRefining {
		t.Error("'c' at step 3 should set taskRunnerRefining to true")
	}
}

func TestParseDueDate(t *testing.T) {
	// Fixed "now" for deterministic tests: 2026-03-14
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		input   string
		want    string
		wantOK  bool
	}{
		// Already YYYY-MM-DD
		{"2026-04-01", "2026-04-01", true},
		{"2025-12-25", "2025-12-25", true},

		// mm dd format — future date in current year
		{"03 20", "2026-03-20", true},
		{"3 20", "2026-03-20", true},
		{"04 01", "2026-04-01", true},
		{"12 25", "2026-12-25", true},

		// mm dd format — date already passed → next year
		{"01 05", "2027-01-05", true},
		{"03 13", "2027-03-13", true},

		// mm dd format — today is still valid (not past)
		{"03 14", "2026-03-14", true},

		// Invalid month/day
		{"13 01", "", false},
		{"00 15", "", false},
		{"03 32", "", false},

		// Natural language — should return false for LLM fallback
		{"wednesday", "", false},
		{"next friday", "", false},
		{"end of month", "", false},
		{"tomorrow", "", false},

		// Empty string
		{"", "", false},
	}

	for _, tt := range tests {
		got, ok := parseDueDate(tt.input, now)
		if ok != tt.wantOK {
			t.Errorf("parseDueDate(%q): ok = %v, want %v", tt.input, ok, tt.wantOK)
		}
		if ok && got != tt.want {
			t.Errorf("parseDueDate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

