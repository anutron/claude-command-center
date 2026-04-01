package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func makeTestEntry(id, status, origin string, costUSD float64) db.AgentHistoryEntry {
	return db.AgentHistoryEntry{
		AgentID:      id,
		Status:       status,
		OriginLabel:  origin,
		OriginRef:    "todo:1",
		OriginType:   "todo",
		Automation:   "manual",
		StartedAt:    time.Now().Add(-30 * time.Second),
		DurationSec:  30,
		CostUSD:      costUSD,
		InputTokens:  1000,
		OutputTokens: 500,
	}
}

func TestConsoleOverlay_ListView(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		makeTestEntry("abc123", "completed", "TODO #42 — Fix the auth bug", 0.0123),
		makeTestEntry("def456", "running", "TODO #43 — Add unit tests", 0.0045),
		makeTestEntry("ghi789", "failed", "PR #7 — Review changes", 0.0067),
	}

	o := consoleOverlay{}
	o.toggle(entries)

	got := o.renderList(120, 40)

	if !strings.Contains(got, overlayTitle) {
		t.Errorf("expected title %q in output", overlayTitle)
	}
	if !strings.Contains(got, overlaySubtitle) {
		t.Errorf("expected subtitle %q in output", overlaySubtitle)
	}
	if !strings.Contains(got, "TODO #42") {
		t.Errorf("expected origin label 'TODO #42' in list view")
	}
	if !strings.Contains(got, "TODO #43") {
		t.Errorf("expected origin label 'TODO #43' in list view")
	}
	if !strings.Contains(got, "PR #7") {
		t.Errorf("expected origin label 'PR #7' in list view")
	}
	// Status icons
	if !strings.Contains(got, "✓") {
		t.Errorf("expected completed icon ✓ in list view")
	}
	if !strings.Contains(got, "●") {
		t.Errorf("expected running icon ● in list view")
	}
	if !strings.Contains(got, "✗") {
		t.Errorf("expected failed icon ✗ in list view")
	}
	// Cost formatting
	if !strings.Contains(got, "$0.0123") {
		t.Errorf("expected cost $0.0123 in list view")
	}
}

func TestConsoleOverlay_EmptyState(t *testing.T) {
	o := consoleOverlay{}
	o.toggle(nil)

	got := o.renderList(120, 40)

	if !strings.Contains(got, overlayTitle) {
		t.Errorf("expected title %q in empty state", overlayTitle)
	}
	if !strings.Contains(got, overlayEmpty) {
		t.Errorf("expected empty state message %q", overlayEmpty)
	}
}

func TestConsoleOverlay_DetailView(t *testing.T) {
	finishedAt := time.Now()
	exitCode := 0
	entry := db.AgentHistoryEntry{
		AgentID:      "abc123fullid",
		Status:       "completed",
		OriginLabel:  "TODO #99 — Implement feature",
		OriginRef:    "todo:99",
		OriginType:   "todo",
		Automation:   "scheduled",
		StartedAt:    time.Now().Add(-2 * time.Minute),
		FinishedAt:   &finishedAt,
		DurationSec:  120,
		CostUSD:      0.025,
		InputTokens:  5000,
		OutputTokens: 2500,
		ExitCode:     &exitCode,
		ProjectDir:   "/Users/aaron/project",
		Repo:         "owner/repo",
		Branch:       "main",
		SessionID:    "session-uuid-001",
	}

	o := consoleOverlay{}
	o.toggle([]db.AgentHistoryEntry{entry})
	o.detail = true

	got := o.renderDetail(120, 40)

	if !strings.Contains(got, overlayTitle) {
		t.Errorf("expected title %q in detail view", overlayTitle)
	}
	if !strings.Contains(got, overlaySubDetail) {
		t.Errorf("expected detail subtitle %q", overlaySubDetail)
	}
	if !strings.Contains(got, "abc123fullid") {
		t.Errorf("expected agent ID in detail view")
	}
	if !strings.Contains(got, "session-uuid-001") {
		t.Errorf("expected session ID in detail view")
	}
	if !strings.Contains(got, "TODO #99") {
		t.Errorf("expected origin label in detail view")
	}
	if !strings.Contains(got, "todo:99") {
		t.Errorf("expected origin ref in detail view")
	}
	if !strings.Contains(got, "scheduled") {
		t.Errorf("expected automation field in detail view")
	}
	if !strings.Contains(got, "owner/repo") {
		t.Errorf("expected repo in detail view")
	}
	if !strings.Contains(got, "main") {
		t.Errorf("expected branch in detail view")
	}
	if !strings.Contains(got, "/Users/aaron/project") {
		t.Errorf("expected project dir in detail view")
	}
	if !strings.Contains(got, "5000") {
		t.Errorf("expected input tokens in detail view")
	}
	if !strings.Contains(got, "2500") {
		t.Errorf("expected output tokens in detail view")
	}
}

func TestConsoleOverlay_Toggle(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		makeTestEntry("e1", "completed", "TODO #1 — task one", 0.001),
	}

	o := consoleOverlay{}
	if o.visible {
		t.Fatal("overlay should start hidden")
	}

	o.toggle(entries)
	if !o.visible {
		t.Error("overlay should be visible after first toggle")
	}
	if o.cursor != 0 {
		t.Error("cursor should reset to 0 on toggle")
	}
	if o.detail {
		t.Error("detail should be false on toggle")
	}

	o.toggle(nil)
	if o.visible {
		t.Error("overlay should be hidden after second toggle")
	}
}

func TestConsoleOverlay_Navigation(t *testing.T) {
	entries := []db.AgentHistoryEntry{
		makeTestEntry("e1", "completed", "TODO #1 — first", 0.001),
		makeTestEntry("e2", "running", "TODO #2 — second", 0.002),
		makeTestEntry("e3", "failed", "TODO #3 — third", 0.003),
	}

	o := consoleOverlay{}
	o.toggle(entries)

	// Move down
	if o.cursor < len(o.entries)-1 {
		o.cursor++
	}
	if o.cursor != 1 {
		t.Errorf("cursor should be 1 after moving down, got %d", o.cursor)
	}

	// Move up
	if o.cursor > 0 {
		o.cursor--
	}
	if o.cursor != 0 {
		t.Errorf("cursor should be 0 after moving up, got %d", o.cursor)
	}

	// Can't go above 0
	if o.cursor > 0 {
		o.cursor--
	}
	if o.cursor != 0 {
		t.Errorf("cursor should stay at 0, got %d", o.cursor)
	}

	// Selected returns correct entry
	sel := o.selected()
	if sel == nil {
		t.Fatal("selected() should not be nil")
	}
	if sel.AgentID != "e1" {
		t.Errorf("selected() should return e1, got %s", sel.AgentID)
	}
}
