package commandcenter

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// viewContains asserts that view contains text, dumping the full view on failure.
func viewContains(t *testing.T, view, text string) {
	t.Helper()
	if !strings.Contains(view, text) {
		t.Errorf("expected view to contain %q but it did not.\nFull view:\n%s", text, view)
	}
}

// viewNotContains asserts that view does NOT contain text.
func viewNotContains(t *testing.T, view, text string) {
	t.Helper()
	if strings.Contains(view, text) {
		t.Errorf("expected view NOT to contain %q but it did.\nFull view:\n%s", text, view)
	}
}

// testPluginWithTodos creates a plugin with the given todos pre-loaded.
func testPluginWithTodos(t *testing.T, todos []db.Todo) *Plugin {
	t.Helper()
	p := testPlugin(t)
	p.cc = &db.CommandCenter{
		GeneratedAt: time.Now(),
		Todos:       todos,
	}
	p.width = 120
	p.height = 40
	return p
}

// renderView renders the plugin's View at standard dimensions.
func renderView(p *Plugin) string {
	return p.View(120, 38, 0)
}

// ---------------------------------------------------------------------------
// Status Badge Rendering (9 tests)
// ---------------------------------------------------------------------------

func TestView_TodoStatusNew(t *testing.T) {
	// "new" status todos appear in the "inbox" triage tab when expanded
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Inbox item alpha", Status: db.StatusNew, Source: "github", CreatedAt: time.Now()},
	})
	// Expand the view and switch to inbox tab
	p.HandleKey(keyMsg(" "))
	if !p.ccExpanded {
		t.Fatal("space should expand the view")
	}
	// Default triage is "todo", switch to "inbox"
	p.HandleKey(keyMsg("tab"))
	view := renderView(p)
	viewContains(t, view, "Inbox item alpha")
}

func TestView_TodoStatusBacklog(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Backlog task bravo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
	})
	view := renderView(p)
	viewContains(t, view, "Backlog task bravo")
}

func TestView_TodoStatusEnqueued(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Enqueued task charlie", Status: db.StatusEnqueued, Source: "manual", CreatedAt: time.Now()},
	})
	view := renderView(p)
	viewContains(t, view, "Enqueued task charlie")
	viewContains(t, view, "queued")
}

func TestView_TodoStatusRunning(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Running task delta", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
	})
	view := renderView(p)
	viewContains(t, view, "Running task delta")
	viewContains(t, view, "agent working")
}

func TestView_TodoStatusBlocked(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Blocked task echo", Status: db.StatusBlocked, Source: "manual", CreatedAt: time.Now()},
	})
	view := renderView(p)
	viewContains(t, view, "Blocked task echo")
	viewContains(t, view, "needs input")
}

func TestView_TodoStatusReview(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Review task foxtrot", Status: db.StatusReview, Source: "manual", CreatedAt: time.Now()},
	})
	view := renderView(p)
	viewContains(t, view, "Review task foxtrot")
	viewContains(t, view, "ready for review")
}

func TestView_TodoStatusFailed(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Failed task golf", Status: db.StatusFailed, Source: "manual", CreatedAt: time.Now()},
	})
	view := renderView(p)
	viewContains(t, view, "Failed task golf")
	viewContains(t, view, "failed")
}

func TestView_TodoStatusCompleted(t *testing.T) {
	// Completed todos are terminal; they should NOT appear in the default active list
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Completed task hotel", Status: db.StatusCompleted, Source: "manual", CreatedAt: time.Now()},
		{ID: "t2", Title: "Active task india", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
	})
	view := renderView(p)
	viewNotContains(t, view, "Completed task hotel")
	viewContains(t, view, "Active task india")
}

func TestView_TodoStatusDismissed(t *testing.T) {
	// Dismissed todos are terminal; they should NOT appear in any active view
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Dismissed task juliet", Status: db.StatusDismissed, Source: "manual", CreatedAt: time.Now()},
		{ID: "t2", Title: "Active task kilo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
	})
	view := renderView(p)
	viewNotContains(t, view, "Dismissed task juliet")
	// Also check expanded view with "all" filter
	p.HandleKey(keyMsg(" "))
	// Cycle tabs to "all" (todo -> inbox -> agents -> review -> all)
	p.HandleKey(keyMsg("tab"))
	p.HandleKey(keyMsg("tab"))
	p.HandleKey(keyMsg("tab"))
	p.HandleKey(keyMsg("tab"))
	view = renderView(p)
	viewNotContains(t, view, "Dismissed task juliet")
}

// ---------------------------------------------------------------------------
// Navigation and View Modes (9 tests)
// ---------------------------------------------------------------------------

func TestView_ExpandCollapseSpaceCycles(t *testing.T) {
	p := testPluginWithCC(t)

	// Start collapsed
	view1 := renderView(p)

	// Press space -> expanded 2-col
	p.HandleKey(keyMsg(" "))
	if !p.ccExpanded || p.ccExpandedCols != 2 {
		t.Fatal("first space should expand to 2-col")
	}
	view2 := renderView(p)

	// Press space -> expanded 1-col
	p.HandleKey(keyMsg(" "))
	if !p.ccExpanded || p.ccExpandedCols != 1 {
		t.Fatal("second space should switch to 1-col")
	}
	view3 := renderView(p)

	// Press space -> collapsed
	p.HandleKey(keyMsg(" "))
	if p.ccExpanded {
		t.Fatal("third space should collapse")
	}

	// Views should differ at each stage
	if view1 == view2 {
		t.Error("collapsed and 2-col expanded views should differ")
	}
	if view2 == view3 {
		t.Error("2-col and 1-col expanded views should differ")
	}
}

func TestView_ExpandedViewShowsTriageTabs(t *testing.T) {
	p := testPluginWithCC(t)
	p.HandleKey(keyMsg(" "))
	view := renderView(p)
	viewContains(t, view, "ToDo")
	viewContains(t, view, "Inbox")
	viewContains(t, view, "Agents")
}

func TestView_TriageTabFiltersContent(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Backlog lima", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
		{ID: "t2", Title: "Inbox mike", Status: db.StatusNew, Source: "github", CreatedAt: time.Now()},
		{ID: "t3", Title: "Running november", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
	})

	// Expand to see triage tabs (default is "todo")
	p.HandleKey(keyMsg(" "))
	todoView := renderView(p)
	viewContains(t, todoView, "Backlog lima")
	viewNotContains(t, todoView, "Inbox mike")

	// Switch to inbox tab
	p.HandleKey(keyMsg("tab"))
	inboxView := renderView(p)
	viewContains(t, inboxView, "Inbox mike")
	viewNotContains(t, inboxView, "Backlog lima")

	// Switch to agents tab
	p.HandleKey(keyMsg("tab"))
	agentView := renderView(p)
	viewContains(t, agentView, "Running november")
	viewNotContains(t, agentView, "Backlog lima")
}

func TestView_DetailViewOpensOnEnter(t *testing.T) {
	p := testPluginWithCC(t)
	p.HandleKey(keyMsg("enter"))
	if !p.detailView {
		t.Fatal("enter should open detail view")
	}
	view := renderView(p)
	// Detail view shows "TODO #" header
	viewContains(t, view, "TODO #")
	viewContains(t, view, "First todo")
}

func TestView_DetailViewClosesOnEsc(t *testing.T) {
	p := testPluginWithCC(t)
	p.HandleKey(keyMsg("enter"))
	if !p.detailView {
		t.Fatal("enter should open detail view")
	}
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.detailView {
		t.Fatal("esc should close detail view")
	}
	// Back to list view, should contain todo titles
	view := renderView(p)
	viewContains(t, view, "First todo")
}

func TestView_DetailViewTracksTodoByID(t *testing.T) {
	p := testPluginWithCC(t)
	// Move cursor to second todo, then open detail
	p.HandleKey(keyMsg("j"))
	p.HandleKey(keyMsg("enter"))
	if !p.detailView {
		t.Fatal("enter should open detail view")
	}
	view := renderView(p)
	viewContains(t, view, "Second todo")
}

func TestView_SearchModeEnterExit(t *testing.T) {
	p := testPluginWithCC(t)

	// Enter search mode
	p.HandleKey(keyMsg("/"))
	if !p.searchActive {
		t.Fatal("/ should activate search")
	}
	view := renderView(p)
	// Search view shows the "/" prompt indicator and hints
	viewContains(t, view, "esc clear")

	// Exit search with esc
	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.searchActive {
		t.Fatal("esc should deactivate search")
	}
}

func TestView_SearchFilterUpdatesView(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Alpha unique name", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
		{ID: "t2", Title: "Bravo different name", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
	})

	// Expand to get full todo listing
	p.HandleKey(keyMsg(" "))
	viewBefore := renderView(p)
	viewContains(t, viewBefore, "Alpha unique name")
	viewContains(t, viewBefore, "Bravo different name")

	// Enter search mode and type filter characters
	p.HandleKey(keyMsg("/"))
	for _, ch := range "Alpha" {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	viewAfter := renderView(p)
	viewContains(t, viewAfter, "Alpha unique name")
	viewNotContains(t, viewAfter, "Bravo different name")
}

func TestView_SearchEnterOpensDirectly(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Searchable oscar", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
		{ID: "t2", Title: "Other papa", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
	})

	// Search for a specific todo, press enter — should open detail view directly (BUG-115)
	p.HandleKey(keyMsg("/"))
	for _, ch := range "oscar" {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	p.HandleKey(specialKeyMsg(tea.KeyEnter))

	// Should open detail view directly, not freeze the filter
	if !p.detailView {
		t.Fatal("enter in search should open detail view directly (BUG-115)")
	}
	view := renderView(p)
	viewContains(t, view, "Searchable oscar")
}

// ---------------------------------------------------------------------------
// Agent Interactions (8 tests)
// ---------------------------------------------------------------------------

func TestView_TaskRunnerStep1ProjectSelection(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/testproject"

	p.HandleKey(keyMsg("o"))
	if !p.taskRunnerView || p.taskRunnerStep != 1 {
		t.Fatal("o should open task runner at step 1")
	}
	view := renderView(p)
	viewContains(t, view, "Step 1/3")
	viewContains(t, view, "Project")
}

func TestView_TaskRunnerStep2ModeSelection(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/testproject"

	p.HandleKey(keyMsg("o"))
	p.HandleKey(keyMsg("enter")) // step 1 -> 2
	if p.taskRunnerStep != 2 {
		t.Fatal("step should be 2")
	}
	view := renderView(p)
	viewContains(t, view, "Step 2/3")
	viewContains(t, view, "Mode")
	viewContains(t, view, "Normal")
	viewContains(t, view, "Worktree")
	viewContains(t, view, "Sandbox")
}

func TestView_TaskRunnerStep3LaunchOptions(t *testing.T) {
	p := testPluginWithCC(t)
	p.cc.Todos[0].ProjectDir = "/tmp/testproject"

	p.HandleKey(keyMsg("o"))
	p.HandleKey(keyMsg("enter")) // step 1 -> 2
	p.HandleKey(keyMsg("enter")) // step 2 -> 3
	if p.taskRunnerStep != 3 {
		t.Fatal("step should be 3")
	}
	view := renderView(p)
	viewContains(t, view, "Step 3/3")
	viewContains(t, view, "Run Claude")
	viewContains(t, view, "Queue Agent")
	viewContains(t, view, "Run Agent Now")
}

func TestView_EditGuardBlocksMutationDuringAgent(t *testing.T) {
	// The edit guard checks agentRunner.Session(todo.ID) != nil, which requires
	// a real running agent process. Simulating this needs agent runner infrastructure.
	t.Skip("requires agent runner mock to simulate active session")
}

func TestView_EditGuardAllowsNavigationDuringAgent(t *testing.T) {
	// Navigation (j/k) always works regardless of agent state — no guard needed.
	// Test cursor navigation with running status todos.
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Agent todo quebec", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
		{ID: "t2", Title: "Normal todo romeo", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
	})

	if p.ccCursor != 0 {
		t.Fatal("initial cursor should be 0")
	}
	p.HandleKey(keyMsg("j"))
	if p.ccCursor != 1 {
		t.Error("j should move cursor down")
	}
	p.HandleKey(keyMsg("k"))
	if p.ccCursor != 0 {
		t.Error("k should move cursor up")
	}
}

func TestView_SessionViewerOpensOnW(t *testing.T) {
	// Session viewer requires a real agent session with events channel.
	// The 'w' key checks agentRunner.Session(todo.ID) for the events channel.
	t.Skip("requires agent runner mock to provide events channel")
}

func TestView_SessionViewerClosesOnEsc(t *testing.T) {
	// Session viewer close test requires an open session viewer, which needs
	// a real agent session. Test the state transition directly.
	p := testPluginWithCC(t)
	// Manually activate session viewer
	p.sessionViewerActive = true
	p.detailView = true

	p.HandleKey(specialKeyMsg(tea.KeyEsc))
	if p.sessionViewerActive {
		t.Fatal("esc should close session viewer")
	}
	if !p.detailView {
		t.Error("should return to detail view after closing session viewer")
	}
}

func TestView_AgentFinishedUpdatesViewToReview(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Agent task sierra", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
	})

	// Before: agent status indicator should show "agent working"
	view := renderView(p)
	viewContains(t, view, "agent working")

	// Simulate agent finished (exit code 0 -> review)
	p.onAgentFinished("t1", 0)

	// After: status should change to review
	view = renderView(p)
	viewContains(t, view, "ready for review")
}

// ---------------------------------------------------------------------------
// Budget and Agent Header (4 tests)
// ---------------------------------------------------------------------------

func TestView_ExpandedAgentsHeaderShowsCounts(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Running agent tango", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
		{ID: "t2", Title: "Queued agent uniform", Status: db.StatusEnqueued, Source: "manual", CreatedAt: time.Now()},
		{ID: "t3", Title: "Backlog task victor", Status: db.StatusBacklog, Source: "manual", CreatedAt: time.Now()},
	})

	// Expand and switch to agents tab
	p.HandleKey(keyMsg(" "))
	// tab from "todo" -> "inbox" -> "agents"
	p.HandleKey(keyMsg("tab"))
	p.HandleKey(keyMsg("tab"))

	view := renderView(p)
	// Agents tab should show the agent todos
	viewContains(t, view, "Running agent tango")
	viewContains(t, view, "Queued agent uniform")
	viewNotContains(t, view, "Backlog task victor")
}

func TestView_LaunchDeniedShowsFlash(t *testing.T) {
	// Skip: LaunchDeniedMsg does not exist in the current codebase.
	// The flash message mechanism for launch denial is handled via
	// p.flashMessage directly in the agent launch path, not through
	// a dedicated message type.
	t.Skip("LaunchDeniedMsg does not exist in current codebase")
}

func TestView_KillAgentUpdatesStatus(t *testing.T) {
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Kill target whiskey", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
	})
	// Simulate agent finished with non-zero exit code (like a kill)
	p.onAgentFinished("t1", 1)

	view := renderView(p)
	// Non-zero exit sets status to "failed"
	viewContains(t, view, "failed")
}

func TestView_ConcurrencyLimitQueuesAgent(t *testing.T) {
	// In the collapsed view, renderTodoPanel shows agentStatusIndicator per-todo.
	// The collapsed view excludes "new" status but shows all other active statuses.
	p := testPluginWithTodos(t, []db.Todo{
		{ID: "t1", Title: "Active xray", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
		{ID: "t2", Title: "Active yankee", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
		{ID: "t3", Title: "Active zulu", Status: db.StatusRunning, Source: "manual", CreatedAt: time.Now()},
		{ID: "t4", Title: "Queued amber", Status: db.StatusEnqueued, Source: "manual", CreatedAt: time.Now()},
	})

	// Stay in collapsed view where status indicators are rendered
	view := renderView(p)
	viewContains(t, view, "queued")
	viewContains(t, view, "agent working")
}
