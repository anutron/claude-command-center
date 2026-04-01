package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/db"
)

// newTestConsoleModel returns a consoleModel with no daemon client (nil),
// seeded with the given entries and events for view-only tests.
func newTestConsoleModel(entries []db.AgentHistoryEntry, events []agent.SessionEvent, done bool) consoleModel {
	return consoleModel{
		client:  nil,
		entries: entries,
		cursor:  0,
		width:   100,
		height:  30,
		events:  events,
		done:    done,
	}
}

func TestConsoleView_EmptyState(t *testing.T) {
	m := newTestConsoleModel(nil, nil, false)
	v := m.View()
	if !strings.Contains(v, "No agents running") {
		t.Errorf("expected empty state message, got:\n%s", v)
	}
	if !strings.Contains(v, "Watching for activity") {
		t.Errorf("expected watching message, got:\n%s", v)
	}
}

func TestConsoleView_SidebarShowsAgents(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		{
			AgentID:     "agent-1",
			Status:      "running",
			OriginLabel: "TODO #42 — Fix the bug",
			StartedAt:   time.Now().Add(-30 * time.Second),
		},
		{
			AgentID:     "agent-2",
			Status:      "completed",
			OriginLabel: "PR #7 — Review changes",
			DurationSec: 120,
		},
	}
	m := newTestConsoleModel(entries, nil, false)
	v := m.View()

	if !strings.Contains(v, "AGENTS") {
		t.Errorf("expected AGENTS title in sidebar, got:\n%s", v)
	}
	if !strings.Contains(v, "TODO #42") {
		t.Errorf("expected TODO #42 in sidebar, got:\n%s", v)
	}
	if !strings.Contains(v, "PR #7") {
		t.Errorf("expected PR #7 in sidebar, got:\n%s", v)
	}
	if !strings.Contains(v, "── completed ──") {
		t.Errorf("expected completed separator in sidebar, got:\n%s", v)
	}
}

func TestConsoleView_FocusPaneHeader(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		{
			AgentID:     "agent-1",
			Status:      "running",
			OriginLabel: "TODO #42 — Fix the bug",
			StartedAt:   time.Now().Add(-30 * time.Second),
			CostUSD:     0.0123,
		},
	}
	m := newTestConsoleModel(entries, nil, false)
	v := m.View()

	if !strings.Contains(v, "TODO #42") {
		t.Errorf("expected origin label in focus pane, got:\n%s", v)
	}
	if !strings.Contains(v, "$") {
		t.Errorf("expected cost in focus pane, got:\n%s", v)
	}
}

func TestConsoleView_FocusPaneEvents(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		{
			AgentID:     "agent-1",
			Status:      "running",
			OriginLabel: "TODO #42 — Fix the bug",
			StartedAt:   time.Now(),
		},
	}
	events := []agent.SessionEvent{
		{Type: "tool_use", ToolName: "Read", ToolInput: "internal/foo.go"},
		{Type: "tool_result", ResultText: "package foo\n", IsError: false},
		{Type: "assistant_text", Text: "I see the issue."},
		{Type: "error", Text: "something went wrong", IsError: true},
	}
	m := newTestConsoleModel(entries, events, false)
	v := m.View()

	if !strings.Contains(v, "Read") {
		t.Errorf("expected tool name 'Read' in events, got:\n%s", v)
	}
	if !strings.Contains(v, "internal/foo.go") {
		t.Errorf("expected tool input in events, got:\n%s", v)
	}
	if !strings.Contains(v, "package foo") {
		t.Errorf("expected tool result text in events, got:\n%s", v)
	}
	if !strings.Contains(v, "I see the issue.") {
		t.Errorf("expected assistant text in events, got:\n%s", v)
	}
	if !strings.Contains(v, "ERROR:") {
		t.Errorf("expected ERROR label in events, got:\n%s", v)
	}
}

func TestConsoleView_WaitingForEvents(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		{
			AgentID:     "agent-1",
			Status:      "running",
			OriginLabel: "TODO #42 — Fix the bug",
			StartedAt:   time.Now(),
		},
	}
	m := newTestConsoleModel(entries, nil, false)
	v := m.View()
	if !strings.Contains(v, "Waiting for events") {
		t.Errorf("expected 'Waiting for events' message, got:\n%s", v)
	}
}

func TestConsoleView_DoneNoEvents(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		{
			AgentID:     "agent-1",
			Status:      "completed",
			OriginLabel: "TODO #42 — Fix the bug",
			DurationSec: 60,
		},
	}
	m := newTestConsoleModel(entries, nil, true)
	v := m.View()
	if !strings.Contains(v, "no event data available") {
		t.Errorf("expected 'no event data available' message, got:\n%s", v)
	}
}

func TestConsoleUpdate_QuitKey(t *testing.T) {
	m := newTestConsoleModel(nil, nil, false)
	m.width = 100
	m.height = 30

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit command, got nil")
	}
	// Verify it produces a Quit msg by running the command.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestConsoleUpdate_CursorNavigation(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		{AgentID: "a1", Status: "running", OriginLabel: "TODO #1"},
		{AgentID: "a2", Status: "running", OriginLabel: "TODO #2"},
		{AgentID: "a3", Status: "completed", OriginLabel: "TODO #3"},
	}
	m := newTestConsoleModel(entries, nil, false)
	m.width = 100
	m.height = 30

	// Move down.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	um := updated.(consoleModel)
	if um.cursor != 1 {
		t.Errorf("expected cursor=1 after j, got %d", um.cursor)
	}

	// Move down again.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	um = updated.(consoleModel)
	if um.cursor != 2 {
		t.Errorf("expected cursor=2 after second j, got %d", um.cursor)
	}

	// Move up.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	um = updated.(consoleModel)
	if um.cursor != 1 {
		t.Errorf("expected cursor=1 after k, got %d", um.cursor)
	}
}

func TestConsoleUpdate_CursorBoundary(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		{AgentID: "a1", Status: "running", OriginLabel: "TODO #1"},
	}
	m := newTestConsoleModel(entries, nil, false)
	m.width = 100
	m.height = 30

	// Already at cursor 0, moving up should stay at 0.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	um := updated.(consoleModel)
	if um.cursor != 0 {
		t.Errorf("expected cursor=0 at boundary, got %d", um.cursor)
	}

	// Only one entry, moving down should stay at 0.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	um = updated.(consoleModel)
	if um.cursor != 0 {
		t.Errorf("expected cursor=0 at lower boundary, got %d", um.cursor)
	}
}

func TestConsoleUpdate_WindowSize(t *testing.T) {
	m := newTestConsoleModel(nil, nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	um := updated.(consoleModel)
	if um.width != 120 || um.height != 40 {
		t.Errorf("expected width=120 height=40, got %d %d", um.width, um.height)
	}
}

func TestConsoleView_NarrowTerminal(t *testing.T) {
	// Narrow terminal should use smaller sidebar width without crashing.
	entries := []db.AgentHistoryEntry{
		{AgentID: "a1", Status: "running", OriginLabel: "TODO #1"},
	}
	m := newTestConsoleModel(entries, nil, false)
	m.width = 40
	m.height = 20
	v := m.View()
	if v == "" {
		t.Error("expected non-empty view for narrow terminal")
	}
}

func TestConsoleSidebar_OnlyActiveAgents(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		{AgentID: "a1", Status: "running", OriginLabel: "TODO #1 — active"},
		{AgentID: "a2", Status: "queued", OriginLabel: "TODO #2 — queued"},
	}
	m := newTestConsoleModel(entries, nil, false)
	v := m.View()

	// Should not have the completed separator when there are no completed agents.
	if strings.Contains(v, "── completed ──") {
		t.Errorf("unexpected 'completed' separator when no completed agents")
	}
}
