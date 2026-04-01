package tui

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// consoleOverlay manages the agent history overlay state.
type consoleOverlay struct {
	visible bool
	entries []db.AgentHistoryEntry
	cursor  int
	detail  bool // true = detail view for selected entry
	scroll  int  // scroll offset in detail view
}

// toggle flips the overlay visibility, loading entries and resetting state.
func (o *consoleOverlay) toggle(entries []db.AgentHistoryEntry) {
	o.visible = !o.visible
	if o.visible {
		o.entries = entries
		o.cursor = 0
		o.detail = false
		o.scroll = 0
	}
}

// close hides the overlay and resets detail/scroll state.
func (o *consoleOverlay) close() {
	o.visible = false
	o.detail = false
	o.scroll = 0
}

// selected returns the entry at the current cursor position, or nil.
func (o *consoleOverlay) selected() *db.AgentHistoryEntry {
	if len(o.entries) == 0 || o.cursor < 0 || o.cursor >= len(o.entries) {
		return nil
	}
	return &o.entries[o.cursor]
}

// detailLineCount returns the number of content lines in the detail view.
func (o *consoleOverlay) detailLineCount() int {
	// 3 header lines: title, subtitle, blank
	const headerLines = 3
	e := o.selected()
	if e == nil {
		return headerLines + 1 // "No entry selected"
	}
	// 16 data fields
	return headerLines + 16
}

// maxDetailScroll returns the maximum scroll offset for the detail view
// given the terminal height. Returns 0 if all content fits.
func (o *consoleOverlay) maxDetailScroll(termHeight int) int {
	const boxChrome = 4 // border top/bottom + padding top/bottom
	visibleRows := termHeight - boxChrome
	if visibleRows < 1 {
		visibleRows = 1
	}
	max := o.detailLineCount() - visibleRows
	if max < 0 {
		return 0
	}
	return max
}

const (
	overlayBoxWidth  = 70
	overlayMinWidth  = 40
	overlayTitle     = "AGENT CONSOLE"
	overlaySubtitle  = "Last 24 hours · ↑↓ select · Enter detail · ~ dismiss"
	overlaySubDetail = "Esc back · ↑↓ scroll · Shift+X kill"
	overlayEmpty     = "No agents in the last 24 hours"
	overlayBorderColor = "#3b4261"
)

// boxWidth returns the actual box width given the terminal width.
func (o *consoleOverlay) boxWidth(termWidth int) int {
	w := overlayBoxWidth
	if termWidth > 0 && termWidth-4 < w {
		w = termWidth - 4
		if w < overlayMinWidth {
			w = overlayMinWidth
		}
	}
	return w
}

// truncate truncates a string to n runes, appending "…" if needed.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}

// renderList renders the list view of agent history entries.
func (o *consoleOverlay) renderList(width, height int) string {
	bw := o.boxWidth(width)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(overlayBorderColor)).
		Padding(1, 2).
		Width(bw)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#c0caf5"))

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89"))

	// inner width = box width - padding*2 - border*2
	innerWidth := bw - 4 - 2 // padding(2*2)=4, border(2*1)=2

	var lines []string
	lines = append(lines, titleStyle.Render(overlayTitle))
	lines = append(lines, subtitleStyle.Render(overlaySubtitle))
	lines = append(lines, "")

	if len(o.entries) == 0 {
		lines = append(lines, subtitleStyle.Render(overlayEmpty))
	} else {
		// Column widths: icon(1) + space(1) + origin(35) + space(2) + elapsed(8) + space(2) + cost(rest)
		const iconW = 1
		const originW = 35
		const elapsedW = 8
		const gapW = 2
		costW := innerWidth - iconW - gapW - originW - gapW - elapsedW - gapW
		if costW < 6 {
			costW = 6
		}

		for i, e := range o.entries {
			icon := ui.AgentStatusIcon(e.Status)
			color := ui.AgentStatusColor(e.Status)

			iconStyled := lipgloss.NewStyle().Foreground(color).Render(icon)
			origin := truncate(e.OriginLabel, originW)
			elapsed := ui.FormatAgentElapsed(e)
			cost := fmt.Sprintf("$%.4f", e.CostUSD)

			// Pad/truncate fields
			originPadded := fmt.Sprintf("%-*s", originW, origin)
			elapsedPadded := fmt.Sprintf("%*s", elapsedW, elapsed)
			costPadded := truncate(cost, costW)

			row := fmt.Sprintf("%s %s  %s  %s",
				iconStyled,
				originPadded,
				elapsedPadded,
				costPadded,
			)

			if i == o.cursor {
				row = lipgloss.NewStyle().
					Background(lipgloss.Color("#292e42")).
					Foreground(lipgloss.Color("#c0caf5")).
					Width(innerWidth).
					Render(row)
			}

			lines = append(lines, row)
		}
	}

	content := strings.Join(lines, "\n")
	box := borderStyle.Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// renderDetail renders the detail view for the selected entry.
func (o *consoleOverlay) renderDetail(width, height int) string {
	bw := o.boxWidth(width)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(overlayBorderColor)).
		Padding(1, 2).
		Width(bw)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#c0caf5"))

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89"))

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7aa2f7"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0caf5"))

	e := o.selected()

	var lines []string
	lines = append(lines, titleStyle.Render(overlayTitle))
	lines = append(lines, subtitleStyle.Render(overlaySubDetail))
	lines = append(lines, "")

	if e == nil {
		lines = append(lines, subtitleStyle.Render("No entry selected"))
	} else {
		color := ui.AgentStatusColor(e.Status)
		icon := ui.AgentStatusIcon(e.Status)
		statusStyled := lipgloss.NewStyle().Foreground(color).Render(icon + " " + e.Status)

		field := func(label, value string) string {
			return labelStyle.Render(fmt.Sprintf("%-14s", label+":")) + " " + valueStyle.Render(value)
		}

		lines = append(lines, field("Status", statusStyled))
		lines = append(lines, field("Origin", e.OriginLabel))
		lines = append(lines, field("Origin Ref", e.OriginRef))
		lines = append(lines, field("Agent ID", e.AgentID))
		lines = append(lines, field("Session ID", e.SessionID))
		lines = append(lines, field("Automation", e.Automation))

		startedStr := "—"
		if !e.StartedAt.IsZero() {
			startedStr = e.StartedAt.Format("2006-01-02 15:04:05")
		}
		lines = append(lines, field("Started", startedStr))

		finishedStr := "—"
		if e.FinishedAt != nil {
			finishedStr = e.FinishedAt.Format("2006-01-02 15:04:05")
		}
		lines = append(lines, field("Finished", finishedStr))

		lines = append(lines, field("Duration", ui.FormatAgentElapsed(*e)))
		lines = append(lines, field("Cost", fmt.Sprintf("$%.6f", e.CostUSD)))
		lines = append(lines, field("Tokens In", fmt.Sprintf("%d", e.InputTokens)))
		lines = append(lines, field("Tokens Out", fmt.Sprintf("%d", e.OutputTokens)))

		exitStr := "—"
		if e.ExitCode != nil {
			exitStr = fmt.Sprintf("%d", *e.ExitCode)
		}
		lines = append(lines, field("Exit Code", exitStr))
		lines = append(lines, field("Project", e.ProjectDir))
		lines = append(lines, field("Repo", e.Repo))
		lines = append(lines, field("Branch", e.Branch))
	}

	// Calculate visible rows inside the box.
	// borderStyle adds: border (1 top + 1 bottom) + padding (1 top + 1 bottom) = 4 rows of chrome.
	const boxChrome = 4
	visibleRows := height - boxChrome
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Clamp scroll so content can't scroll past the last row.
	maxScroll := len(lines) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if o.scroll > maxScroll {
		o.scroll = maxScroll
	}

	// Apply scroll offset
	visibleLines := lines
	if o.scroll > 0 && o.scroll < len(lines) {
		visibleLines = lines[o.scroll:]
	}

	content := strings.Join(visibleLines, "\n")
	box := borderStyle.Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// render dispatches to renderList or renderDetail based on current state.
func (o *consoleOverlay) render(width, height int) string {
	if o.detail {
		return o.renderDetail(width, height)
	}
	return o.renderList(width, height)
}
