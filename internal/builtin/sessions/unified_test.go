package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
)

func newTestUnifiedView() *unifiedView {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	return NewUnifiedView(nil, styles)
}

// TestUnifiedViewMainMode verifies that the main (non-archive) mode
// shows LIVE and SAVED sections with appropriate content.
func TestUnifiedViewMainMode(t *testing.T) {
	uv := newTestUnifiedView()

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-001",
			Topic:        "Live Topic Here",
			Project:      "/home/user/project-a",
			State:        "active", // daemon uses "active", not "running"
			RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
	}
	uv.savedSessions = []db.Session{
		{
			SessionID: "saved-001",
			Project:   "/home/user/project-b",
			Repo:      "project-b",
			Branch:    "main",
			Summary:   "Saved summary text",
			Created:   now.Add(-1 * time.Hour),
			Type:      db.SessionBookmark,
		},
	}

	output := uv.View(120, 40)

	if !strings.Contains(output, "LIVE") {
		t.Errorf("expected LIVE section header in output:\n%s", output)
	}
	if !strings.Contains(output, "Live Topic Here") {
		t.Errorf("expected live topic in output:\n%s", output)
	}
	// "active" state should produce green ● indicator (not gray ○)
	if !strings.Contains(output, "●") {
		t.Errorf("expected green ● for active session:\n%s", output)
	}
	if !strings.Contains(output, "SAVED") {
		t.Errorf("expected SAVED section header in output:\n%s", output)
	}
	if !strings.Contains(output, "Saved summary text") {
		t.Errorf("expected saved summary in output:\n%s", output)
	}
}

// TestUnifiedViewArchiveMode verifies that archive mode shows ARCHIVED header
// and does NOT show LIVE or live sessions.
func TestUnifiedViewArchiveMode(t *testing.T) {
	uv := newTestUnifiedView()

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-001",
			Topic:        "ShouldNotAppear",
			Project:      "/home/user/project-a",
			State:        "running",
			RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
	}
	uv.archivedSessions = []db.ArchivedSession{
		{
			SessionID:    "arch-001",
			Topic:        "Archived Topic",
			Project:      "/home/user/project-c",
			RegisteredAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
			EndedAt:      now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}

	uv.ToggleArchive()

	output := uv.View(120, 40)

	if strings.Contains(output, "LIVE") {
		t.Errorf("archive mode should NOT show LIVE section:\n%s", output)
	}
	if strings.Contains(output, "ShouldNotAppear") {
		t.Errorf("archive mode should NOT show live session topic:\n%s", output)
	}
	if !strings.Contains(output, "ARCHIVED") {
		t.Errorf("expected ARCHIVED section header in output:\n%s", output)
	}
	if !strings.Contains(output, "Archived Topic") {
		t.Errorf("expected archived topic in output:\n%s", output)
	}
}

// TestUnifiedViewToggleMode verifies that ToggleArchive flips the mode
// and resets cursor to 0.
func TestUnifiedViewToggleMode(t *testing.T) {
	uv := newTestUnifiedView()

	if uv.archiveMode {
		t.Fatal("expected archiveMode to start false")
	}

	// Set cursor to non-zero to verify reset
	uv.cursor = 3

	uv.ToggleArchive()

	if !uv.archiveMode {
		t.Error("expected archiveMode to be true after first toggle")
	}
	if uv.cursor != 0 {
		t.Errorf("expected cursor reset to 0 after toggle, got %d", uv.cursor)
	}

	uv.ToggleArchive()

	if uv.archiveMode {
		t.Error("expected archiveMode to be false after second toggle")
	}
	if uv.cursor != 0 {
		t.Errorf("expected cursor still at 0 after second toggle, got %d", uv.cursor)
	}
}

// TestUnifiedViewDeduplication verifies that a session appearing in both
// live and saved lists shows the bookmark indicator (★) and the topic
// appears only once in the Live section (not duplicated in Saved).
func TestUnifiedViewDeduplication(t *testing.T) {
	uv := newTestUnifiedView()

	now := time.Now()
	sharedID := "shared-session-id"

	uv.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    sharedID,
			Topic:        "Shared Topic",
			Project:      "/home/user/project-a",
			State:        "running",
			RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
	}
	uv.savedSessions = []db.Session{
		{
			SessionID: sharedID,
			Project:   "/home/user/project-a",
			Repo:      "project-a",
			Branch:    "main",
			Summary:   "Shared session bookmark",
			Created:   now.Add(-10 * time.Minute),
			Type:      db.SessionBookmark,
		},
	}

	output := uv.View(120, 40)

	// Should show bookmark star indicator
	if !strings.Contains(output, "★") {
		t.Errorf("expected ★ bookmark indicator for shared session:\n%s", output)
	}

	// Topic should appear exactly once (live section only, not duplicated in saved)
	count := strings.Count(output, "Shared Topic")
	if count != 1 {
		t.Errorf("expected 'Shared Topic' to appear exactly once, got %d times:\n%s", count, output)
	}
}

// TestUnifiedViewEmptyState verifies that empty view shows "No sessions".
func TestUnifiedViewEmptyState(t *testing.T) {
	uv := newTestUnifiedView()

	output := uv.View(120, 40)

	if !strings.Contains(output, "No sessions") {
		t.Errorf("expected 'No sessions' in empty state output:\n%s", output)
	}
}

// TestUnifiedViewNavigation verifies MoveDown and MoveUp with wrapping.
func TestUnifiedViewNavigation(t *testing.T) {
	uv := newTestUnifiedView()

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-001",
			Topic:        "Live One",
			Project:      "/home/user/a",
			State:        "running",
			RegisteredAt: now.Format(time.RFC3339),
		},
	}
	uv.savedSessions = []db.Session{
		{
			SessionID: "saved-001",
			Project:   "/home/user/b",
			Repo:      "b",
			Branch:    "main",
			Summary:   "Saved One",
			Created:   now.Add(-1 * time.Hour),
			Type:      db.SessionBookmark,
		},
	}

	if uv.cursor != 0 {
		t.Fatalf("expected initial cursor at 0, got %d", uv.cursor)
	}

	uv.MoveDown()
	if uv.cursor != 1 {
		t.Fatalf("expected cursor at 1 after MoveDown, got %d", uv.cursor)
	}

	uv.MoveDown() // should wrap to 0
	if uv.cursor != 0 {
		t.Fatalf("expected cursor to wrap to 0, got %d", uv.cursor)
	}
}

func TestUnifiedViewAutoArchive(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	uv := newTestUnifiedView()
	uv.db = database

	now := time.Now()

	// Simulate: previous refresh had s1 running
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Was running", Project: "/proj", Repo: "repo", Branch: "main", State: "active", RegisteredAt: now.Add(-10 * time.Minute).Format(time.RFC3339)},
	}

	// Now s1 has ended
	uv.archiveEndedSessions([]daemon.SessionInfo{
		{SessionID: "s1", Topic: "Was running", Project: "/proj", Repo: "repo", Branch: "main", State: "ended", RegisteredAt: now.Add(-10 * time.Minute).Format(time.RFC3339), EndedAt: now.Format(time.RFC3339)},
	})

	// Should be in archived sessions table
	archived, _ := db.DBLoadArchivedSessions(database)
	if len(archived) != 1 {
		t.Fatalf("expected 1 archived, got %d", len(archived))
	}
	if archived[0].SessionID != "s1" {
		t.Fatalf("expected s1 archived, got %s", archived[0].SessionID)
	}
}

func TestUnifiedViewAutoArchiveSkipsBookmarked(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	// Pre-bookmark s1
	_ = db.DBInsertBookmark(database, db.Session{SessionID: "s1", Project: "/proj", Branch: "main", Created: time.Now()}, "test")

	uv := newTestUnifiedView()
	uv.db = database

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Bookmarked", Project: "/proj", State: "active", RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339)},
	}

	uv.archiveEndedSessions([]daemon.SessionInfo{
		{SessionID: "s1", Topic: "Bookmarked", Project: "/proj", State: "ended", RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339), EndedAt: now.Format(time.RFC3339)},
	})

	// Should NOT be archived (it's bookmarked)
	archived, _ := db.DBLoadArchivedSessions(database)
	if len(archived) != 0 {
		t.Fatalf("expected 0 archived (bookmarked session), got %d", len(archived))
	}
}

// TestUnifiedViewSelectedItem verifies SelectedItem returns the correct
// UnifiedItem with the correct Tier.
func TestUnifiedViewSelectedItem(t *testing.T) {
	uv := newTestUnifiedView()

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{
			SessionID:    "live-001",
			Topic:        "Live Session",
			Project:      "/home/user/a",
			State:        "running",
			RegisteredAt: now.Format(time.RFC3339),
		},
	}
	uv.savedSessions = []db.Session{
		{
			SessionID: "saved-001",
			Project:   "/home/user/b",
			Repo:      "b",
			Branch:    "main",
			Summary:   "Saved Session",
			Created:   now.Add(-1 * time.Hour),
			Type:      db.SessionBookmark,
		},
	}

	// Cursor 0 → live item
	item := uv.SelectedItem()
	if item == nil {
		t.Fatal("expected non-nil SelectedItem at cursor 0")
	}
	if item.Tier != TierLive {
		t.Errorf("expected Tier=%q at cursor 0, got %q", TierLive, item.Tier)
	}
	if item.SessionID != "live-001" {
		t.Errorf("expected SessionID=live-001, got %q", item.SessionID)
	}

	uv.MoveDown()

	// Cursor 1 → saved item
	item = uv.SelectedItem()
	if item == nil {
		t.Fatal("expected non-nil SelectedItem at cursor 1")
	}
	if item.Tier != TierSaved {
		t.Errorf("expected Tier=%q at cursor 1, got %q", TierSaved, item.Tier)
	}
	if item.SessionID != "saved-001" {
		t.Errorf("expected SessionID=saved-001, got %q", item.SessionID)
	}
}
