package prs

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// View renders the plugin's current view.
func (p *Plugin) View(width, height, frame int) string {
	p.width = width
	p.height = height
	p.frame = frame

	viewWidth := ui.ContentMaxWidth
	if width > 0 && width < viewWidth {
		viewWidth = width
	}

	tabBar := p.renderTabBar(viewWidth)
	filtered := p.filteredPRs(p.activeTab)

	var listView string
	if len(filtered) == 0 {
		cat := categoryOrder[p.activeTab]
		msg := categoryEmptyMessage[cat]
		listView = "\n" + p.styles.Hint.Render("  "+msg) + "\n"
	} else {
		listView = p.renderPRList(filtered, viewWidth)
	}

	hints := p.renderHints()

	// Clear stale flash messages
	if p.flashMessage != "" && time.Since(p.flashMessageAt) > 5*time.Second {
		p.flashMessage = ""
	}
	if p.flashMessage != "" {
		flash := p.styles.Hint.Render("  > " + p.flashMessage)
		hints = flash + "\n" + hints
	}

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, listView, hints)
}

// renderTabBar renders the sub-tab header with counts.
func (p *Plugin) renderTabBar(width int) string {
	counts := p.categoryCounts()
	var parts []string

	for i, cat := range categoryOrder {
		name := categoryDisplayName[cat]
		label := fmt.Sprintf("[%d] %s (%d)", i+1, name, counts[i])
		if i == p.activeTab {
			parts = append(parts, p.styles.ActiveTab.Render(label))
		} else {
			parts = append(parts, p.styles.InactiveTab.Render(label))
		}
	}

	return "  " + strings.Join(parts, "  ") + "\n"
}

// renderPRList renders the PR list for the active tab.
func (p *Plugin) renderPRList(prs []db.PullRequest, width int) string {
	var lines []string
	cursor := p.cursors[p.activeTab]

	for i, pr := range prs {
		pointer := "  "
		if i == cursor {
			pointer = ui.PulsingPointerStyle(&p.grad, p.frame).Render("> ")
		}

		line := p.renderPRRow(pr, width-4)

		if i == cursor {
			line = p.styles.SelectedItem.Render(line)
		}

		lines = append(lines, pointer+line)
	}

	return strings.Join(lines, "\n")
}

// renderPRRow renders a single PR row with contextual columns based on the active tab.
func (p *Plugin) renderPRRow(pr db.PullRequest, maxWidth int) string {
	cat := categoryOrder[p.activeTab]

	// Common: repo#number + title
	repoNum := p.styles.DescMuted.Render(fmt.Sprintf("%s#%d", pr.Repo, pr.Number))
	title := p.styles.TitleBoldW.Render(pr.Title)

	var detail string
	switch cat {
	case CategoryWaiting:
		detail = p.renderWaitingDetail(pr)
	case CategoryRespond:
		detail = p.renderRespondDetail(pr)
	case CategoryReview:
		detail = p.renderReviewDetail(pr)
	case CategoryStale:
		detail = p.renderStaleDetail(pr)
	}

	line := repoNum + " " + title
	if detail != "" {
		line += "  " + detail
	}

	if flag := p.renderAgentStatus(pr); flag != "" {
		line += "  " + flag
	}

	return truncate(line, maxWidth)
}

// renderWaitingDetail shows reviewer statuses, CI status, and age since last activity.
func (p *Plugin) renderWaitingDetail(pr db.PullRequest) string {
	var parts []string

	// Reviewer statuses
	if len(pr.ReviewerLogins) > 0 {
		var reviewers []string
		pending := make(map[string]bool)
		for _, login := range pr.PendingReviewerLogins {
			pending[login] = true
		}
		for _, login := range pr.ReviewerLogins {
			if pending[login] {
				reviewers = append(reviewers, p.rowStyle.pending.Render(login+" \u23f3"))
			} else {
				reviewers = append(reviewers, p.rowStyle.success.Render(login+" \u2713"))
			}
		}
		parts = append(parts, strings.Join(reviewers, " "))
	}

	// CI status
	parts = append(parts, p.renderCIStatus(pr.CIStatus))

	// Age
	parts = append(parts, p.styles.DescMuted.Render(formatAge(pr.LastActivityAt)))

	return strings.Join(parts, "  ")
}

// renderRespondDetail shows unresolved threads, review decision, and who requested changes.
func (p *Plugin) renderRespondDetail(pr db.PullRequest) string {
	var parts []string

	if pr.UnresolvedThreadCount > 0 {
		parts = append(parts, p.rowStyle.pending.Render(fmt.Sprintf("%d threads", pr.UnresolvedThreadCount)))
	}

	if pr.ReviewDecision != "" {
		parts = append(parts, p.renderReviewDecision(pr.ReviewDecision))
	}

	// Show who is waiting (pending reviewers who already reviewed = changes requested)
	if pr.ReviewDecision == "CHANGES_REQUESTED" && len(pr.ReviewerLogins) > 0 {
		parts = append(parts, p.styles.DescMuted.Render("from "+strings.Join(pr.ReviewerLogins, ", ")))
	}

	return strings.Join(parts, "  ")
}

// renderReviewDetail shows PR author, title info, and age since review requested.
func (p *Plugin) renderReviewDetail(pr db.PullRequest) string {
	var parts []string

	parts = append(parts, p.styles.BranchYellow.Render("@"+pr.Author))
	parts = append(parts, p.styles.DescMuted.Render(formatAge(pr.LastActivityAt)))

	if pr.Draft {
		parts = append(parts, p.rowStyle.draft.Render("draft"))
	}

	return strings.Join(parts, "  ")
}

// renderStaleDetail shows days since last activity and draft status.
func (p *Plugin) renderStaleDetail(pr db.PullRequest) string {
	var parts []string

	parts = append(parts, p.styles.DescMuted.Render(formatAge(pr.LastActivityAt)))

	if pr.Draft {
		parts = append(parts, p.rowStyle.draft.Render("draft"))
	}

	if pr.CIStatus != "" {
		parts = append(parts, p.renderCIStatus(pr.CIStatus))
	}

	return strings.Join(parts, "  ")
}

// renderCIStatus renders a CI status indicator.
func (p *Plugin) renderCIStatus(status string) string {
	switch status {
	case "success":
		return p.rowStyle.success.Render("CI \u2713")
	case "failure":
		return p.rowStyle.failure.Render("CI \u2717")
	case "pending":
		return p.rowStyle.pending.Render("CI \u23f3")
	default:
		return ""
	}
}

// renderReviewDecision renders a review decision badge.
func (p *Plugin) renderReviewDecision(decision string) string {
	switch decision {
	case "APPROVED":
		return p.rowStyle.success.Render("approved")
	case "CHANGES_REQUESTED":
		return p.rowStyle.failure.Render("changes requested")
	case "REVIEW_REQUIRED":
		return p.rowStyle.pending.Render("review required")
	default:
		return p.styles.DescMuted.Render(decision)
	}
}

// renderAgentStatus returns a styled status indicator for agent-processed PRs.
func (p *Plugin) renderAgentStatus(pr db.PullRequest) string {
	switch pr.AgentStatus {
	case "pending":
		return p.rowStyle.pending.Render("⏳ queued")
	case "running":
		return p.rowStyle.pending.Render("⏳ running")
	case "completed":
		return p.rowStyle.success.Render("✓ ready")
	case "failed":
		return p.rowStyle.failure.Render("✗ failed")
	default:
		if (pr.Category == "review" || pr.Category == "respond") && p.resolveRepoDir(pr.Repo) == "" {
			return p.rowStyle.failure.Render("⚠ no repo")
		}
		return ""
	}
}

// renderHints renders the bottom hint line.
func (p *Plugin) renderHints() string {
	hints := p.styles.Hint.Render("1-4 tab  j/k nav  enter review/respond  o open  w watch  i ignore  r refresh")
	return "\n" + lipgloss.PlaceHorizontal(ui.ContentMaxWidth, lipgloss.Center, hints)
}

// formatAge returns a compact relative time string (e.g. "2d", "1w", "3h").
func formatAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		m := int(d.Minutes())
		if m <= 1 {
			return "now"
		}
		return fmt.Sprintf("%dm", m)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
}

// truncate truncates a string to maxWidth, appending "..." if truncated.
func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	if ansi.StringWidth(s) > maxWidth {
		return ansi.Truncate(s, maxWidth-1, "...")
	}
	return s
}
