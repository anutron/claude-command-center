package settings

import (
	"fmt"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/lipgloss"
)

// prListItem represents one removable item in the PRs settings pane.
type prListItem struct {
	label string // display text
	repo  string // non-empty for ignored repos
	prID  string // non-empty for ignored PRs
}

// loadPRListItems loads ignored repos and PRs into a flat list for the cursor.
func (p *Plugin) loadPRListItems() []prListItem {
	var items []prListItem

	repos, _ := db.DBLoadIgnoredRepos(p.database)
	for _, r := range repos {
		items = append(items, prListItem{
			label: r,
			repo:  r,
		})
	}

	ignoredPRs, _ := db.DBLoadIgnoredPRs(p.database)
	for _, pr := range ignoredPRs {
		items = append(items, prListItem{
			label: fmt.Sprintf("%s  %s", pr.ID, pr.Title),
			prID:  pr.ID,
		})
	}

	return items
}

// viewPRSettingsContent renders the interactive PRs settings pane.
func (p *Plugin) viewPRSettingsContent(width, height int) string {
	repos, _ := db.DBLoadIgnoredRepos(p.database)
	ignoredPRs, _ := db.DBLoadIgnoredPRs(p.database)
	totalItems := len(repos) + len(ignoredPRs)

	var lines []string

	// --- Ignored Repos section ---
	lines = append(lines, p.styles.categoryHeader.Render("  IGNORED REPOS"))
	if len(repos) == 0 {
		lines = append(lines, "  "+p.styles.muted.Render("No repos ignored. Press I on a PR to ignore its repo."))
	} else {
		lines = append(lines, "  "+p.styles.muted.Render("PRs from these repos are hidden from all tabs:"))
		for i, r := range repos {
			cursor := "  "
			if p.focusZone == FocusForm && i == p.prCursor {
				cursor = p.styles.pointer.Render("> ")
			}
			label := r
			hint := p.styles.muted.Render("  [x remove]")
			if p.focusZone == FocusForm && i == p.prCursor {
				label = p.styles.navSelected.Render(r)
			}
			lines = append(lines, fmt.Sprintf("  %s%s%s", cursor, label, hint))
		}
	}

	lines = append(lines, "")

	// --- Ignored PRs section ---
	lines = append(lines, p.styles.categoryHeader.Render("  IGNORED PRS"))
	if len(ignoredPRs) == 0 {
		lines = append(lines, "  "+p.styles.muted.Render("No PRs ignored. Press i on a PR to ignore it."))
	} else {
		lines = append(lines, "  "+p.styles.muted.Render("These individual PRs are hidden (auto-cleaned when closed):"))
		repoOffset := len(repos) // PRs come after repos in the flat list
		for i, pr := range ignoredPRs {
			idx := repoOffset + i
			cursor := "  "
			if p.focusZone == FocusForm && idx == p.prCursor {
				cursor = p.styles.pointer.Render("> ")
			}
			id := pr.ID
			title := p.styles.muted.Render("  " + pr.Title)
			hint := p.styles.muted.Render("  [x restore]")
			if p.focusZone == FocusForm && idx == p.prCursor {
				id = p.styles.navSelected.Render(pr.ID)
			}
			lines = append(lines, fmt.Sprintf("  %s%s%s%s", cursor, id, title, hint))
		}
	}

	// Help line when focused
	if p.focusZone == FocusForm && totalItems > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+p.styles.muted.Render("j/k navigate  x/delete remove  esc back"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// handlePRSettingsKey processes key events in the PRs settings pane.
func (p *Plugin) handlePRSettingsKey(msg string) {
	items := p.loadPRListItems()
	if len(items) == 0 {
		return
	}

	// Clamp cursor
	if p.prCursor >= len(items) {
		p.prCursor = len(items) - 1
	}
	if p.prCursor < 0 {
		p.prCursor = 0
	}

	switch msg {
	case "up", "k":
		if p.prCursor > 0 {
			p.prCursor--
		}
	case "down", "j":
		if p.prCursor < len(items)-1 {
			p.prCursor++
		}
	case "x", "delete", "backspace":
		if p.prCursor >= 0 && p.prCursor < len(items) {
			item := items[p.prCursor]
			if item.repo != "" {
				if err := db.DBRemoveIgnoredRepo(p.database, item.repo); err == nil {
					p.flashMessage = item.repo + " restored"
				} else {
					p.flashMessage = "Failed to remove: " + err.Error()
				}
			} else if item.prID != "" {
				if err := db.DBSetPRIgnored(p.database, item.prID, false); err == nil {
					p.flashMessage = "PR restored"
				} else {
					p.flashMessage = "Failed to restore: " + err.Error()
				}
			}
			p.flashMessageAt = currentTime()
			// Re-check items and adjust cursor
			newItems := p.loadPRListItems()
			if p.prCursor >= len(newItems) {
				p.prCursor = len(newItems) - 1
			}
			if p.prCursor < 0 {
				p.prCursor = 0
			}
		}
	}
}
