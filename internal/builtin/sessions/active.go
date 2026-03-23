package sessions

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/charmbracelet/lipgloss"
)

// activeView manages the "Active Sessions" sub-view, showing live sessions
// from the daemon's session registry.
type activeView struct {
	sessions     []daemon.SessionInfo
	cursor       int
	daemonClient func() *daemon.Client // nil-safe getter
	styles       sessionStyles
}

// NewActiveView creates a new active sessions view. The clientFn argument is a
// function that returns the current daemon RPC client (or nil if disconnected).
func NewActiveView(clientFn func() *daemon.Client, styles sessionStyles) *activeView {
	return &activeView{
		daemonClient: clientFn,
		styles:       styles,
	}
}

// Refresh fetches the latest session list from the daemon.
func (av *activeView) Refresh() {
	if av.daemonClient == nil {
		return
	}
	client := av.daemonClient()
	if client == nil {
		return
	}
	sessions, err := client.ListSessions()
	if err != nil {
		return
	}
	av.sessions = sessions
	// Clamp cursor
	if av.cursor >= len(av.sessions) {
		av.cursor = max(0, len(av.sessions)-1)
	}
}

// displayItem is a flattened session for rendering, preserving the original
// session data along with its display position.
type displayItem struct {
	session daemon.SessionInfo
	isFirst bool   // first item in its project group
	project string // display name (basename of project path)
}

// displayItems returns sessions grouped by project and sorted by recency,
// with the most recently active project group first.
func (av *activeView) displayItems() []displayItem {
	if len(av.sessions) == 0 {
		return nil
	}

	// Group sessions by project
	groups := map[string][]daemon.SessionInfo{}
	groupOrder := map[string]time.Time{} // most recent time per group

	for _, s := range av.sessions {
		groups[s.Project] = append(groups[s.Project], s)
		t := parseTime(s.RegisteredAt)
		if existing, ok := groupOrder[s.Project]; !ok || t.After(existing) {
			groupOrder[s.Project] = t
		}
	}

	// Sort project keys by most recent session time (descending)
	projects := make([]string, 0, len(groups))
	for p := range groups {
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool {
		return groupOrder[projects[i]].After(groupOrder[projects[j]])
	})

	// Within each group, sort by recency (most recent first)
	var items []displayItem
	for _, proj := range projects {
		sessions := groups[proj]
		sort.Slice(sessions, func(i, j int) bool {
			ti := parseTime(sessions[i].RegisteredAt)
			tj := parseTime(sessions[j].RegisteredAt)
			return ti.After(tj)
		})

		projName := filepath.Base(proj)
		for i, s := range sessions {
			items = append(items, displayItem{
				session: s,
				isFirst: i == 0,
				project: projName,
			})
		}
	}

	return items
}

// View renders the active sessions view.
func (av *activeView) View(width, height int) string {
	items := av.displayItems()

	if len(items) == 0 {
		return av.styles.hint.Render("  No active sessions. Start a Claude session to see it here.")
	}

	var lines []string
	itemIdx := 0 // tracks which item maps to the cursor
	for _, item := range items {
		// Project group header
		if item.isFirst {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, av.styles.sectionHeader.Render("  "+item.project))
		}

		// Status indicator
		var indicator string
		if item.session.State == "running" {
			indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Render("●")
		} else {
			indicator = av.styles.descMuted.Render("○")
		}

		// Topic or fallback
		label := item.session.Topic
		if label == "" {
			if item.session.Branch != "" {
				label = item.session.Branch
			} else {
				label = filepath.Base(item.session.Project)
			}
		}

		// Age
		age := sessionAge(item.session.RegisteredAt)

		// Build the line
		pointer := "  "
		if itemIdx == av.cursor {
			pointer = "> "
		}

		labelStyle := av.styles.titleBoldW
		if itemIdx == av.cursor {
			labelStyle = av.styles.selectedItem
		}

		line := fmt.Sprintf("%s%s %s  %s",
			pointer,
			indicator,
			labelStyle.Render(label),
			av.styles.descMuted.Render(age),
		)
		lines = append(lines, line)
		itemIdx++
	}

	return strings.Join(lines, "\n")
}

// MoveDown moves the cursor down, wrapping around.
func (av *activeView) MoveDown() {
	total := len(av.sessions)
	if total == 0 {
		return
	}
	av.cursor++
	if av.cursor >= total {
		av.cursor = 0
	}
}

// MoveUp moves the cursor up, wrapping around.
func (av *activeView) MoveUp() {
	total := len(av.sessions)
	if total == 0 {
		return
	}
	av.cursor--
	if av.cursor < 0 {
		av.cursor = total - 1
	}
}

// SelectedSession returns the currently selected session, or nil if empty.
func (av *activeView) SelectedSession() *daemon.SessionInfo {
	items := av.displayItems()
	if len(items) == 0 || av.cursor >= len(items) {
		return nil
	}
	s := items[av.cursor].session
	return &s
}

// parseTime parses an RFC3339 time string, returning zero time on failure.
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// sessionAge returns a human-readable age string from an RFC3339 timestamp.
func sessionAge(registered string) string {
	t := parseTime(registered)
	if t.IsZero() {
		return ""
	}
	return timeAgo(t)
}
