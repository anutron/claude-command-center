package sessions

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/lipgloss"
)

// Tier constants for UnifiedItem.
const (
	TierLive     = "live"
	TierSaved    = "saved"
	TierArchived = "archived"
)

// ViewFilter constants control which tiers mainItems returns.
const (
	ViewFilterAll      = "" // show both live + saved (legacy)
	ViewFilterLiveOnly = "live_only"
	ViewFilterSavedOnly = "saved_only"
)

// UnifiedItem is a flattened, renderable session from any tier. Exported so
// the Plugin can use it for key handling without knowing which underlying list
// was queried.
type UnifiedItem struct {
	SessionID    string
	Topic        string
	Project      string
	Repo         string
	Branch       string
	WorktreePath string
	RegisteredAt string
	EndedAt      string

	// State holds the daemon state string for live items (e.g. "active", "ended").
	State string

	// Tier is TierLive, TierSaved, or TierArchived.
	Tier string

	// IsBookmarked is true when this is a live session that is also in the
	// saved (bookmark) list.
	IsBookmarked bool

	// IsFirst is true for the first item in its section, used by the renderer
	// to decide when to emit a section header.
	IsFirst bool
}

// unifiedView combines live sessions (from daemon), saved sessions (bookmarks /
// winddowns stored in DB), and archived sessions into a single scrollable list
// with two modes: main (live + saved) and archive.
type unifiedView struct {
	liveSessions     []daemon.SessionInfo
	savedSessions    []db.Session
	archivedSessions []db.ArchivedSession
	agentsByID       map[string]daemon.AgentStatusResult
	cursor           int
	archiveMode      bool
	viewFilter       string // ViewFilterAll, ViewFilterLiveOnly, or ViewFilterSavedOnly
	daemonClient     func() *daemon.Client
	styles           sessionStyles
	db               *sql.DB
}

// NewUnifiedView creates a new unified view. clientFn may be nil when the
// daemon is not available.
func NewUnifiedView(clientFn func() *daemon.Client, styles sessionStyles) *unifiedView {
	return &unifiedView{
		daemonClient: clientFn,
		styles:       styles,
	}
}

// ToggleArchive flips between main mode (live + saved) and archive mode.
// The cursor is always reset to 0 on toggle.
func (uv *unifiedView) ToggleArchive() {
	uv.archiveMode = !uv.archiveMode
	uv.cursor = 0
}

// displayItems delegates to mainItems or archivedItems based on mode.
func (uv *unifiedView) displayItems() []UnifiedItem {
	if uv.archiveMode {
		return uv.archivedItems()
	}
	return uv.mainItems()
}

// mainItems builds the combined live + saved item list.
//
// Live section: all live sessions, sorted by recency (most recent first).
// Saved section: saved sessions whose session ID is NOT already in the live
//
//	section (deduplication). Live sessions that are also bookmarked get
//	IsBookmarked=true.
func (uv *unifiedView) mainItems() []UnifiedItem {
	// Build a set of live session IDs for deduplication and bookmark detection.
	liveIDs := make(map[string]bool, len(uv.liveSessions))
	for _, s := range uv.liveSessions {
		liveIDs[s.SessionID] = true
	}

	// Build a set of saved session IDs for bookmark detection.
	savedIDs := make(map[string]bool, len(uv.savedSessions))
	for _, s := range uv.savedSessions {
		if s.SessionID != "" {
			savedIDs[s.SessionID] = true
		}
	}

	// Sort live sessions by recency (most recent RegisteredAt first).
	sorted := make([]daemon.SessionInfo, len(uv.liveSessions))
	copy(sorted, uv.liveSessions)
	sort.Slice(sorted, func(i, j int) bool {
		ti := parseTime(sorted[i].RegisteredAt)
		tj := parseTime(sorted[j].RegisteredAt)
		return ti.After(tj)
	})

	var items []UnifiedItem

	// Live section (skipped when filter is saved-only).
	if uv.viewFilter != ViewFilterSavedOnly {
		for i, s := range sorted {
			items = append(items, UnifiedItem{
				SessionID:    s.SessionID,
				Topic:        s.Topic,
				Project:      s.Project,
				Repo:         s.Repo,
				Branch:       s.Branch,
				WorktreePath: s.WorktreePath,
				RegisteredAt: s.RegisteredAt,
				EndedAt:      s.EndedAt,
				State:        s.State,
				Tier:         TierLive,
				IsBookmarked: savedIDs[s.SessionID],
				IsFirst:      i == 0,
			})
		}
	}

	// Saved section: exclude any session already shown in live (skipped when filter is live-only).
	if uv.viewFilter != ViewFilterLiveOnly {
		savedIdx := 0
		for _, s := range uv.savedSessions {
			if s.SessionID != "" && liveIDs[s.SessionID] {
				// Already shown as live — skip.
				continue
			}
			items = append(items, UnifiedItem{
				SessionID:    s.SessionID,
				Topic:        s.Summary,
				Project:      s.Project,
				Repo:         s.Repo,
				Branch:       s.Branch,
				WorktreePath: s.WorktreePath,
				RegisteredAt: s.Created.Format("2006-01-02T15:04:05Z07:00"),
				Tier:         TierSaved,
				IsFirst:      savedIdx == 0,
			})
			savedIdx++
		}
	}

	return items
}

// archivedItems returns archived sessions as UnifiedItems.
func (uv *unifiedView) archivedItems() []UnifiedItem {
	items := make([]UnifiedItem, 0, len(uv.archivedSessions))
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
		switch uv.viewFilter {
		case ViewFilterSavedOnly:
			return uv.styles.hint.Render("  No saved sessions. Bookmark a session with 'b' to save it.")
		case ViewFilterLiveOnly:
			return uv.styles.hint.Render("  No active sessions. Start a Claude session to see it here.")
		default:
			return uv.styles.hint.Render("  No sessions. Start a Claude session to see it here.")
		}
	}

	var lines []string
	prevTier := ""

	for i, item := range items {
		// Section header on tier change.
		if item.Tier != prevTier {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			var header string
			switch item.Tier {
			case TierLive:
				header = uv.styles.sectionHeader.Render("  LIVE")
			case TierSaved:
				header = uv.styles.sectionHeader.Render("  SAVED")
			case TierArchived:
				header = uv.styles.sectionHeader.Render("  ARCHIVED")
			}
			lines = append(lines, header)
			prevTier = item.Tier
		}

		// Pointer.
		pointer := "  "
		if i == uv.cursor {
			pointer = "> "
		}

		// Status indicator.
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

		// Label (topic / summary / branch fallback).
		label := item.Topic
		if label == "" {
			if item.Branch != "" {
				label = item.Branch
			} else {
				label = filepath.Base(item.Project)
			}
		}

		// Bookmark annotation for live sessions that are also saved.
		if item.IsBookmarked {
			label += " ★"
		}

		// Label style.
		labelStyle := uv.styles.titleBoldW
		if i == uv.cursor {
			labelStyle = uv.styles.selectedItem
		}

		// Suffix.
		var suffix string
		switch item.Tier {
		case TierLive:
			age := sessionAge(item.RegisteredAt)
			suffix = uv.styles.descMuted.Render(age)
			if (item.State == "running" || item.State == "active") && uv.isSessionBlocked(item.SessionID) {
				blocked := lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")).Render("Blocked")
				suffix = blocked + "  " + suffix
			}
		case TierSaved:
			proj := filepath.Base(item.Project)
			var sfxText string
			if item.Branch != "" {
				sfxText = fmt.Sprintf("%s (%s)", proj, item.Branch)
			} else {
				sfxText = proj
			}
			suffix = uv.styles.descMuted.Render(sfxText)
		case TierArchived:
			age := sessionAge(item.EndedAt)
			suffix = uv.styles.descMuted.Render(age)
		}

		line := fmt.Sprintf("%s%s %s  %s",
			pointer,
			indicator,
			labelStyle.Render(label),
			suffix,
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

// SelectedItem returns the UnifiedItem at the current cursor position, or nil
// if the list is empty.
func (uv *unifiedView) SelectedItem() *UnifiedItem {
	items := uv.displayItems()
	if len(items) == 0 || uv.cursor >= len(items) {
		return nil
	}
	item := items[uv.cursor]
	return &item
}

// RemoveSession removes a session from whichever list it belongs to, then
// clamps the cursor.
func (uv *unifiedView) RemoveSession(sessionID string) {
	for i, s := range uv.liveSessions {
		if s.SessionID == sessionID {
			uv.liveSessions = append(uv.liveSessions[:i], uv.liveSessions[i+1:]...)
			uv.clampCursor()
			return
		}
	}
	for i, s := range uv.savedSessions {
		if s.SessionID == sessionID {
			uv.savedSessions = append(uv.savedSessions[:i], uv.savedSessions[i+1:]...)
			uv.clampCursor()
			return
		}
	}
	for i, s := range uv.archivedSessions {
		if s.SessionID == sessionID {
			uv.archivedSessions = append(uv.archivedSessions[:i], uv.archivedSessions[i+1:]...)
			uv.clampCursor()
			return
		}
	}
}

// clampCursor ensures the cursor stays within bounds of the current item list.
func (uv *unifiedView) clampCursor() {
	total := len(uv.displayItems())
	if uv.cursor >= total && uv.cursor > 0 {
		uv.cursor = total - 1
	}
}

// sessionsRefreshMsg carries fetched session data from a background goroutine
// back to the main bubbletea loop for safe state mutation.
type sessionsRefreshMsg struct {
	liveSessions     []daemon.SessionInfo
	savedSessions    []db.Session
	archivedSessions []db.ArchivedSession
	agentsByID       map[string]daemon.AgentStatusResult
}

// archiveNewlyEndedSessions compares previous live sessions with fresh ones and
// archives any that transitioned to ended and aren't bookmarked. Safe to call
// from background goroutines — takes all needed state as parameters.
func archiveNewlyEndedSessions(database *sql.DB, prevLive, newSessions []daemon.SessionInfo) {
	if database == nil {
		return
	}

	// Build set of previously running session IDs
	prevRunning := make(map[string]daemon.SessionInfo, len(prevLive))
	for _, s := range prevLive {
		if s.State == "running" || s.State == "active" {
			prevRunning[s.SessionID] = s
		}
	}

	// Build set of bookmarked IDs
	bookmarks, _ := db.DBLoadBookmarks(database)
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

		_ = db.DBInsertArchivedSession(database, db.ArchivedSession{
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

// Refresh fetches the latest session and agent data from the daemon.
func (uv *unifiedView) Refresh() {
	if uv.daemonClient == nil {
		return
	}
	client := uv.daemonClient()
	if client == nil {
		return
	}

	sessions, err := client.ListSessions()
	if err == nil {
		// Auto-archive newly ended sessions before updating the live list
		archiveNewlyEndedSessions(uv.db, uv.liveSessions, sessions)
		uv.liveSessions = sessions
	}
	uv.clampCursor()

	// Fetch agent statuses to detect blocked sessions.
	uv.agentsByID = nil
	agents, err := client.ListAgents()
	if err == nil && len(agents) > 0 {
		uv.agentsByID = make(map[string]daemon.AgentStatusResult, len(agents))
		for _, a := range agents {
			uv.agentsByID[a.ID] = a
		}
	}
}

// SetSavedSessions replaces the saved (bookmark) session list.
func (uv *unifiedView) SetSavedSessions(sessions []db.Session) {
	uv.savedSessions = sessions
}

// SetArchivedSessions replaces the archived session list.
func (uv *unifiedView) SetArchivedSessions(sessions []db.ArchivedSession) {
	uv.archivedSessions = sessions
}

// ReloadArchived loads archived sessions from the database.
func (uv *unifiedView) ReloadArchived() {
	if uv.db == nil {
		return
	}
	sessions, _ := db.DBLoadArchivedSessions(uv.db)
	uv.archivedSessions = sessions
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
