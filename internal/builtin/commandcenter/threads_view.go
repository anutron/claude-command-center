package commandcenter

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// renderThreadsView renders the full Threads tab content.
func renderThreadsView(s *ccStyles, g *gradientColors, cc *db.CommandCenter, width, height, cursor, frame int) string {
	if cc == nil {
		return lipgloss.PlaceHorizontal(width, lipgloss.Center,
			s.DescMuted.Render("No data yet."))
	}

	active := cc.ActiveThreads()
	paused := cc.PausedThreads()

	var sections []string

	activeSection := renderThreadSection(s, g, "ACTIVE THREADS", active, cursor, 0, width, frame)
	sections = append(sections, activeSection)

	pausedSection := renderThreadSection(s, g, "PAUSED THREADS", paused, cursor, len(active), width, frame)
	sections = append(sections, pausedSection)

	footer := renderThreadsFooter(s, width)
	sections = append(sections, footer)

	return strings.Join(sections, "\n\n")
}

func renderThreadSection(s *ccStyles, g *gradientColors, title string, threads []db.Thread, cursor int, cursorOffset int, width, frame int) string {
	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	var lines []string
	if len(threads) == 0 {
		lines = append(lines, s.DescMuted.Render("  (none)"))
	}
	for i, t := range threads {
		globalIdx := cursorOffset + i
		selected := globalIdx == cursor

		pointer := "  "
		if selected {
			pointer = ui.PulsingPointerStyle(g, frame).Render("> ")
		}

		prefix := threadTypePrefix(s, t.Type)
		titleStr := t.Title
		if t.Repo != "" {
			titleStr += " (" + t.Repo + ")"
		}

		var age string
		if t.PausedAt != nil && t.Status == "paused" {
			age = "paused " + t.PausedAt.Format("Mon Jan 2")
		} else {
			age = db.RelativeTime(t.CreatedAt)
		}

		summary := t.Summary

		var styledTitle, styledSummary, styledAge string
		if t.Status == "paused" {
			styledTitle = s.ThreadPaused.Render(titleStr)
			styledSummary = s.ThreadPaused.Render(summary)
			styledAge = s.ThreadPaused.Render(age)
		} else {
			styledTitle = s.ThreadActive.Render(titleStr)
			styledSummary = s.DescMuted.Render(summary)
			styledAge = s.DescMuted.Render(age)
		}

		var parts []string
		if prefix != "" {
			parts = append(parts, prefix)
		}
		parts = append(parts, styledTitle)
		if summary != "" {
			parts = append(parts, styledSummary)
		}
		parts = append(parts, styledAge)

		line := pointer + strings.Join(parts, "  ")

		if selected {
			line = s.SelectedItem.Render(line)
		}

		lines = append(lines, line)
	}

	body := strings.Join(lines, "\n")

	headerLine := fmt.Sprintf(" %s ", s.TitleBoldC.Render(title))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.ColorMuted).
		Padding(0, 1).
		Width(innerWidth).
		Render(body)

	boxLines := strings.Split(box, "\n")
	if len(boxLines) > 0 {
		topBorder := boxLines[0]
		if len(topBorder) > 3 {
			boxLines[0] = topBorder[:2] + headerLine + topBorder[2+lipgloss.Width(headerLine):]
		}
	}

	return strings.Join(boxLines, "\n")
}

func renderThreadsFooter(s *ccStyles, width int) string {
	hints := s.DescMuted.Render("  \u2191\u2193 navigate \u00b7 enter open \u00b7 p pause \u00b7 s start \u00b7 x close \u00b7 a add")
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, hints)
}

func threadTypePrefix(s *ccStyles, t string) string {
	switch t {
	case "pr":
		return lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("PR")
	case "email":
		return lipgloss.NewStyle().Foreground(s.ColorYellow).Bold(true).Render("Email")
	case "slack":
		return lipgloss.NewStyle().Foreground(s.ColorPurple).Bold(true).Render("Slack")
	case "project":
		return lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render("Project")
	case "manual":
		return ""
	default:
		return ""
	}
}
