package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
)

func TestActiveViewEmptyState(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	av := NewActiveView(nil, styles)

	output := av.View(120, 40)
	if !strings.Contains(output, "No active sessions") {
		t.Fatalf("expected empty state message, got: %s", output)
	}
}

func TestActiveViewRendersSessions(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	av := NewActiveView(nil, styles)

	now := time.Now()
	av.sessions = []daemon.SessionInfo{
		{
			SessionID:    "s1",
			Topic:        "Fixing the bug",
			Project:      "/home/user/project-a",
			Branch:       "main",
			State:        "running",
			RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
		{
			SessionID:    "s2",
			Topic:        "",
			Project:      "/home/user/project-a",
			Branch:       "feature-x",
			State:        "ended",
			RegisteredAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
			EndedAt:      now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
		{
			SessionID:    "s3",
			Topic:        "Building UI",
			Project:      "/home/user/project-b",
			State:        "running",
			RegisteredAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
		},
	}

	output := av.View(120, 40)

	// Should show status indicators
	if !strings.Contains(output, "●") {
		t.Fatal("expected green dot (●) for running sessions")
	}
	if !strings.Contains(output, "○") {
		t.Fatal("expected gray dot (○) for ended sessions")
	}

	// Should show topics
	if !strings.Contains(output, "Fixing the bug") {
		t.Fatal("expected topic 'Fixing the bug' in output")
	}
	if !strings.Contains(output, "Building UI") {
		t.Fatal("expected topic 'Building UI' in output")
	}

	// Session without topic should fall back to branch
	if !strings.Contains(output, "feature-x") {
		t.Fatal("expected branch fallback 'feature-x' for session without topic")
	}
}

func TestActiveViewGroupsByProject(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	av := NewActiveView(nil, styles)

	now := time.Now()
	av.sessions = []daemon.SessionInfo{
		{
			SessionID:    "s1",
			Topic:        "First",
			Project:      "/home/user/project-a",
			State:        "running",
			RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
		{
			SessionID:    "s2",
			Topic:        "Second",
			Project:      "/home/user/project-b",
			State:        "running",
			RegisteredAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
		},
		{
			SessionID:    "s3",
			Topic:        "Third",
			Project:      "/home/user/project-a",
			State:        "running",
			RegisteredAt: now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}

	output := av.View(120, 40)

	// Should have project headers
	if !strings.Contains(output, "project-a") {
		t.Fatal("expected project-a group header")
	}
	if !strings.Contains(output, "project-b") {
		t.Fatal("expected project-b group header")
	}

	// project-a should appear before project-b (more recent session)
	idxA := strings.Index(output, "project-a")
	idxB := strings.Index(output, "project-b")
	if idxA > idxB {
		t.Fatal("expected project-a before project-b (has more recent session)")
	}
}

func TestActiveViewNavigation(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	av := NewActiveView(nil, styles)

	now := time.Now()
	av.sessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "First", Project: "/a", State: "running", RegisteredAt: now.Format(time.RFC3339)},
		{SessionID: "s2", Topic: "Second", Project: "/b", State: "running", RegisteredAt: now.Format(time.RFC3339)},
	}

	if av.cursor != 0 {
		t.Fatal("expected initial cursor at 0")
	}

	av.MoveDown()
	if av.cursor != 1 {
		t.Fatalf("expected cursor at 1 after down, got %d", av.cursor)
	}

	av.MoveDown() // wrap
	if av.cursor != 0 {
		t.Fatalf("expected cursor at 0 after wrap, got %d", av.cursor)
	}

	av.MoveUp() // wrap to end
	if av.cursor != 1 {
		t.Fatalf("expected cursor at 1 after up wrap, got %d", av.cursor)
	}
}

func TestActiveViewSelectedSession(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	av := NewActiveView(nil, styles)

	// Empty: should return nil
	if av.SelectedSession() != nil {
		t.Fatal("expected nil for empty sessions")
	}

	now := time.Now()
	av.sessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "First", Project: "/a", State: "running", RegisteredAt: now.Format(time.RFC3339)},
		{SessionID: "s2", Topic: "Second", Project: "/b", State: "running", RegisteredAt: now.Format(time.RFC3339)},
	}

	sel := av.SelectedSession()
	if sel == nil || sel.SessionID != "s1" {
		t.Fatal("expected first session selected")
	}

	av.MoveDown()
	sel = av.SelectedSession()
	if sel == nil || sel.SessionID != "s2" {
		t.Fatal("expected second session selected after move down")
	}
}

func TestActiveViewDaemonNotConnected(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	av := NewActiveView(nil, styles)

	// With no daemon client getter, should show daemon not connected
	output := av.View(120, 40)
	if !strings.Contains(output, "No active sessions") {
		t.Fatalf("expected empty state when daemon not connected, got: %s", output)
	}
}

func TestActiveViewDisplayOrder(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	av := NewActiveView(nil, styles)

	now := time.Now()
	av.sessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Older", Project: "/home/user/proj", State: "running", RegisteredAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
		{SessionID: "s2", Topic: "Newer", Project: "/home/user/proj", State: "running", RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339)},
	}

	// The display list should have newer first within a project group
	items := av.displayItems()
	if len(items) != 2 {
		t.Fatalf("expected 2 display items, got %d", len(items))
	}
	if items[0].session.Topic != "Newer" {
		t.Fatal("expected newer session first within group")
	}
}
