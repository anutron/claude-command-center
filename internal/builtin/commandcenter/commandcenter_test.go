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
	"github.com/charmbracelet/lipgloss"
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
			{ID: "t1", Title: "First todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
			{ID: "t2", Title: "Second todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
			{ID: "t3", Title: "Third todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
		},
	}
	p.width = 120
	p.height = 40
	return p
}

// insertTestPaths inserts paths into the cc_learned_paths table so that
// DBLoadPaths will return them. This is needed because enterTaskRunner reloads
// paths from the DB dynamically.
func insertTestPaths(t *testing.T, database *sql.DB, paths []string) {
	t.Helper()
	for i, p := range paths {
		_, err := database.Exec(`INSERT INTO cc_learned_paths (path, description, sort_order, added_at) VALUES (?, '', ?, datetime('now'))`, p, i)
		if err != nil {
			t.Fatalf("failed to insert test path %q: %v", p, err)
		}
	}
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
	if len(routes) != 1 {
		t.Fatalf("Routes() returned %d routes, want 1", len(routes))
	}
	if routes[0].Slug != "commandcenter" {
		t.Errorf("routes[0].Slug = %q, want %q", routes[0].Slug, "commandcenter")
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

	p.NavigateTo("commandcenter", nil)
	if p.subView != "command" {
		t.Errorf("after NavigateTo command: subView = %q, want %q", p.subView, "command")
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
			{ID: "new1", Title: "New todo", Status: db.StatusBacklog},
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

func TestCommandTextAreaWrapsText(t *testing.T) {
	p := testPluginWithCC(t)
	termWidth := 120

	// Enter detail view, then command input
	_ = p.HandleKey(keyMsg("enter"))
	_ = p.HandleKey(keyMsg("c"))
	if p.detailMode != "commandInput" {
		t.Fatalf("detailMode = %q, want commandInput", p.detailMode)
	}

	// Type a long string via HandleKey (like a real user typing)
	longText := strings.Repeat("x", 130) // Longer than textarea width
	for _, ch := range longText {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// Render the view
	view := p.View(termWidth, 40, 0)
	lines := strings.Split(view, "\n")

	// No rendered line should exceed the terminal width
	maxLineWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxLineWidth {
			maxLineWidth = w
		}
	}
	if maxLineWidth > termWidth {
		t.Errorf("text overflows: max line width %d > terminal width %d", maxLineWidth, termWidth)
	}

	// The long text should wrap across multiple lines
	xLines := 0
	for _, line := range lines {
		if strings.Contains(line, "xxx") {
			xLines++
		}
	}
	if xLines < 2 {
		t.Errorf("expected text to wrap across multiple lines, but only found %d lines with 'xxx'", xLines)
	}

	// All textarea lines should be consistently indented (PaddingLeft applied uniformly)
	taView := p.commandTextArea.View()
	taLines := strings.Split(taView, "\n")
	for _, line := range taLines {
		w := lipgloss.Width(line)
		if w > p.textareaWidth() {
			t.Errorf("textarea line wider than textareaWidth(): %d > %d", w, p.textareaWidth())
		}
	}
}

func TestCommandTextAreaWrapsNarrowTerminal(t *testing.T) {
	p := testPluginWithCC(t)
	p.width = 80

	// Enter detail view, then command input
	_ = p.HandleKey(keyMsg("enter"))
	_ = p.HandleKey(keyMsg("c"))

	// Type text that exceeds narrow terminal width
	longText := strings.Repeat("y", 100)
	for _, ch := range longText {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	view := p.View(80, 40, 0)
	lines := strings.Split(view, "\n")

	maxLineWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxLineWidth {
			maxLineWidth = w
		}
	}
	if maxLineWidth > 80 {
		t.Errorf("text overflows narrow terminal: max line width %d > 80", maxLineWidth)
	}

	// Text should wrap
	yLines := 0
	for _, line := range lines {
		if strings.Contains(line, "yyy") {
			yLines++
		}
	}
	if yLines < 2 {
		t.Errorf("expected text to wrap in narrow terminal, but only found %d lines with 'yyy'", yLines)
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

func TestTaskRunnerPathPickerNoSelection(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	// Enter task runner, then manually open path picker
	p.HandleKey(keyMsg("o"))
	p.taskRunnerPickingPath = true
	p.detailPaths = []string{"/tmp/a", "/tmp/b"}

	// Set cursor to -1 (no selection) and press enter — should NOT panic
	p.taskRunnerPathCursor = -1
	p.HandleKey(keyMsg("enter"))
	if p.taskRunnerPickingPath {
		t.Error("enter should close the path picker")
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

func TestTaskRunnerLaunchInteractive(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	// Enter task runner, advance to step 3
	p.HandleKey(keyMsg("o"))
	p.HandleKey(keyMsg("enter")) // step 1 -> 2
	p.HandleKey(keyMsg("enter")) // step 2 -> 3

	// Default launch cursor is 0 (Run Claude)
	if p.taskRunnerLaunchCursor != 0 {
		t.Fatalf("initial launch cursor = %d, want 0", p.taskRunnerLaunchCursor)
	}

	// Enter at cursor 0 should launch interactive session
	action := p.HandleKey(keyMsg("enter"))
	if p.taskRunnerView {
		t.Error("task runner should be closed after launch")
	}
	if action.Type != "launch" {
		t.Errorf("action type = %q, want 'launch'", action.Type)
	}
	if action.Args["dir"] != "/tmp/myproject" {
		t.Errorf("launch dir = %q, want '/tmp/myproject'", action.Args["dir"])
	}
	if action.Args["initial_prompt"] == "" {
		t.Error("interactive launch should include initial_prompt")
	}
}

func TestTaskRunnerLaunchQueue(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"

	// Enter task runner, advance to step 3
	p.HandleKey(keyMsg("o"))
	p.HandleKey(keyMsg("enter")) // step 1 -> 2
	p.HandleKey(keyMsg("enter")) // step 2 -> 3

	// Move launch cursor to 1 (Queue Agent)
	p.HandleKey(keyMsg("right"))
	if p.taskRunnerLaunchCursor != 1 {
		t.Fatalf("launch cursor = %d, want 1", p.taskRunnerLaunchCursor)
	}

	// Enter at cursor 1 should queue agent
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

	// Move launch cursor to 2 (Run Agent Now)
	p.HandleKey(keyMsg("right"))
	p.HandleKey(keyMsg("right"))
	if p.taskRunnerLaunchCursor != 2 {
		t.Fatalf("launch cursor = %d, want 2", p.taskRunnerLaunchCursor)
	}

	// Enter at cursor 2 should launch immediately
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

	// Press 'c' to enter instruction input mode
	p.HandleKey(keyMsg("c"))
	if !p.taskRunnerInputting {
		t.Error("'c' at step 3 should set taskRunnerInputting to true")
	}

	// Esc should cancel input mode
	p.HandleKey(specialKeyMsg(tea.KeyEscape))
	if p.taskRunnerInputting {
		t.Error("esc should cancel instruction input")
	}
}

func TestWizardSelectionsPersistedOnBackout(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"
	insertTestPaths(t, p.database, []string{"/tmp/a", "/tmp/b", "/tmp/myproject"})
	p.detailPaths = []string{"/tmp/a", "/tmp/b", "/tmp/myproject"}

	// Press 'o' to enter wizard
	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerView || p.taskRunnerStep != 1 {
		t.Fatalf("expected taskRunnerView=true step=1, got view=%v step=%d", p.taskRunnerView, p.taskRunnerStep)
	}

	// Step 1 -> Step 2
	p.HandleKey(keyMsg("enter"))
	if p.taskRunnerStep != 2 {
		t.Fatalf("step = %d, want 2", p.taskRunnerStep)
	}

	// Change mode to worktree
	p.HandleKey(keyMsg("right")) // normal -> worktree
	if p.taskRunnerMode != "worktree" {
		t.Fatalf("mode = %q, want worktree", p.taskRunnerMode)
	}

	// Step 2 -> Step 3
	p.HandleKey(keyMsg("enter"))
	if p.taskRunnerStep != 3 {
		t.Fatalf("step = %d, want 3", p.taskRunnerStep)
	}

	// Now back out: Step 3 -> Step 2
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.taskRunnerStep != 2 {
		t.Fatalf("step = %d, want 2", p.taskRunnerStep)
	}

	// Step 2 -> Step 1
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.taskRunnerStep != 1 {
		t.Fatalf("step = %d, want 1", p.taskRunnerStep)
	}

	// Step 1 -> exit task runner (should save selections)
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.taskRunnerView {
		t.Fatal("taskRunnerView should be false after esc at step 1")
	}

	// Exit detail view
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.detailView {
		t.Fatal("detailView should be false after esc")
	}

	// Re-open wizard on the same todo
	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerView {
		t.Fatal("taskRunnerView should be true after re-opening")
	}

	// The mode should be restored to worktree
	if p.taskRunnerMode != "worktree" {
		t.Errorf("after re-open: mode = %q, want worktree", p.taskRunnerMode)
	}

	// The path cursor should be restored
	expectedPathCursor := 2 // /tmp/myproject is at index 2
	if p.taskRunnerPathCursor != expectedPathCursor {
		t.Errorf("after re-open: pathCursor = %d, want %d", p.taskRunnerPathCursor, expectedPathCursor)
	}
}

func TestWizardSelectionsPersistedWithPathChange(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "" // no project dir
	insertTestPaths(t, p.database, []string{"/tmp/a", "/tmp/b", "/tmp/c"})
	p.detailPaths = []string{"/tmp/a", "/tmp/b", "/tmp/c"}

	// Press 'o' to enter wizard — path picker should auto-open
	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerView {
		t.Fatal("taskRunnerView should be true")
	}
	if !p.taskRunnerPickingPath {
		t.Fatal("path picker should auto-open for todo with no project dir")
	}

	// Navigate to /tmp/b (index 1) and select it
	// Cursor starts at -1 due to no project dir, j increments to 0, then to 1
	p.HandleKey(keyMsg("j")) // cursor -1 -> 0
	p.HandleKey(keyMsg("j")) // cursor 0 -> 1
	p.HandleKey(keyMsg("enter")) // select /tmp/b
	if p.taskRunnerPickingPath {
		t.Fatal("path picker should close after enter")
	}
	if p.taskRunnerPathCursor != 1 {
		t.Fatalf("pathCursor = %d, want 1", p.taskRunnerPathCursor)
	}

	// Step 1 -> Step 2
	p.HandleKey(keyMsg("enter"))
	// Change mode to sandbox
	p.HandleKey(keyMsg("right")) // normal -> worktree
	p.HandleKey(keyMsg("right")) // worktree -> sandbox

	// Back out: Step 2 -> Step 1 -> exit
	p.HandleKey(specialKeyMsg(tea.KeyEsc)) // step 2 -> 1
	p.HandleKey(specialKeyMsg(tea.KeyEsc)) // step 1 -> exit (saves)
	p.HandleKey(specialKeyMsg(tea.KeyEsc)) // exit detail view

	// Re-open wizard
	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerView {
		t.Fatal("taskRunnerView should be true")
	}

	// Path cursor should be restored to 1 (/tmp/b)
	if p.taskRunnerPathCursor != 1 {
		t.Errorf("after re-open: pathCursor = %d, want 1", p.taskRunnerPathCursor)
	}

	// Mode should be restored to sandbox
	if p.taskRunnerMode != "sandbox" {
		t.Errorf("after re-open: mode = %q, want sandbox", p.taskRunnerMode)
	}

	// Path picker should NOT auto-open since we have saved selections
	if p.taskRunnerPickingPath {
		t.Error("path picker should NOT auto-open when saved selections exist")
	}
}

func TestWizardSelectionsPersistedEscFromStep2(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"
	insertTestPaths(t, p.database, []string{"/tmp/a", "/tmp/b", "/tmp/myproject"})
	p.detailPaths = []string{"/tmp/a", "/tmp/b", "/tmp/myproject"}

	// Press 'o' to enter wizard
	p.HandleKey(keyMsg("o"))

	// Step 1 -> Step 2
	p.HandleKey(keyMsg("enter"))

	// Change mode to worktree on step 2
	p.HandleKey(keyMsg("right")) // normal -> worktree

	// Now escape from step 2 (goes to step 1, does NOT save yet)
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.taskRunnerStep != 1 {
		t.Fatalf("step = %d, want 1", p.taskRunnerStep)
	}

	// Escape from step 1 (saves and exits wizard)
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.taskRunnerView {
		t.Fatal("should have exited task runner")
	}

	// Verify the mode was saved
	saved, ok := p.wizardSelections[p.cc.Todos[0].ID]
	if !ok {
		t.Fatal("wizard selections should be saved")
	}
	if saved.mode != "worktree" {
		t.Errorf("saved mode = %q, want worktree", saved.mode)
	}

	// Exit detail view and re-open
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	p.HandleKey(keyMsg("o"))

	if p.taskRunnerMode != "worktree" {
		t.Errorf("after re-open: mode = %q, want worktree", p.taskRunnerMode)
	}
}

func TestWizardSelectionsPersistedFromDetailView(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/myproject"
	insertTestPaths(t, p.database, []string{"/tmp/a", "/tmp/b", "/tmp/myproject"})
	p.detailPaths = []string{"/tmp/a", "/tmp/b", "/tmp/myproject"}

	// Enter detail view first (not task runner)
	p.HandleKey(keyMsg("enter"))
	if !p.detailView {
		t.Fatal("should be in detail view")
	}
	if p.taskRunnerView {
		t.Fatal("should NOT be in task runner yet")
	}

	// Press 'o' from detail view to enter task runner
	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerView {
		t.Fatal("should be in task runner")
	}

	// Advance to step 2 and change mode
	p.HandleKey(keyMsg("enter"))
	p.HandleKey(keyMsg("right")) // normal -> worktree

	// Back out to list
	p.HandleKey(specialKeyMsg(tea.KeyEsc)) // step 2 -> 1
	p.HandleKey(specialKeyMsg(tea.KeyEsc)) // step 1 -> exit task runner (saves)
	if p.taskRunnerView {
		t.Fatal("should have exited task runner")
	}

	// Exit detail view
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.detailView {
		t.Fatal("should have exited detail view")
	}

	// Re-open via 'o' from list
	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerView {
		t.Fatal("should be in task runner again")
	}

	if p.taskRunnerMode != "worktree" {
		t.Errorf("mode = %q, want worktree", p.taskRunnerMode)
	}
}

func TestWizardPickingPathNotStaleOnReopen(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "" // no project dir triggers auto-open
	insertTestPaths(t, p.database, []string{"/tmp/a", "/tmp/b"})
	p.detailPaths = []string{"/tmp/a", "/tmp/b"}

	// Enter wizard — auto-opens path picker
	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerPickingPath {
		t.Fatal("path picker should auto-open")
	}

	// Select a path
	p.HandleKey(keyMsg("enter"))
	if p.taskRunnerPickingPath {
		t.Fatal("path picker should close after enter")
	}

	// Go to step 2 and change mode
	p.HandleKey(keyMsg("enter"))
	p.HandleKey(keyMsg("right")) // worktree

	// Back out all the way
	p.HandleKey(specialKeyMsg(tea.KeyEsc)) // step 2 -> 1
	p.HandleKey(specialKeyMsg(tea.KeyEsc)) // step 1 -> exit (saves)
	p.HandleKey(specialKeyMsg(tea.KeyEsc)) // exit detail

	// Re-enter — path picker should NOT auto-open
	p.HandleKey(keyMsg("o"))
	if p.taskRunnerPickingPath {
		t.Error("path picker should NOT auto-open when saved selections exist")
	}
	if p.taskRunnerMode != "worktree" {
		t.Errorf("mode = %q, want worktree", p.taskRunnerMode)
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

func TestYKeyInTriageFilterDoesNotPanic(t *testing.T) {
	p := testPluginWithCC(t)
	// Set up todos with different triage statuses
	p.cc.Todos = []db.Todo{
		{ID: "t-new-1", Title: "New todo 1", Status: db.StatusNew, Source: "manual", CreatedAt: time.Now(), ProjectDir: "/tmp/proj1"},
		{ID: "t-acc-1", Title: "Accepted todo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now(), ProjectDir: "/tmp/proj2"},
		{ID: "t-new-2", Title: "New todo 2", Status: db.StatusNew, Source: "manual", CreatedAt: time.Now(), ProjectDir: "/tmp/proj3"},
	}
	insertTestPaths(t, p.database, []string{"/tmp/proj1", "/tmp/proj2", "/tmp/proj3"})

	// Enter expanded view and set triage filter to "new"
	p.HandleKey(keyMsg(" ")) // toggle expanded
	if !p.ccExpanded {
		t.Fatal("expected expanded view")
	}
	// Set filter to "inbox" — only t-new-1 and t-new-2 should be visible
	p.triageFilter = "inbox"
	p.ccCursor = 0

	filtered := p.filteredTodos()
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered todos in 'new' filter, got %d", len(filtered))
	}
	if filtered[0].ID != "t-new-1" {
		t.Fatalf("expected first filtered todo to be t-new-1, got %s", filtered[0].ID)
	}

	// Press Y on cursor 0 — should NOT panic and should open task runner for t-new-1
	p.HandleKey(keyMsg("Y"))

	if !p.detailView {
		t.Error("Y should open detail view")
	}
	if !p.taskRunnerView {
		t.Error("Y should open task runner view")
	}
	if p.detailTodoID != "t-new-1" {
		t.Errorf("detailTodoID = %q, want %q", p.detailTodoID, "t-new-1")
	}
}

func TestExtractSessionSummary(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		wantSub  string // substring expected in summary
		wantNot  string // substring that should NOT appear
	}{
		{
			name:     "empty output success",
			output:   "",
			exitCode: 0,
			wantSub:  "Session completed successfully",
		},
		{
			name:     "empty output failure",
			output:   "",
			exitCode: 1,
			wantSub:  "exited with code 1",
		},
		{
			name: "assistant text content",
			output: `{"type":"system","session_id":"abc123"}
{"type":"assistant","content":[{"type":"text","text":"I fixed the bug in main.go by updating the error handler."}]}
`,
			exitCode: 0,
			wantSub:  "I fixed the bug in main.go",
			wantNot:  `"type"`,
		},
		{
			name: "multiple assistant messages uses last",
			output: `{"type":"assistant","content":[{"type":"text","text":"Looking at the code..."}]}
{"type":"assistant","content":[{"type":"text","text":"Done! I updated 3 files and added tests."}]}
`,
			exitCode: 0,
			wantSub:  "Done! I updated 3 files",
		},
		{
			name: "result event preferred over assistant",
			output: `{"type":"assistant","content":[{"type":"text","text":"Working on it..."}]}
{"type":"result","result":"Completed: fixed the login bug and added unit tests."}
`,
			exitCode: 0,
			wantSub:  "Completed: fixed the login bug",
		},
		{
			name: "ignores non-text content blocks",
			output: `{"type":"assistant","content":[{"type":"tool_use","name":"Read","input":{"path":"/foo"}},{"type":"text","text":"I read the file and found the issue."}]}
`,
			exitCode: 0,
			wantSub:  "I read the file and found the issue",
			wantNot:  "tool_use",
		},
		{
			name:     "malformed JSON lines skipped",
			output:   "not json\n{\"type\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Summary here.\"}]}\n",
			exitCode: 0,
			wantSub:  "Summary here.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := &agentSession{
				exitCode: tt.exitCode,
			}
			sess.Output.WriteString(tt.output)

			got := extractSessionSummary(sess)
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("extractSessionSummary() = %q, want substring %q", got, tt.wantSub)
			}
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("extractSessionSummary() = %q, should not contain %q", got, tt.wantNot)
			}
		})
	}
}

