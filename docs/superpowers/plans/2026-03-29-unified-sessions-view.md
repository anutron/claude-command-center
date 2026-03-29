# Unified Sessions View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge the "active" and "resume" sub-tabs into a single unified sessions view with three tiers (Live, Saved, Archived) and an archive toggle.

**Architecture:** Replace the separate `activeView` + `resumeList` with a new `unifiedView` struct that combines daemon sessions (Live), bookmarks (Saved), and a new `cc_archived_sessions` DB table (Archived). The view has two modes: main (Live+Saved) and archive. The `a` key toggles between modes instead of switching sub-tabs.

**Tech Stack:** Go, bubbletea, SQLite, lipgloss

**Design spec:** `docs/superpowers/specs/2026-03-29-unified-sessions-view-design.md`

---

### Task 1: Add `cc_archived_sessions` Table and DB Functions

**Files:**
- Modify: `internal/db/schema.go` (add CREATE TABLE in schema block)
- Modify: `internal/db/write.go` (add insert/delete functions)
- Modify: `internal/db/read.go` (add load function)
- Create: `internal/db/archived_sessions_test.go`

- [ ] **Step 1: Write failing tests for archived session DB operations**

```go
// internal/db/archived_sessions_test.go
package db

import (
	"testing"
	"time"
)

func TestArchivedSessionInsertAndLoad(t *testing.T) {
	db := openTestDB(t)

	sess := ArchivedSession{
		SessionID:    "abc-123",
		Topic:        "Fixing bug",
		Project:      "/home/user/proj",
		Repo:         "origin/proj",
		Branch:       "main",
		WorktreePath: "",
		RegisteredAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		EndedAt:      time.Now().Format(time.RFC3339),
	}
	err := DBInsertArchivedSession(db, sess)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	sessions, err := DBLoadArchivedSessions(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1, got %d", len(sessions))
	}
	if sessions[0].SessionID != "abc-123" {
		t.Fatalf("expected abc-123, got %s", sessions[0].SessionID)
	}
	if sessions[0].Topic != "Fixing bug" {
		t.Fatalf("expected topic, got %s", sessions[0].Topic)
	}
}

func TestArchivedSessionDelete(t *testing.T) {
	db := openTestDB(t)

	sess := ArchivedSession{
		SessionID:    "abc-123",
		Topic:        "Fixing bug",
		Project:      "/home/user/proj",
		RegisteredAt: time.Now().Format(time.RFC3339),
		EndedAt:      time.Now().Format(time.RFC3339),
	}
	_ = DBInsertArchivedSession(db, sess)

	err := DBDeleteArchivedSession(db, "abc-123")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	sessions, _ := DBLoadArchivedSessions(db)
	if len(sessions) != 0 {
		t.Fatalf("expected 0, got %d", len(sessions))
	}
}

func TestArchivedSessionUpsert(t *testing.T) {
	db := openTestDB(t)

	sess := ArchivedSession{
		SessionID:    "abc-123",
		Topic:        "Original",
		Project:      "/home/user/proj",
		RegisteredAt: time.Now().Format(time.RFC3339),
		EndedAt:      time.Now().Format(time.RFC3339),
	}
	_ = DBInsertArchivedSession(db, sess)

	sess.Topic = "Updated"
	_ = DBInsertArchivedSession(db, sess)

	sessions, _ := DBLoadArchivedSessions(db)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 after upsert, got %d", len(sessions))
	}
	if sessions[0].Topic != "Updated" {
		t.Fatalf("expected Updated, got %s", sessions[0].Topic)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/ -run TestArchivedSession -v`
Expected: Compilation error — `ArchivedSession` type and DB functions don't exist yet.

- [ ] **Step 3: Add ArchivedSession type**

Add to `internal/db/sessions.go` after the `Session` struct:

```go
// ArchivedSession represents an auto-saved ended session in cc_archived_sessions.
type ArchivedSession struct {
	SessionID    string
	Topic        string
	Project      string
	Repo         string
	Branch       string
	WorktreePath string
	RegisteredAt string
	EndedAt      string
}
```

- [ ] **Step 4: Add CREATE TABLE to schema**

In `internal/db/schema.go`, add inside the multi-table `CREATE TABLE` block (after the `cc_bookmarks` table):

```sql
CREATE TABLE IF NOT EXISTS cc_archived_sessions (
    session_id TEXT PRIMARY KEY,
    topic TEXT,
    project TEXT,
    repo TEXT,
    branch TEXT,
    worktree_path TEXT,
    registered_at TEXT NOT NULL,
    ended_at TEXT NOT NULL
);
```

- [ ] **Step 5: Add write functions**

In `internal/db/write.go`, add:

```go
// DBInsertArchivedSession inserts or replaces an archived session.
func DBInsertArchivedSession(db *sql.DB, s ArchivedSession) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO cc_archived_sessions
		(session_id, topic, project, repo, branch, worktree_path, registered_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.SessionID, s.Topic, s.Project, s.Repo, s.Branch, s.WorktreePath, s.RegisteredAt, s.EndedAt)
	return err
}

// DBDeleteArchivedSession removes an archived session by ID.
func DBDeleteArchivedSession(db *sql.DB, sessionID string) error {
	_, err := db.Exec(`DELETE FROM cc_archived_sessions WHERE session_id = ?`, sessionID)
	return err
}
```

- [ ] **Step 6: Add read function**

In `internal/db/read.go`, add:

```go
// DBLoadArchivedSessions loads all archived sessions, most recent first.
func DBLoadArchivedSessions(db *sql.DB) ([]ArchivedSession, error) {
	rows, err := db.Query(`SELECT session_id, topic, project, repo, branch, worktree_path, registered_at, ended_at
		FROM cc_archived_sessions ORDER BY ended_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []ArchivedSession
	for rows.Next() {
		var s ArchivedSession
		if err := rows.Scan(&s.SessionID, &s.Topic, &s.Project, &s.Repo, &s.Branch, &s.WorktreePath, &s.RegisteredAt, &s.EndedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/db/ -run TestArchivedSession -v`
Expected: All 3 tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/db/schema.go internal/db/sessions.go internal/db/write.go internal/db/read.go internal/db/archived_sessions_test.go
git commit -m "Add cc_archived_sessions table and DB functions"
```

---

### Task 2: Create `unifiedView` Struct

This replaces the old `activeView` with a combined view that holds live sessions, bookmarks, and archived sessions, with mode toggling.

**Files:**
- Create: `internal/builtin/sessions/unified.go`
- Create: `internal/builtin/sessions/unified_test.go`

- [ ] **Step 1: Write failing tests for the unified view**

```go
// internal/builtin/sessions/unified_test.go
package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
)

func TestUnifiedViewMainMode(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Live one", Project: "/proj", State: "running", RegisteredAt: now.Format(time.RFC3339)},
	}
	uv.savedSessions = []db.Session{
		{SessionID: "s2", Project: "/proj", Repo: "origin/proj", Branch: "main", Summary: "Saved one", Created: now.Add(-1 * time.Hour)},
	}

	output := uv.View(120, 40)
	if !strings.Contains(output, "LIVE") {
		t.Fatal("expected LIVE section header")
	}
	if !strings.Contains(output, "Live one") {
		t.Fatal("expected live session topic")
	}
	if !strings.Contains(output, "SAVED") {
		t.Fatal("expected SAVED section header")
	}
	if !strings.Contains(output, "Saved one") {
		t.Fatal("expected saved session summary")
	}
}

func TestUnifiedViewArchiveMode(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Live", Project: "/proj", State: "running", RegisteredAt: now.Format(time.RFC3339)},
	}
	uv.archivedSessions = []db.ArchivedSession{
		{SessionID: "s3", Topic: "Archived one", Project: "/proj", RegisteredAt: now.Add(-2 * time.Hour).Format(time.RFC3339), EndedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
	}
	uv.archiveMode = true

	output := uv.View(120, 40)
	if strings.Contains(output, "LIVE") {
		t.Fatal("LIVE section should not appear in archive mode")
	}
	if !strings.Contains(output, "ARCHIVED") {
		t.Fatal("expected ARCHIVED section header")
	}
	if !strings.Contains(output, "Archived one") {
		t.Fatal("expected archived session")
	}
}

func TestUnifiedViewToggleMode(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)

	if uv.archiveMode {
		t.Fatal("expected main mode by default")
	}
	uv.ToggleArchive()
	if !uv.archiveMode {
		t.Fatal("expected archive mode after toggle")
	}
	if uv.cursor != 0 {
		t.Fatal("expected cursor reset on toggle")
	}
	uv.ToggleArchive()
	if uv.archiveMode {
		t.Fatal("expected main mode after second toggle")
	}
}

func TestUnifiedViewDeduplication(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Live and bookmarked", Project: "/proj", State: "running", RegisteredAt: now.Format(time.RFC3339)},
	}
	uv.savedSessions = []db.Session{
		{SessionID: "s1", Project: "/proj", Repo: "origin/proj", Branch: "main", Summary: "Same session", Created: now},
	}

	output := uv.View(120, 40)
	// Should show bookmark indicator on the live session
	if !strings.Contains(output, "\u2605") { // ★
		t.Fatal("expected bookmark indicator on live session that is also saved")
	}
	// Should NOT show the session in saved section (it's already in live)
	// Count occurrences of the session topic — should appear once
	if strings.Count(output, "Live and bookmarked") != 1 {
		t.Fatal("expected session to appear only once (in Live, not duplicated in Saved)")
	}
}

func TestUnifiedViewEmptyState(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)

	output := uv.View(120, 40)
	if !strings.Contains(output, "No sessions") {
		t.Fatalf("expected empty state, got: %s", output)
	}
}

func TestUnifiedViewNavigation(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "First", Project: "/proj", State: "running", RegisteredAt: now.Format(time.RFC3339)},
	}
	uv.savedSessions = []db.Session{
		{SessionID: "s2", Project: "/proj", Repo: "repo", Branch: "main", Summary: "Second", Created: now},
	}

	if uv.cursor != 0 {
		t.Fatal("expected cursor at 0")
	}
	uv.MoveDown()
	if uv.cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", uv.cursor)
	}
	uv.MoveDown() // wrap
	if uv.cursor != 0 {
		t.Fatalf("expected cursor at 0 after wrap, got %d", uv.cursor)
	}
}

func TestUnifiedViewSelectedItem(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Live", Project: "/proj-a", State: "running", RegisteredAt: now.Format(time.RFC3339)},
	}
	uv.savedSessions = []db.Session{
		{SessionID: "s2", Project: "/proj-b", Repo: "repo", Branch: "feat", Summary: "Saved", Created: now},
	}

	sel := uv.SelectedItem()
	if sel == nil || sel.SessionID != "s1" {
		t.Fatal("expected first item (live) selected")
	}
	if sel.Tier != TierLive {
		t.Fatal("expected TierLive")
	}

	uv.MoveDown()
	sel = uv.SelectedItem()
	if sel == nil || sel.SessionID != "s2" {
		t.Fatal("expected second item (saved) selected")
	}
	if sel.Tier != TierSaved {
		t.Fatal("expected TierSaved")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/builtin/sessions/ -run TestUnifiedView -v`
Expected: Compilation error — `NewUnifiedView`, `unifiedView`, etc. don't exist.

- [ ] **Step 3: Implement the unified view**

```go
// internal/builtin/sessions/unified.go
package sessions

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/lipgloss"
)

// Tier constants for identifying which section an item belongs to.
const (
	TierLive     = "live"
	TierSaved    = "saved"
	TierArchived = "archived"
)

// UnifiedItem is a flattened item for rendering and selection.
type UnifiedItem struct {
	SessionID    string
	Topic        string
	Project      string
	Repo         string
	Branch       string
	WorktreePath string
	RegisteredAt string
	EndedAt      string
	State        string // daemon state (running/active/blocked/ended) — empty for saved/archived
	Tier         string // TierLive, TierSaved, TierArchived
	IsBookmarked bool   // true if this live session is also in bookmarks
	IsFirst      bool   // first item in its section
}

// unifiedView combines live, saved, and archived sessions into one navigable view.
type unifiedView struct {
	liveSessions     []daemon.SessionInfo
	savedSessions    []db.Session
	archivedSessions []db.ArchivedSession
	agentsByID       map[string]daemon.AgentStatusResult
	cursor           int
	archiveMode      bool
	daemonClient     func() *daemon.Client
	styles           sessionStyles
}

// NewUnifiedView creates a new unified sessions view.
func NewUnifiedView(clientFn func() *daemon.Client, styles sessionStyles) *unifiedView {
	return &unifiedView{
		daemonClient: clientFn,
		styles:       styles,
	}
}

// ToggleArchive switches between main and archive mode, resetting the cursor.
func (uv *unifiedView) ToggleArchive() {
	uv.archiveMode = !uv.archiveMode
	uv.cursor = 0
}

// displayItems returns the flat list of items for the current mode.
func (uv *unifiedView) displayItems() []UnifiedItem {
	if uv.archiveMode {
		return uv.archivedItems()
	}
	return uv.mainItems()
}

// mainItems returns Live + Saved items, with deduplication.
func (uv *unifiedView) mainItems() []UnifiedItem {
	// Build set of bookmarked session IDs for dedup
	bookmarkedIDs := make(map[string]bool, len(uv.savedSessions))
	for _, s := range uv.savedSessions {
		bookmarkedIDs[s.SessionID] = true
	}

	var items []UnifiedItem

	// Live section: sorted by recency
	liveSorted := make([]daemon.SessionInfo, len(uv.liveSessions))
	copy(liveSorted, uv.liveSessions)
	sort.Slice(liveSorted, func(i, j int) bool {
		return parseTime(liveSorted[i].RegisteredAt).After(parseTime(liveSorted[j].RegisteredAt))
	})

	for i, s := range liveSorted {
		items = append(items, UnifiedItem{
			SessionID:    s.SessionID,
			Topic:        s.Topic,
			Project:      s.Project,
			Repo:         s.Repo,
			Branch:       s.Branch,
			WorktreePath: s.WorktreePath,
			RegisteredAt: s.RegisteredAt,
			State:        s.State,
			Tier:         TierLive,
			IsBookmarked: bookmarkedIDs[s.SessionID],
			IsFirst:      i == 0,
		})
	}

	// Saved section: exclude sessions already in Live
	liveIDs := make(map[string]bool, len(uv.liveSessions))
	for _, s := range uv.liveSessions {
		liveIDs[s.SessionID] = true
	}

	first := true
	for _, s := range uv.savedSessions {
		if liveIDs[s.SessionID] {
			continue // already shown in Live with ★
		}
		items = append(items, UnifiedItem{
			SessionID:    s.SessionID,
			Topic:        s.Summary,
			Project:      s.Project,
			Repo:         s.Repo,
			Branch:       s.Branch,
			WorktreePath: s.WorktreePath,
			Tier:         TierSaved,
			IsFirst:      first,
		})
		first = false
	}

	return items
}

// archivedItems returns archived sessions for archive mode.
func (uv *unifiedView) archivedItems() []UnifiedItem {
	var items []UnifiedItem
	for i, s := range uv.archivedSessions {
		items = append(items, UnifiedItem{
			SessionID:    s.SessionID,
			Topic:        s.Topic,
			Project:      s.Project,
			Repo:         s.Repo,
			Branch:       s.Branch,
			WorktreePath: s.WorktreePath,
			RegisteredAt: s.RegisteredAt,
			EndedAt:      s.EndedAt,
			Tier:         TierArchived,
			IsFirst:      i == 0,
		})
	}
	return items
}

// View renders the unified view.
func (uv *unifiedView) View(width, height int) string {
	items := uv.displayItems()

	if len(items) == 0 {
		if uv.archiveMode {
			return uv.styles.hint.Render("  No archived sessions.")
		}
		return uv.styles.hint.Render("  No sessions. Start a Claude session to see it here.")
	}

	var lines []string
	currentSection := ""

	for idx, item := range items {
		// Section header
		section := item.Tier
		if section != currentSection {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			var header string
			switch section {
			case TierLive:
				header = "LIVE"
			case TierSaved:
				header = "SAVED"
			case TierArchived:
				header = "ARCHIVED"
			}
			lines = append(lines, uv.styles.sectionHeader.Render("  "+header))
			currentSection = section
		}

		// Status indicator
		var indicator string
		switch item.Tier {
		case TierLive:
			if item.State == "running" || item.State == "active" {
				if uv.isSessionBlocked(item.SessionID) {
					indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")).Render("●")
				} else {
					indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Render("●")
				}
			} else {
				indicator = uv.styles.descMuted.Render("○")
			}
		case TierSaved:
			indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#bd93f9")).Render("★")
		case TierArchived:
			indicator = uv.styles.descMuted.Render("○")
		}

		// Label
		label := item.Topic
		if label == "" {
			if item.Branch != "" {
				label = item.Branch
			} else {
				label = filepath.Base(item.Project)
			}
		}

		// Bookmark indicator for live sessions
		bookmarkMark := ""
		if item.Tier == TierLive && item.IsBookmarked {
			bookmarkMark = " \u2605" // ★
		}

		// Right side info
		var suffix string
		switch item.Tier {
		case TierLive:
			suffix = sessionAge(item.RegisteredAt)
			if item.State == "running" && uv.isSessionBlocked(item.SessionID) {
				suffix = lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")).Render("Blocked") + "  " + suffix
			}
		case TierSaved:
			suffix = filepath.Base(item.Project)
			if item.Branch != "" {
				suffix += " (" + item.Branch + ")"
			}
		case TierArchived:
			suffix = sessionAge(item.EndedAt)
		}

		// Build line
		pointer := "  "
		if idx == uv.cursor {
			pointer = "> "
		}

		labelStyle := uv.styles.titleBoldW
		if idx == uv.cursor {
			labelStyle = uv.styles.selectedItem
		}

		line := fmt.Sprintf("%s%s %s%s  %s",
			pointer,
			indicator,
			labelStyle.Render(label),
			bookmarkMark,
			uv.styles.descMuted.Render(suffix),
		)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// MoveDown moves the cursor down, wrapping around.
func (uv *unifiedView) MoveDown() {
	total := len(uv.displayItems())
	if total == 0 {
		return
	}
	uv.cursor++
	if uv.cursor >= total {
		uv.cursor = 0
	}
}

// MoveUp moves the cursor up, wrapping around.
func (uv *unifiedView) MoveUp() {
	total := len(uv.displayItems())
	if total == 0 {
		return
	}
	uv.cursor--
	if uv.cursor < 0 {
		uv.cursor = total - 1
	}
}

// SelectedItem returns the currently selected unified item, or nil if empty.
func (uv *unifiedView) SelectedItem() *UnifiedItem {
	items := uv.displayItems()
	if len(items) == 0 || uv.cursor >= len(items) {
		return nil
	}
	item := items[uv.cursor]
	return &item
}

// RemoveSession removes a session by ID from whichever list it's in.
func (uv *unifiedView) RemoveSession(sessionID string) {
	for i, s := range uv.liveSessions {
		if s.SessionID == sessionID {
			uv.liveSessions = append(uv.liveSessions[:i], uv.liveSessions[i+1:]...)
			break
		}
	}
	for i, s := range uv.savedSessions {
		if s.SessionID == sessionID {
			uv.savedSessions = append(uv.savedSessions[:i], uv.savedSessions[i+1:]...)
			break
		}
	}
	for i, s := range uv.archivedSessions {
		if s.SessionID == sessionID {
			uv.archivedSessions = append(uv.archivedSessions[:i], uv.archivedSessions[i+1:]...)
			break
		}
	}
	total := len(uv.displayItems())
	if uv.cursor >= total && uv.cursor > 0 {
		uv.cursor = total - 1
	}
}

// Refresh fetches live sessions from the daemon.
func (uv *unifiedView) Refresh() {
	if uv.daemonClient == nil {
		return
	}
	client := uv.daemonClient()
	if client == nil {
		return
	}
	sessions, err := client.ListSessions()
	if err != nil {
		return
	}
	uv.liveSessions = sessions
	// Clamp cursor
	total := len(uv.displayItems())
	if uv.cursor >= total && total > 0 {
		uv.cursor = total - 1
	}

	// Fetch agent statuses for blocked detection
	uv.agentsByID = nil
	agents, err := client.ListAgents()
	if err != nil {
		return
	}
	if len(agents) > 0 {
		uv.agentsByID = make(map[string]daemon.AgentStatusResult, len(agents))
		for _, a := range agents {
			uv.agentsByID[a.ID] = a
		}
	}
}

// isSessionBlocked checks if a session has a CCC-spawned agent in "blocked" state.
func (uv *unifiedView) isSessionBlocked(sessionID string) bool {
	if len(uv.agentsByID) == 0 || sessionID == "" {
		return false
	}
	for _, a := range uv.agentsByID {
		if a.SessionID == sessionID && a.Status == "blocked" {
			return true
		}
		if a.ID == sessionID && a.Status == "blocked" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/builtin/sessions/ -run TestUnifiedView -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/builtin/sessions/unified.go internal/builtin/sessions/unified_test.go
git commit -m "Add unifiedView struct combining live, saved, and archived sessions"
```

---

### Task 3: Auto-Archive Ended Sessions in Refresh

**Files:**
- Modify: `internal/builtin/sessions/unified.go` (add archive detection to Refresh)
- Modify: `internal/builtin/sessions/unified_test.go` (add archive detection test)

The `Refresh()` method needs to detect newly ended sessions and auto-archive them.

- [ ] **Step 1: Write failing test for auto-archive**

Add to `unified_test.go`:

```go
func TestUnifiedViewAutoArchive(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)
	uv.db = database

	now := time.Now()

	// Simulate: previous refresh had s1 running
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Was running", Project: "/proj", Repo: "repo", Branch: "main", State: "running", RegisteredAt: now.Add(-10 * time.Minute).Format(time.RFC3339)},
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

	pal := config.GetPalette("aurora", nil)
	styles := newSessionStyles(pal)
	uv := NewUnifiedView(nil, styles)
	uv.db = database

	now := time.Now()
	uv.liveSessions = []daemon.SessionInfo{
		{SessionID: "s1", Topic: "Bookmarked", Project: "/proj", State: "running", RegisteredAt: now.Add(-5 * time.Minute).Format(time.RFC3339)},
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/builtin/sessions/ -run TestUnifiedViewAutoArchive -v`
Expected: Compilation error — `uv.db` and `archiveEndedSessions` don't exist.

- [ ] **Step 3: Add `db` field and `archiveEndedSessions` method**

In `unified.go`, add `db` field to the struct:

```go
type unifiedView struct {
	liveSessions     []daemon.SessionInfo
	savedSessions    []db.Session
	archivedSessions []db.ArchivedSession
	agentsByID       map[string]daemon.AgentStatusResult
	cursor           int
	archiveMode      bool
	daemonClient     func() *daemon.Client
	db               *sql.DB // for auto-archiving
	styles           sessionStyles
}
```

Add the `archiveEndedSessions` method:

```go
// archiveEndedSessions compares the new session list against the previous one
// and auto-archives any newly ended sessions that aren't bookmarked.
func (uv *unifiedView) archiveEndedSessions(newSessions []daemon.SessionInfo) {
	if uv.db == nil {
		return
	}

	// Build set of previously running session IDs
	prevRunning := make(map[string]daemon.SessionInfo, len(uv.liveSessions))
	for _, s := range uv.liveSessions {
		if s.State == "running" || s.State == "active" {
			prevRunning[s.SessionID] = s
		}
	}

	// Build set of bookmarked IDs
	bookmarks, _ := db.DBLoadBookmarks(uv.db)
	bookmarkedIDs := make(map[string]bool, len(bookmarks))
	for _, b := range bookmarks {
		bookmarkedIDs[b.SessionID] = true
	}

	// Find sessions that were previously running but are now ended
	newByID := make(map[string]daemon.SessionInfo, len(newSessions))
	for _, s := range newSessions {
		newByID[s.SessionID] = s
	}

	for id, prev := range prevRunning {
		newState, exists := newByID[id]
		ended := !exists || newState.State == "ended"
		if !ended {
			continue
		}
		if bookmarkedIDs[id] {
			continue
		}

		endedAt := time.Now().Format(time.RFC3339)
		if exists && newState.EndedAt != "" {
			endedAt = newState.EndedAt
		}

		_ = db.DBInsertArchivedSession(uv.db, db.ArchivedSession{
			SessionID:    id,
			Topic:        prev.Topic,
			Project:      prev.Project,
			Repo:         prev.Repo,
			Branch:       prev.Branch,
			WorktreePath: prev.WorktreePath,
			RegisteredAt: prev.RegisteredAt,
			EndedAt:      endedAt,
		})
	}
}
```

Update `Refresh()` to call `archiveEndedSessions` before updating `liveSessions`:

```go
func (uv *unifiedView) Refresh() {
	if uv.daemonClient == nil {
		return
	}
	client := uv.daemonClient()
	if client == nil {
		return
	}
	sessions, err := client.ListSessions()
	if err != nil {
		return
	}

	// Auto-archive newly ended sessions before updating the live list
	uv.archiveEndedSessions(sessions)

	uv.liveSessions = sessions
	// ... rest of Refresh (clamp cursor, fetch agents) stays the same
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/builtin/sessions/ -run TestUnifiedViewAutoArchive -v`
Expected: Both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/builtin/sessions/unified.go internal/builtin/sessions/unified_test.go
git commit -m "Add auto-archive for ended sessions in unified view refresh"
```

---

### Task 4: Wire Unified View Into Plugin (Replace Active + Resume)

This is the integration task. Replace `activeView` + `resumeList` with `unifiedView` in the Plugin struct, update Init, Routes, NavigateTo, HandleKey, HandleMessage, and View.

**Files:**
- Modify: `internal/builtin/sessions/sessions.go`
- Modify: `internal/builtin/sessions/unified.go` (add `SetSavedSessions` and `SetArchivedSessions`)
- Delete: `internal/builtin/sessions/active.go` (replaced by unified.go)
- Delete: `internal/builtin/sessions/active_test.go` (replaced by unified_test.go)

- [ ] **Step 1: Add setter methods to unifiedView**

In `unified.go`, add:

```go
// SetSavedSessions updates the saved (bookmarked) sessions list.
func (uv *unifiedView) SetSavedSessions(sessions []db.Session) {
	uv.savedSessions = sessions
}

// SetArchivedSessions updates the archived sessions list.
func (uv *unifiedView) SetArchivedSessions(sessions []db.ArchivedSession) {
	uv.archivedSessions = sessions
}

// ReloadArchived loads archived sessions from DB.
func (uv *unifiedView) ReloadArchived() {
	if uv.db == nil {
		return
	}
	sessions, _ := db.DBLoadArchivedSessions(uv.db)
	uv.archivedSessions = sessions
}
```

- [ ] **Step 2: Update Plugin struct**

In `sessions.go`, replace the `resumeList` and `active` fields:

Remove:
```go
resumeList    list.Model
active         *activeView
```

Add:
```go
unified *unifiedView
```

- [ ] **Step 3: Update Init()**

Replace the `activeView` creation (line ~369) and the resume list setup:

```go
// Replace: p.active = NewActiveView(nil, p.styles)
// Replace: p.resumeList = ... setup ...
p.unified = NewUnifiedView(nil, p.styles)
p.unified.db = database
```

Update the `data.refreshed` subscription to use unified:

```go
p.bus.Subscribe("data.refreshed", func(e plugin.Event) {
    if p.db != nil {
        sessions, _ := db.DBLoadBookmarks(p.db)
        p.unified.SetSavedSessions(sessions)
        p.unified.ReloadArchived()
    }
})
```

Update the daemon session event subscriptions:

```go
for _, topic := range []string{"session.registered", "session.updated", "session.ended"} {
    p.bus.Subscribe(topic, func(e plugin.Event) {
        if p.unified != nil {
            p.unified.Refresh()
            p.unified.ReloadArchived()
        }
    })
}
```

Also load initial bookmarks and archived sessions into unified during Init (where bookmarks were previously loaded into resumeList):

```go
if p.db != nil {
    sessions, _ := db.DBLoadBookmarks(p.db)
    p.unified.SetSavedSessions(sessions)
    p.unified.ReloadArchived()
}
```

- [ ] **Step 4: Update Routes()**

```go
func (p *Plugin) Routes() []plugin.Route {
    return []plugin.Route{
        {Slug: "sessions", Description: "Sessions sub-tab"},
        {Slug: "new", Description: "New session sub-tab"},
        {Slug: "worktrees", Description: "Worktrees sub-tab"},
    }
}
```

- [ ] **Step 5: Update NavigateTo()**

Replace the `"active"` and `"resume"` cases with a single `"sessions"` case:

```go
func (p *Plugin) NavigateTo(route string, args map[string]string) {
    p.filterText = ""
    switch route {
    case "sessions":
        p.subTab = "sessions"
        if p.unified != nil {
            p.unified.Refresh()
            p.unified.ReloadArchived()
        }
    case "new":
        p.subTab = "new"
        p.applyFilter()
    case "worktrees":
        p.subTab = "worktrees"
        p.refreshWorktreeList()
    }
    if todoTitle, ok := args["pending_todo_title"]; ok {
        p.pendingLaunchTodo = &db.Todo{Title: todoTitle}
    }
}
```

- [ ] **Step 6: Update HandleKey() sub-tab switching**

Replace the `"a"`, `"n"`, `"r"`, `"t"` cases:

```go
case "s":
    if !filtering {
        p.subTab = "sessions"
        p.filterText = ""
        if p.unified != nil {
            p.unified.Refresh()
            p.unified.ReloadArchived()
        }
        return plugin.NoopAction()
    }
case "n":
    if !filtering {
        p.subTab = "new"
        p.filterText = ""
        return plugin.NoopAction()
    }
case "t":
    if !filtering {
        p.subTab = "worktrees"
        p.filterText = ""
        p.refreshWorktreeList()
        return plugin.NoopAction()
    }
```

Remove the `"a"` and `"r"` sub-tab switching cases entirely.

Update the `esc` handler: replace `p.subTab == "active"` with `p.subTab == "sessions"`:

```go
case "esc":
    if filtering {
        p.filterText = ""
        p.applyFilter()
        return plugin.NoopAction()
    }
    if p.subTab == "sessions" || p.subTab == "worktrees" {
        p.subTab = "new"
        return plugin.NoopAction()
    }
```

Update the sub-tab delegate switch:

```go
switch p.subTab {
case "sessions":
    return p.handleSessionsTab(msg)
case "new":
    return p.handleNewTab(msg)
case "worktrees":
    return p.handleWorktreesTab(msg)
}
```

- [ ] **Step 7: Create handleSessionsTab()**

Replace `handleActiveTab` and `handleResumeTab` with a single handler:

```go
func (p *Plugin) handleSessionsTab(msg tea.KeyMsg) plugin.Action {
    switch msg.String() {
    case "up", "k":
        if p.unified != nil {
            p.unified.MoveUp()
        }
        return plugin.NoopAction()
    case "down", "j":
        if p.unified != nil {
            p.unified.MoveDown()
        }
        return plugin.NoopAction()

    case "a":
        // Toggle archive mode
        if p.unified != nil {
            p.unified.ToggleArchive()
        }
        return plugin.NoopAction()

    case "enter":
        if p.unified == nil {
            return plugin.NoopAction()
        }
        sel := p.unified.SelectedItem()
        if sel == nil {
            return plugin.NoopAction()
        }
        dir := sel.Project
        if sel.WorktreePath != "" {
            dir = sel.WorktreePath
        }
        return plugin.Action{
            Type: plugin.ActionLaunch,
            Args: map[string]string{
                "dir":       dir,
                "resume_id": sel.SessionID,
            },
        }

    case "b":
        if p.unified == nil {
            return plugin.NoopAction()
        }
        sel := p.unified.SelectedItem()
        if sel == nil {
            return plugin.NoopAction()
        }
        if sel.Tier == TierLive || sel.Tier == TierArchived {
            if p.db != nil {
                bk := db.Session{
                    SessionID:    sel.SessionID,
                    Project:      sel.Project,
                    Repo:         sel.Repo,
                    Branch:       sel.Branch,
                    Created:      parseTime(sel.RegisteredAt),
                    Summary:      sel.Topic,
                    WorktreePath: sel.WorktreePath,
                }
                label := sel.Topic
                if label == "" {
                    label = sel.Branch
                }
                _ = db.DBInsertBookmark(p.db, bk, label)
                // If promoting from archive, remove from archive table
                if sel.Tier == TierArchived {
                    _ = db.DBDeleteArchivedSession(p.db, sel.SessionID)
                    p.unified.ReloadArchived()
                }
                // Reload saved sessions
                sessions, _ := db.DBLoadBookmarks(p.db)
                p.unified.SetSavedSessions(sessions)
            }
            p.flashMessage = "Bookmarked: " + sel.SessionID[:min(8, len(sel.SessionID))]
            p.flashMessageAt = time.Now()
        }
        return plugin.ConsumedAction()

    case "d":
        if p.unified == nil {
            return plugin.NoopAction()
        }
        sel := p.unified.SelectedItem()
        if sel == nil {
            return plugin.NoopAction()
        }
        switch sel.Tier {
        case TierLive:
            if sel.State == "active" || sel.State == "running" {
                p.flashMessage = "Can't dismiss running session"
                p.flashMessageAt = time.Now()
                return plugin.ConsumedAction()
            }
            // Archive ended live session via daemon, then remove from view
            if p.unified.daemonClient != nil {
                client := p.unified.daemonClient()
                if client != nil {
                    _ = client.ArchiveSession(daemon.ArchiveSessionParams{SessionID: sel.SessionID})
                }
            }
            p.unified.RemoveSession(sel.SessionID)
            p.flashMessage = "Dismissed: " + sel.SessionID[:min(8, len(sel.SessionID))]
        case TierSaved:
            if p.db != nil {
                _ = db.DBRemoveBookmark(p.db, sel.SessionID)
                sessions, _ := db.DBLoadBookmarks(p.db)
                p.unified.SetSavedSessions(sessions)
            }
            p.flashMessage = "Removed bookmark"
        case TierArchived:
            if p.db != nil {
                _ = db.DBDeleteArchivedSession(p.db, sel.SessionID)
                p.unified.ReloadArchived()
            }
            p.unified.RemoveSession(sel.SessionID)
            p.flashMessage = "Deleted archived session"
        }
        p.flashMessageAt = time.Now()
        return plugin.ConsumedAction()
    }
    return plugin.NoopAction()
}
```

- [ ] **Step 8: Update View()**

Replace the `"active"` and `"resume"` cases in `View()`:

```go
switch p.subTab {
case "sessions":
    content = p.viewSessionsTab()
case "new":
    content = p.viewNewTab()
case "worktrees":
    content = p.viewWorktreesTab()
}
```

Add the new view method:

```go
func (p *Plugin) viewSessionsTab() string {
    if p.unified == nil {
        return p.styles.hint.Render("  Daemon not connected.")
    }
    listView := p.unified.View(p.width, p.height)
    hints := p.renderHints()
    if p.flashMessage != "" && time.Since(p.flashMessageAt) < 3*time.Second {
        flash := p.styles.sectionHeader.Render("  " + p.flashMessage)
        return lipgloss.JoinVertical(lipgloss.Left, listView, "", flash, "", hints)
    }
    return lipgloss.JoinVertical(lipgloss.Left, listView, "", hints)
}
```

- [ ] **Step 9: Update HandleMessage()**

Remove the `sessionsLoadedMsg` handler (bookmarks are now loaded directly into unified). Remove the `resumeList` references from `WindowSizeMsg` and the delegate update in `View()`.

In `HandleMessage`, remove:
```go
case sessionsLoadedMsg:
    p.loading = false
    p.resumeList.SetItems(buildSessionItems(msg.sessions))
    return true, plugin.NoopAction()
```

In `WindowSizeMsg` handler, remove:
```go
p.resumeList.SetSize(listWidth, listHeight)
```

In `View()`, remove:
```go
p.resumeList.SetDelegate(itemDelegate{frame: frame, styles: &p.styles, grad: &p.grad})
```

Remove the `"resume"` case from the delegate in `HandleMessage`:
```go
case "resume":
    var cmd tea.Cmd
    p.resumeList, cmd = p.resumeList.Update(msg)
```

- [ ] **Step 10: Update KeyBindings()**

```go
func (p *Plugin) KeyBindings() []plugin.KeyBinding {
    return []plugin.KeyBinding{
        {Key: "s", Description: "Sessions sub-tab", Promoted: true},
        {Key: "n", Description: "New session sub-tab", Promoted: true},
        {Key: "t", Description: "Worktrees sub-tab", Promoted: true},
        {Key: "a", Description: "Toggle archived sessions", Promoted: true},
        {Key: "w", Description: "Launch in worktree", Promoted: true},
        {Key: "enter", Description: "Launch/resume session", Promoted: true},
        {Key: "b", Description: "Bookmark session", Promoted: true},
        {Key: "d", Description: "Dismiss/delete session", Promoted: true},
        {Key: "shift+up/down", Description: "Reorder paths", Promoted: true},
        {Key: "delete", Description: "Remove saved path", Promoted: true},
        {Key: "esc", Description: "Quit or cancel"},
    }
}
```

- [ ] **Step 11: Update renderHints()**

Replace the `"active"` and `"resume"` cases:

```go
case "sessions":
    if p.unified != nil && p.unified.archiveMode {
        hints = p.styles.hint.Render("enter resume   b save   d delete   j/k navigate   a back   s sessions   n new   t worktrees")
    } else {
        hints = p.styles.hint.Render("enter resume   b bookmark   d dismiss   j/k navigate   a archive   s sessions   n new   t worktrees")
    }
```

Update `"worktrees"` and `"new"` hints to use `s` instead of `a active` and `r resume`.

- [ ] **Step 12: Update default subTab in Init**

Change the initial subTab assignment. Find where `p.subTab` is initialized (if it defaults to "new" or "active") and set it to `"sessions"`:

```go
p.subTab = "sessions"
```

- [ ] **Step 13: Wire daemon client to unified view**

Find where `p.active.daemonClient` is set (likely in a `SetDaemonClient` method or similar wiring in the host) and update it to `p.unified.daemonClient`. Search for references to `p.active` that wire the daemon client and update them.

- [ ] **Step 14: Delete old files**

```bash
rm internal/builtin/sessions/active.go internal/builtin/sessions/active_test.go
```

- [ ] **Step 15: Remove dead code**

Remove from `sessions.go`:
- `handleActiveTab()` method
- `handleResumeTab()` method
- `viewActiveTab()` method
- `viewResumeTab()` method
- `sessionsLoadedMsg` type (if no longer used)
- `sessionItem` type and `buildSessionItems()` helper (if only used by resumeList)
- `loadSessionsCmd()` method (if no longer used)
- Any import of `list` from bubbletea if no longer needed (newList still uses it)

- [ ] **Step 16: Run all tests**

Run: `go test ./internal/builtin/sessions/ -v`
Expected: All tests PASS. Some old tests may need updating (see Task 5).

- [ ] **Step 17: Commit**

```bash
git add -A internal/builtin/sessions/ internal/db/
git commit -m "Replace active + resume tabs with unified sessions view"
```

---

### Task 5: Update Existing Tests

Old tests in `sessions_test.go` reference `p.subTab = "active"`, `p.subTab = "resume"`, `p.active`, `p.resumeList`, etc. These need updating.

**Files:**
- Modify: `internal/builtin/sessions/sessions_test.go`

- [ ] **Step 1: Find all references to old sub-tabs**

Search `sessions_test.go` for `"active"`, `"resume"`, `p.active`, `p.resumeList`. Replace:
- `p.subTab = "active"` → `p.subTab = "sessions"`
- `p.subTab = "resume"` → `p.subTab = "sessions"` (with appropriate bookmarks loaded into unified)
- `p.active.sessions = ...` → `p.unified.liveSessions = ...`
- `p.resumeList` references → `p.unified.savedSessions` or `p.unified.SetSavedSessions(...)`
- Key `"a"` for switching tabs → `"s"` for switching to sessions
- Key `"r"` for switching tabs → removed (no longer exists)

- [ ] **Step 2: Update `setupActivePlugin` helper in unified_test.go**

The old `setupActivePlugin` from `active_test.go` was deleted. If any integration tests need a plugin wired to the sessions tab with injected data, create:

```go
func setupSessionsPlugin(t *testing.T, live []daemon.SessionInfo, saved []db.Session) *Plugin {
    t.Helper()
    t.Setenv("CCC_CONFIG_DIR", t.TempDir())
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
    p.HandleMessage(tea.WindowSizeMsg{Width: 120, Height: 40})
    p.subTab = "sessions"
    p.unified.liveSessions = live
    p.unified.SetSavedSessions(saved)
    return p
}
```

- [ ] **Step 3: Port key integration tests**

Port the critical tests from the deleted `active_test.go` to use the new helper:
- `TestActiveEnterProducesLaunchAction` → `TestSessionsEnterLaunchesLive`
- `TestActiveBookmarkSavesToDB` → `TestSessionsBookmarkSavesToDB`
- `TestActiveDismissEndedSession` → `TestSessionsDismissEndedSession`
- `TestActiveDismissActiveSessionBlocked` → `TestSessionsDismissRunningBlocked`

These tests should use `handleSessionsTab` via `p.HandleKey(...)` with `p.subTab = "sessions"`.

- [ ] **Step 4: Add archive toggle test**

```go
func TestSessionsArchiveToggle(t *testing.T) {
    p := setupSessionsPlugin(t, nil, nil)
    p.unified.archivedSessions = []db.ArchivedSession{
        {SessionID: "s1", Topic: "Old", Project: "/proj", RegisteredAt: time.Now().Format(time.RFC3339), EndedAt: time.Now().Format(time.RFC3339)},
    }

    // Press 'a' to toggle archive mode
    action := p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
    if action.Type != plugin.ActionNoop {
        t.Fatalf("expected noop, got %s", action.Type)
    }
    if !p.unified.archiveMode {
        t.Fatal("expected archive mode after pressing 'a'")
    }

    // Press 'a' again to go back
    p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
    if p.unified.archiveMode {
        t.Fatal("expected main mode after second 'a'")
    }
}
```

- [ ] **Step 5: Run all tests**

Run: `go test ./internal/builtin/sessions/ -v`
Expected: All tests PASS.

- [ ] **Step 6: Run full test suite**

Run: `go test ./... 2>&1 | tail -20`
Expected: All tests PASS. Check for compilation errors from other packages referencing removed types.

- [ ] **Step 7: Commit**

```bash
git add internal/builtin/sessions/
git commit -m "Update sessions tests for unified view"
```

---

### Task 6: Update Spec

**Files:**
- Modify: `specs/builtin/sessions.md` (if it exists) or create it

- [ ] **Step 1: Check if sessions spec exists**

Run: `ls specs/builtin/sessions.md 2>/dev/null || echo "no spec"`

- [ ] **Step 2: Update or create the spec**

Update the sessions spec to reflect:
- Three sub-tabs: sessions (default), new, worktrees
- Sessions tab has two modes: main (Live + Saved) and archive
- `a` toggles archive mode, `s`/`n`/`t` switch sub-tabs
- Auto-archiving behavior on session end
- `b` bookmarks (Live→Saved, Archived→Saved), `d` dismisses/deletes
- `cc_archived_sessions` table

- [ ] **Step 3: Run build to verify everything compiles**

Run: `make build`
Expected: Successful build.

- [ ] **Step 4: Commit**

```bash
git add specs/
git commit -m "Update sessions spec for unified view with archive mode"
```

---

### Future: Type-to-Filter in Unified View

The design spec mentions `/` for type-to-filter across both sections. The old resume tab had this via bubbletea's `list.Model` built-in filtering. The unified view uses custom rendering (not `list.Model`), so type-to-filter requires adding a `filterText` field to `unifiedView` and filtering `displayItems()` by topic/project/branch substring match. This is a follow-up task — the core merge and archive toggle work without it.
