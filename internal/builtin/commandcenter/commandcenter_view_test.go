package commandcenter

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

// testPluginWithAgent creates a plugin with todos and an active agent session
// for the first todo. Returns the plugin and the session's EventsCh for pushing events.
func testPluginWithAgent(t *testing.T) (*Plugin, chan sessionEvent) {
	t.Helper()
	p := testPluginWithCC(t)
	todoID := p.cc.Todos[0].ID

	eventsCh := make(chan sessionEvent, 64)
	sess := &agentSession{
		TodoID:    todoID,
		Status:    "processing",
		StartedAt: time.Now(),
		EventsCh:  eventsCh,
		done:      make(chan struct{}),
	}
	p.activeSessions[todoID] = sess
	p.cc.Todos[0].Status = db.StatusRunning

	return p, eventsCh
}

// renderView renders the plugin at standard test dimensions.
func renderView(p *Plugin) string {
	return p.View(120, 40, 0)
}

// viewContains checks that the rendered view contains the given substring.
func viewContains(t *testing.T, p *Plugin, substr string) {
	t.Helper()
	v := renderView(p)
	if !strings.Contains(v, substr) {
		t.Errorf("view should contain %q but does not.\nView:\n%s", substr, v)
	}
}

// viewNotContains checks that the rendered view does NOT contain the given substring.
func viewNotContains(t *testing.T, p *Plugin, substr string) {
	t.Helper()
	v := renderView(p)
	if strings.Contains(v, substr) {
		t.Errorf("view should NOT contain %q but does.\nView:\n%s", substr, v)
	}
}

// openDetailView navigates into the detail view for the todo at the current cursor.
func openDetailView(t *testing.T, p *Plugin) {
	t.Helper()
	p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if !p.detailView {
		t.Fatal("expected detailView to be true after enter")
	}
}

func TestView_EditGuardBlocksMutationDuringAgent(t *testing.T) {
	p, _ := testPluginWithAgent(t)

	// Open detail view for the first todo (which has an active session)
	openDetailView(t, p)

	// Verify we're in viewing mode
	if p.detailMode != "viewing" {
		t.Fatalf("detailMode = %q, want viewing", p.detailMode)
	}

	// Press enter — would normally open field edit, but should be blocked
	p.HandleKey(specialKeyMsg(tea.KeyEnter))

	// Should show flash message about agent
	if !strings.Contains(p.flashMessage, "being updated by agent") {
		t.Errorf("flashMessage = %q, want something containing 'being updated by agent'", p.flashMessage)
	}

	// Should still be in viewing mode (not editing)
	if p.detailMode != "viewing" {
		t.Errorf("detailMode = %q after blocked enter, want 'viewing'", p.detailMode)
	}

	// View should render without panic and still show detail
	v := renderView(p)
	if v == "" {
		t.Error("view should not be empty")
	}
}

func TestView_EditGuardBlocksCommandInputDuringAgent(t *testing.T) {
	p, _ := testPluginWithAgent(t)
	openDetailView(t, p)

	// Press 'c' — would normally open command input, but should be blocked
	p.HandleKey(keyMsg("c"))

	if !strings.Contains(p.flashMessage, "being updated by agent") {
		t.Errorf("flashMessage = %q, want something containing 'being updated by agent'", p.flashMessage)
	}

	if p.detailMode != "viewing" {
		t.Errorf("detailMode = %q after blocked c, want 'viewing'", p.detailMode)
	}
}

func TestView_SessionViewerOpensOnW(t *testing.T) {
	p, _ := testPluginWithAgent(t)
	openDetailView(t, p)

	// Press 'w' to open session viewer
	p.HandleKey(keyMsg("w"))

	if !p.sessionViewerActive {
		t.Error("sessionViewerActive should be true after pressing w")
	}
	if p.sessionViewerTodoID != p.cc.Todos[0].ID {
		t.Errorf("sessionViewerTodoID = %q, want %q", p.sessionViewerTodoID, p.cc.Todos[0].ID)
	}

	// Render and check for SESSION VIEWER header
	viewContains(t, p, "SESSION VIEWER")
}

func TestView_SessionViewerShowsEvents(t *testing.T) {
	p, eventsCh := testPluginWithAgent(t)
	openDetailView(t, p)

	// Open session viewer
	p.HandleKey(keyMsg("w"))
	if !p.sessionViewerActive {
		t.Fatal("session viewer should be active")
	}

	// Push an event to the session's events list directly (simulating what
	// the background goroutine + handleAgentEvent would do)
	sess := p.activeSessions[p.cc.Todos[0].ID]
	testEvent := sessionEvent{
		Type:      "assistant_text",
		Text:      "Working on your task...",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	sess.Events = append(sess.Events, testEvent)
	p.updateSessionViewerContent()

	// Render — should not panic
	v := renderView(p)
	if v == "" {
		t.Error("session viewer should render non-empty")
	}

	// The view should contain SESSION VIEWER
	if !strings.Contains(v, "SESSION VIEWER") {
		t.Error("view should contain SESSION VIEWER header")
	}

	// Clean up channel to avoid goroutine leak
	close(eventsCh)
}

func TestView_EditGuardAllowsWatchDuringAgent(t *testing.T) {
	p, _ := testPluginWithAgent(t)
	openDetailView(t, p)

	// Verify edit is blocked (sanity check)
	p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if !strings.Contains(p.flashMessage, "being updated by agent") {
		t.Fatalf("edit should be blocked, but flashMessage = %q", p.flashMessage)
	}
	p.flashMessage = "" // clear

	// But 'w' should still work — not blocked by edit guard
	p.HandleKey(keyMsg("w"))

	if !p.sessionViewerActive {
		t.Error("sessionViewerActive should be true — w should not be blocked by edit guard")
	}
}

func TestView_AgentKillViaDetailView(t *testing.T) {
	p, _ := testPluginWithAgent(t)
	todoID := p.cc.Todos[0].ID
	openDetailView(t, p)

	// Verify session exists
	if _, ok := p.activeSessions[todoID]; !ok {
		t.Fatal("expected active session for todo")
	}

	// Press delete/backspace to kill agent
	p.HandleKey(specialKeyMsg(tea.KeyBackspace))

	// Session should be removed
	if _, ok := p.activeSessions[todoID]; ok {
		t.Error("active session should be removed after kill")
	}

	// Flash message should confirm kill
	if !strings.Contains(p.flashMessage, "Agent killed") {
		t.Errorf("flashMessage = %q, want 'Agent killed'", p.flashMessage)
	}

	// Todo status should revert to backlog
	for _, todo := range p.cc.Todos {
		if todo.ID == todoID {
			if todo.Status != db.StatusBacklog {
				t.Errorf("todo status = %q after kill, want %q", todo.Status, db.StatusBacklog)
			}
			break
		}
	}

	// After kill, edit should no longer be blocked
	p.flashMessage = "" // clear
	p.HandleKey(specialKeyMsg(tea.KeyEnter))
	if strings.Contains(p.flashMessage, "being updated by agent") {
		t.Error("edit should no longer be blocked after agent kill")
	}
}

func TestView_AgentFinishedUpdatesStatus(t *testing.T) {
	p, _ := testPluginWithAgent(t)
	todoID := p.cc.Todos[0].ID

	// Simulate agent finishing via the message handler
	p.HandleMessage(agentFinishedMsg{todoID: todoID, exitCode: 0})

	// Session should be cleaned up
	if _, ok := p.activeSessions[todoID]; ok {
		t.Error("active session should be removed after finish")
	}

	// Todo should move to review status
	for _, todo := range p.cc.Todos {
		if todo.ID == todoID {
			if todo.Status != db.StatusReview {
				t.Errorf("todo status = %q after success finish, want %q", todo.Status, db.StatusReview)
			}
			break
		}
	}
}

func TestView_AgentFinishedWithFailureStatus(t *testing.T) {
	p, _ := testPluginWithAgent(t)
	todoID := p.cc.Todos[0].ID

	// Simulate agent finishing with error
	p.HandleMessage(agentFinishedMsg{todoID: todoID, exitCode: 1})

	// Todo should move to failed status
	for _, todo := range p.cc.Todos {
		if todo.ID == todoID {
			if todo.Status != db.StatusFailed {
				t.Errorf("todo status = %q after failed finish, want %q", todo.Status, db.StatusFailed)
			}
			break
		}
	}
}

func TestView_SessionViewerClosesOnEsc(t *testing.T) {
	p, _ := testPluginWithAgent(t)
	openDetailView(t, p)

	// Open session viewer
	p.HandleKey(keyMsg("w"))
	if !p.sessionViewerActive {
		t.Fatal("session viewer should be active")
	}

	// Press esc to close
	p.HandleKey(keyMsg("esc"))
	if p.sessionViewerActive {
		t.Error("session viewer should be closed after esc")
	}
}

func TestView_NoSessionShowsFlash(t *testing.T) {
	p := testPluginWithCC(t)
	openDetailView(t, p)

	// Press 'w' without an active session — should show flash
	p.HandleKey(keyMsg("w"))

	if p.sessionViewerActive {
		t.Error("session viewer should not open without active session")
	}
	if !strings.Contains(p.flashMessage, "No active session") {
		t.Errorf("flashMessage = %q, want something containing 'No active session'", p.flashMessage)
	}
}

func TestView_KillWithNoAgentShowsFlash(t *testing.T) {
	p := testPluginWithCC(t)
	openDetailView(t, p)

	// Press backspace without agent
	p.HandleKey(specialKeyMsg(tea.KeyBackspace))

	if !strings.Contains(p.flashMessage, "No running agent") {
		t.Errorf("flashMessage = %q, want something containing 'No running agent'", p.flashMessage)
	}
}

func TestView_AgentBlockedStatusPropagates(t *testing.T) {
	p, _ := testPluginWithAgent(t)
	todoID := p.cc.Todos[0].ID

	// Simulate agent blocked status via message handler
	p.HandleMessage(agentStatusMsg{todoID: todoID, status: "blocked", question: "Need approval"})

	// Check status propagated
	for _, todo := range p.cc.Todos {
		if todo.ID == todoID {
			if todo.Status != "blocked" {
				t.Errorf("todo status = %q after block, want 'blocked'", todo.Status)
			}
			break
		}
	}
}
