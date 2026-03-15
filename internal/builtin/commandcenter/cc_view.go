package commandcenter

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// formatDuration renders a time.Duration as a compact string like "30m", "1h", "1h30m".
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// shortDirName returns just the final directory name from an absolute path.
// If the path is empty or filepath.Base returns ".", it returns the original path.
func shortDirName(path string) string {
	if path == "" {
		return path
	}
	base := filepath.Base(path)
	if base == "." {
		return path
	}
	return base
}

// renderCommandCenterView is the main entry point for the command center tab.
func renderCommandCenterView(s *ccStyles, g *gradientColors, cc *db.CommandCenter, calendars []config.CalendarEntry, calendarEnabled bool, width, height, todoCursor, scrollOffset, frame int, loadingTodoID string, showBacklog bool, refreshing bool, lastRefreshError string, filteredTodos []db.Todo, triageCounts map[string]int) string {
	if cc == nil {
		empty := lipgloss.NewStyle().
			Foreground(s.ColorMuted).
			Width(width).
			Align(lipgloss.Center).
			Render("No data yet. Run refresh or wait for next refresh.")
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, empty)
	}

	warningBanner := renderWarningBanner(s, cc.Warnings, width)

	usedHeight := 2
	if warningBanner != "" {
		usedHeight += lipgloss.Height(warningBanner) + 1
	}
	suggestion := renderSuggestionBanner(s, &cc.Suggestions, width)
	if suggestion != "" {
		usedHeight += lipgloss.Height(suggestion) + 1
	}
	usedHeight += 2

	panelHeight := height - usedHeight
	if panelHeight < 10 {
		panelHeight = 10
	}

	var completed []db.Todo
	if showBacklog {
		completed = cc.CompletedTodos()
	}

	var columns string
	if calendarEnabled {
		colWidth := width/2 - 2
		maxVisibleTodos := (panelHeight - 3) / 2
		if maxVisibleTodos < 5 {
			maxVisibleTodos = 5
		}
		calCol := renderCalendarColumn(s, calendars, &cc.Calendar, colWidth, panelHeight)
		todoCol := renderTodoPanel(s, g, filteredTodos, completed, todoCursor, scrollOffset, maxVisibleTodos, colWidth, frame, loadingTodoID, triageCounts)
		calPanel := s.PanelBorder.Width(colWidth).Render(calCol)
		todoPanel := s.PanelBorder.Width(colWidth).Render(todoCol)
		columns = lipgloss.JoinHorizontal(lipgloss.Top, calPanel, " ", todoPanel)
	} else {
		// Calendar disabled: full-width todos with hint
		todoWidth := width - 4
		maxVisibleTodos := (panelHeight - 3) / 2
		if maxVisibleTodos < 5 {
			maxVisibleTodos = 5
		}
		todoCol := renderTodoPanel(s, g, filteredTodos, completed, todoCursor, scrollOffset, maxVisibleTodos, todoWidth, frame, loadingTodoID, triageCounts)
		hint := s.CalendarFree.Render("  Configure calendar in Settings to see your schedule here")
		todoContent := lipgloss.JoinVertical(lipgloss.Left, todoCol, "", hint)
		columns = s.PanelBorder.Width(todoWidth).Render(todoContent)
	}

	footer := renderCCFooter(s, cc.GeneratedAt, width, refreshing, frame, lastRefreshError)

	var parts []string
	if warningBanner != "" {
		parts = append(parts, warningBanner, "")
	}
	parts = append(parts, columns)
	if suggestion != "" {
		parts = append(parts, "", suggestion)
	}
	parts = append(parts, "", footer)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderCalendarColumn renders today (and optionally tomorrow) sections.
func renderCalendarColumn(s *ccStyles, calendars []config.CalendarEntry, cal *db.CalendarData, width, maxHeight int) string {
	now := time.Now()
	afternoon := now.Hour() >= 12

	todayEvents := visibleEvents(cal.Today)
	if afternoon {
		todayEvents = upcomingEvents(todayEvents, now)
	}

	todayLabel := fmt.Sprintf("TODAY (%s)", strings.ToUpper(now.Format("Mon Jan 2")))

	parts := []string{}
	usedLines := 2

	if afternoon {
		todayMax := (maxHeight - usedLines - 2) * 3 / 5
		tomorrowMax := maxHeight - usedLines - todayMax - 2
		if todayMax < 3 {
			todayMax = 3
		}
		if tomorrowMax < 3 {
			tomorrowMax = 3
		}

		todaySection := renderCalendarPanelCapped(s, calendars, todayEvents, todayLabel, width, todayMax)
		parts = append(parts, todaySection)

		tomorrow := now.AddDate(0, 0, 1)
		tomorrowLabel := fmt.Sprintf("TOMORROW (%s)", strings.ToUpper(tomorrow.Format("Mon Jan 2")))
		tomorrowSection := renderCalendarPanelCapped(s, calendars, visibleEvents(cal.Tomorrow), tomorrowLabel, width, tomorrowMax)
		parts = append(parts, "", tomorrowSection)
	} else {
		calMax := maxHeight - usedLines
		if calMax < 5 {
			calMax = 5
		}
		todaySection := renderCalendarPanelCapped(s, calendars, todayEvents, todayLabel, width, calMax)
		parts = append(parts, todaySection)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func visibleEvents(events []db.CalendarEvent) []db.CalendarEvent {
	var out []db.CalendarEvent
	for _, ev := range events {
		if ev.Declined {
			continue
		}
		if strings.TrimSpace(ev.Title) == "" {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// upcomingEvents filters to events that haven't ended yet (for afternoon view).
func upcomingEvents(events []db.CalendarEvent, now time.Time) []db.CalendarEvent {
	var out []db.CalendarEvent
	for _, ev := range events {
		if ev.End.After(now) {
			out = append(out, ev)
		}
	}
	return out
}

func renderCalendarPanelCapped(s *ccStyles, calendars []config.CalendarEntry, events []db.CalendarEvent, label string, width, maxLines int) string {
	availableForEvents := maxLines - 1
	if availableForEvents < 1 {
		availableForEvents = 1
	}
	if len(events) > availableForEvents {
		events = events[:availableForEvents]
	}
	return renderCalendarPanel(s, calendars, events, label, width)
}

type conflictPos int

const (
	posNone conflictPos = iota
	posFirst
	posMiddle
	posLast
)

func computeConflictPositions(events []db.CalendarEvent) ([]conflictPos, []time.Time) {
	n := len(events)
	pos := make([]conflictPos, n)
	groupEnd := make([]time.Time, n)

	i := 0
	for i < n {
		if events[i].AllDay {
			i++
			continue
		}
		maxEnd := events[i].End
		j := i + 1
		for j < n && !events[j].AllDay && events[j].Start.Before(maxEnd) {
			if events[j].End.After(maxEnd) {
				maxEnd = events[j].End
			}
			j++
		}
		if j-i > 1 {
			pos[i] = posFirst
			for k := i + 1; k < j-1; k++ {
				pos[k] = posMiddle
			}
			pos[j-1] = posLast
			for k := i; k < j; k++ {
				groupEnd[k] = maxEnd
			}
		}
		i = j
	}

	return pos, groupEnd
}

// defaultCalendarColors is a palette for calendars without a configured color.
var defaultCalendarColors = []string{
	"#7aa2f7", // blue
	"#9ece6a", // green
	"#bb9af7", // purple
	"#e0af68", // yellow
	"#7dcfff", // cyan
	"#ff9e64", // orange
}

// calendarColor returns the color for a calendar ID, falling back to a default palette.
func calendarColor(calendars []config.CalendarEntry, calendarID string, idx int) string {
	for _, c := range calendars {
		if c.ID == calendarID && c.Color != "" {
			return c.Color
		}
	}
	if idx >= 0 {
		return defaultCalendarColors[idx%len(defaultCalendarColors)]
	}
	return ""
}

// calendarIDIndex returns a stable index for a calendar ID based on config order.
func calendarIDIndex(calendars []config.CalendarEntry, calendarID string) int {
	for i, c := range calendars {
		if c.ID == calendarID {
			return i
		}
	}
	return -1
}

func renderCalendarPanel(s *ccStyles, calendars []config.CalendarEntry, events []db.CalendarEvent, label string, width int) string {
	var lines []string
	lines = append(lines, s.SectionHeader.Render(label))

	if len(events) == 0 {
		lines = append(lines, s.CalendarFree.Render("  No events"))
		return strings.Join(lines, "\n")
	}

	now := time.Now()
	positions, groupEnds := computeConflictPositions(events)

	// Fixed-width time column: 8 chars for time like "12:00pm" + 1 space
	const timeColWidth = 9
	// Duration column: 6 chars like " 1h30m"
	const durColWidth = 6

	var maxEndSoFar time.Time
	for i, ev := range events {
		isPast := ev.End.Before(now)

		// Free-gap marker: show "---- free ----" when >30min gap between events
		if i > 0 && !maxEndSoFar.IsZero() && !ev.AllDay {
			gap := ev.Start.Sub(maxEndSoFar)
			if gap > 30*time.Minute {
				freeTime := s.CalendarTime.Render(fmt.Sprintf("%-*s", timeColWidth, maxEndSoFar.Format("3:04pm")))
				freeLine := fmt.Sprintf("  %s%s", freeTime, s.CalendarFree.Render(fmt.Sprintf("---- %s free ----", formatDuration(gap))))
				lines = append(lines, freeLine)
			}
		}

		isConflict := positions[i] != posNone

		var connector string
		switch positions[i] {
		case posFirst:
			connector = s.DueOverdue.Render("\u256d")
		case posMiddle, posLast:
			connector = s.DueOverdue.Render("\u2502")
		default:
			connector = " "
		}

		// All-day events: show "all day" instead of time/duration
		if ev.AllDay {
			titleMaxWidth := width - 2 - timeColWidth - 2
			if titleMaxWidth < 10 {
				titleMaxWidth = 10
			}
			title := ev.Title
			if len(title) > titleMaxWidth && titleMaxWidth > 0 {
				title = title[:titleMaxWidth-1] + "~"
			}
			titlePadded := title
			if len(title) < titleMaxWidth {
				titlePadded = title + strings.Repeat(" ", titleMaxWidth-len(title))
			}

			timeStr := s.CalendarFree.Render(fmt.Sprintf("%-*s", timeColWidth, "all day"))
			calIdx := calendarIDIndex(calendars, ev.CalendarID)
			color := calendarColor(calendars, ev.CalendarID, calIdx)
			if color != "" {
				titleStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(titlePadded)
				line := fmt.Sprintf("%s %s%s", connector, timeStr, titleStyled)
				lines = append(lines, line)
			} else {
				line := fmt.Sprintf("%s %s%s", connector, timeStr, titlePadded)
				lines = append(lines, line)
			}
			continue
		}

		timeFmt := ev.Start.Format("3:04pm")
		dur := ev.End.Sub(ev.Start)
		durFmt := formatDuration(dur)

		// Calculate title space: total width - connector(2) - time(timeColWidth) - dur(durColWidth) - spacing(2)
		titleMaxWidth := width - 2 - timeColWidth - durColWidth - 2
		if titleMaxWidth < 10 {
			titleMaxWidth = 10
		}

		title := ev.Title
		if len(title) > titleMaxWidth && titleMaxWidth > 0 {
			title = title[:titleMaxWidth-1] + "~"
		}

		// Right-pad title to fill the column
		titlePadded := title
		if len(title) < titleMaxWidth {
			titlePadded = title + strings.Repeat(" ", titleMaxWidth-len(title))
		}

		// Right-align duration
		durPadded := fmt.Sprintf("%*s", durColWidth, durFmt)

		// Apply styling based on state
		if isPast {
			// Past events are dimmed
			timeStr := s.CalendarPast.Render(fmt.Sprintf("%-*s", timeColWidth, timeFmt))
			titleStyled := s.CalendarPast.Render(titlePadded)
			durStr := s.CalendarPast.Render(durPadded)
			line := fmt.Sprintf("%s %s%s %s", connector, timeStr, titleStyled, durStr)
			lines = append(lines, line)
		} else if isConflict {
			timeStr := s.DueOverdue.Render(fmt.Sprintf("%-*s", timeColWidth, timeFmt))
			titleStyled := s.DueOverdue.Render(titlePadded)
			durStr := s.DueOverdue.Render(durPadded)
			line := fmt.Sprintf("%s %s%s %s", connector, timeStr, titleStyled, durStr)
			lines = append(lines, line)
		} else {
			timeStr := s.CalendarTime.Render(fmt.Sprintf("%-*s", timeColWidth, timeFmt))

			// Color the title by calendar
			calIdx := calendarIDIndex(calendars, ev.CalendarID)
			color := calendarColor(calendars, ev.CalendarID, calIdx)
			if color != "" {
				titleStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(titlePadded)
				durStr := s.CalendarTime.Render(durPadded)
				line := fmt.Sprintf("%s %s%s %s", connector, timeStr, titleStyled, durStr)
				lines = append(lines, line)
			} else {
				durStr := s.CalendarTime.Render(durPadded)
				line := fmt.Sprintf("%s %s%s %s", connector, timeStr, titlePadded, durStr)
				lines = append(lines, line)
			}
		}

		if positions[i] == posLast {
			endStr := groupEnds[i].Format("3:04pm")
			prefix := "\u2570\u2500\u2500\u2500 " + endStr + " "
			fillLen := width - len([]rune(prefix))
			if fillLen < 1 {
				fillLen = 1
			}
			closer := s.DueOverdue.Render(prefix + strings.Repeat("\u2500", fillLen))
			lines = append(lines, closer)
		}

		// Track the furthest end time for free-gap detection
		if !ev.AllDay && ev.End.After(maxEndSoFar) {
			maxEndSoFar = ev.End
		}
	}

	return strings.Join(lines, "\n")
}

func renderTodoPanel(s *ccStyles, g *gradientColors, todos []db.Todo, completed []db.Todo, cursor, scrollOffset, maxVisible, width int, frame int, loadingTodoID string, triageCounts map[string]int) string {
	var lines []string

	header := s.SectionHeader.Render(fmt.Sprintf("TODOS (%d active)", len(todos)))
	lines = append(lines, header)

	// Agent status header line
	agentHeader := renderAgentStatusHeader(s, todos)
	if agentHeader != "" {
		lines = append(lines, agentHeader)
	}

	lines = append(lines, "")

	if len(todos) == 0 {
		lines = append(lines, s.CalendarFree.Render("  No active todos"))
	}

	visStart := scrollOffset
	visEnd := scrollOffset + maxVisible
	if visStart < 0 {
		visStart = 0
	}
	if visEnd > len(todos) {
		visEnd = len(todos)
	}

	if visStart > 0 {
		lines = append(lines, s.CalendarTime.Render(fmt.Sprintf("  \u25b2 %d more above", visStart)))
	}

	titleMaxWidth := width - 8
	if titleMaxWidth < 20 {
		titleMaxWidth = 20
	}

	for i := visStart; i < visEnd; i++ {
		todo := todos[i]
		num := i + 1

		isLoading := loadingTodoID != "" && todo.ID == loadingTodoID

		title := truncateToWidth(flattenTitle(todo.Title), titleMaxWidth)
		numStr := fmt.Sprintf("%d", num)
		if isLoading {
			numStr = loadingSpinnerChar(frame)
		}
		var line1 string
		if i == cursor {
			pointer := ui.PulsingPointerStyle(g, frame).Render("> ")
			styledNum := lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(numStr + ". " + title)
			if isLoading {
				styledNum = lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render(numStr) +
					lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(". "+title)
			}
			line1 = pointer + styledNum
		} else {
			if isLoading {
				line1 = "  " + lipgloss.NewStyle().Foreground(s.ColorCyan).Render(numStr) + ". " + title
			} else {
				line1 = fmt.Sprintf("  %s. %s", numStr, title)
			}
		}
		lines = append(lines, line1)

		var details []string
		if indicator := agentStatusIndicator(s, todo.SessionStatus); indicator != "" {
			details = append(details, indicator)
		}
		if todo.Due != "" {
			urgency := db.DueUrgency(todo.Due)
			label := db.FormatDueLabel(todo.Due)
			details = append(details, s.DueStyle(urgency).Render(label))
		}
		if todo.WhoWaiting != "" {
			details = append(details, s.CalendarTime.Render(todo.WhoWaiting+" waiting"))
		} else {
			details = append(details, s.CalendarTime.Render("no blocker"))
		}
		if todo.Effort != "" {
			details = append(details, s.CalendarTime.Render("~"+todo.Effort))
		}
		if len(details) > 0 {
			detailStr := strings.Join(details, s.CalendarTime.Render(" \u00b7 "))
			lines = append(lines, "     "+detailStr)
		}
	}

	if visEnd < len(todos) {
		lines = append(lines, s.CalendarTime.Render(fmt.Sprintf("  \u25bc %d more below", len(todos)-visEnd)))
	}

	if len(completed) > 0 {
		lines = append(lines, "")
		lines = append(lines, s.CalendarTime.Render(fmt.Sprintf("  COMPLETED (%d)", len(completed))))
		for _, todo := range completed {
			title := s.CalendarFree.Render("  \u2713 " + todo.Title)
			lines = append(lines, title)
		}
	}

	// Triage status bar
	if triageCounts != nil {
		statusBar := renderTriageStatusBar(s, triageCounts, width)
		if statusBar != "" {
			lines = append(lines, "", statusBar)
		}
	}

	return strings.Join(lines, "\n")
}

// agentStatusIndicator returns a styled indicator string for a given session status.
func agentStatusIndicator(s *ccStyles, status string) string {
	switch status {
	case "active":
		return lipgloss.NewStyle().Foreground(s.ColorCyan).Render("● agent working")
	case "blocked":
		return lipgloss.NewStyle().Foreground(s.ColorYellow).Render("● needs input")
	case "review":
		return lipgloss.NewStyle().Foreground(s.ColorGreen).Render("● ready for review")
	case "queued":
		return lipgloss.NewStyle().Foreground(s.ColorMuted).Render("⏳ queued")
	default:
		return ""
	}
}

// renderAgentStatusHeader returns a summary line like "2/3 agents running, 1 queued".
func renderAgentStatusHeader(s *ccStyles, todos []db.Todo) string {
	var active, queued int
	for _, t := range todos {
		switch t.SessionStatus {
		case "active":
			active++
		case "queued":
			queued++
		}
	}
	if active == 0 && queued == 0 {
		return ""
	}
	parts := []string{}
	parts = append(parts, fmt.Sprintf("%d/3 agents running", active))
	if queued > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", queued))
	}
	return "  " + lipgloss.NewStyle().Foreground(s.ColorCyan).Render(strings.Join(parts, ", "))
}

func renderWarningBanner(s *ccStyles, warnings []db.Warning, width int) string {
	if len(warnings) == 0 {
		return ""
	}

	warningHeaderStyle := lipgloss.NewStyle().Foreground(s.ColorYellow).Bold(true)
	warningBorderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.ColorYellow)
	warningMsgStyle := lipgloss.NewStyle().Foreground(s.ColorYellow)

	header := warningHeaderStyle.Render(fmt.Sprintf("\u26a0 DATA SOURCE WARNINGS (%d)", len(warnings)))
	var wLines []string
	for _, w := range warnings {
		line := fmt.Sprintf("  %s: %s",
			warningMsgStyle.Bold(true).Render(w.Source),
			warningMsgStyle.Render(w.Message),
		)
		wLines = append(wLines, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, wLines...)...)
	return warningBorderStyle.Width(width - 2).Render(content)
}

func renderSuggestionBanner(s *ccStyles, suggestions *db.Suggestions, width int) string {
	if suggestions == nil || suggestions.Focus == "" {
		return ""
	}

	header := s.SectionHeader.Render("SUGGESTED FOCUS")
	body := s.Suggestion.Render(fmt.Sprintf("%q", suggestions.Focus))

	content := lipgloss.JoinVertical(lipgloss.Left, header, body)
	return s.PanelBorder.Width(width - 2).Render(content)
}

func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			if len(current)+1+len(word) > maxWidth {
				lines = append(lines, current)
				current = word
			} else {
				current += " " + word
			}
		}
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

func renderDetailView(s *ccStyles, todo db.Todo, detailMode string, selectedField int, fieldInputView string, commandInputView string, width int, notice string, noticeType string, statusCursor int, filteredPaths []string, pathCursor int, pathFilter string) string {
	innerWidth := width - 4
	if innerWidth < 40 {
		innerWidth = 40
	}

	title := s.SectionHeader.Render(fmt.Sprintf("TODO #%d", todo.DisplayID))
	todoTitle := lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(todo.Title)

	// Two-column layout for fields
	colWidth := (innerWidth - 6) / 2
	if colWidth < 20 {
		colWidth = 20
	}

	// Editable fields (left column): Status, Due, Project
	type fieldEntry struct {
		label string
		value string
		idx   int // field index for selection
	}
	editableFields := []fieldEntry{
		{"Status", todo.Status, 0},
		{"Due", "", 1},
		{"Project", shortDirName(todo.ProjectDir), 2},
	}
	// Format due with urgency
	if todo.Due != "" {
		urgency := db.DueUrgency(todo.Due)
		label := db.FormatDueLabel(todo.Due)
		editableFields[1].value = s.DueStyle(urgency).Render(todo.Due + " (" + label + ")")
	}

	// Read-only fields (right column)
	type roField struct {
		label string
		value string
	}
	var rightFields []roField
	if todo.Source != "" {
		rightFields = append(rightFields, roField{"Source", todo.Source})
	}
	if todo.Context != "" {
		rightFields = append(rightFields, roField{"Context", displayContext(todo.Context)})
	}
	if todo.WhoWaiting != "" {
		rightFields = append(rightFields, roField{"Who waiting", todo.WhoWaiting})
	}
	rightFields = append(rightFields, roField{"Created", todo.CreatedAt.Format("Jan 2, 2006")})

	// Build left column lines
	var leftLines []string
	for _, f := range editableFields {
		label := s.SectionHeader.Render(f.label + ":")
		val := f.value
		if val == "" {
			val = s.DescMuted.Render("—")
		}

		if detailMode == "selectingStatus" && f.idx == 0 {
			// Render inline status options with cursor
			var optParts []string
			for i, opt := range statusOptions {
				if i == statusCursor {
					optParts = append(optParts, lipgloss.NewStyle().
						Background(s.ColorCyan).
						Foreground(lipgloss.Color("#000000")).
						Bold(true).
						Padding(0, 1).
						Render(opt))
				} else {
					optParts = append(optParts, s.DescMuted.Render(opt))
				}
			}
			leftLines = append(leftLines, fmt.Sprintf("  %-14s %s", label, strings.Join(optParts, "  ")))
		} else if detailMode == "selectingPath" && f.idx == 2 {
			// Show path filter input
			filterDisplay := pathFilter
			if filterDisplay == "" {
				filterDisplay = s.DescMuted.Render("type to filter...")
			} else {
				filterDisplay = lipgloss.NewStyle().Foreground(s.ColorCyan).Render(filterDisplay)
			}
			leftLines = append(leftLines, fmt.Sprintf("  %-14s %s", label, filterDisplay))
		} else if detailMode == "editingField" && selectedField == f.idx {
			// Show input for the field being edited
			leftLines = append(leftLines, fmt.Sprintf("  %-14s %s", label, fieldInputView))
		} else if (detailMode == "viewing" || detailMode == "commandInput") && selectedField == f.idx {
			// Highlight selected field with brackets
			leftLines = append(leftLines, fmt.Sprintf("  %-14s [%s]", label, val))
		} else {
			leftLines = append(leftLines, fmt.Sprintf("  %-14s %s", label, val))
		}
	}

	// Build right column lines
	var rightLines []string
	for _, f := range rightFields {
		label := s.SectionHeader.Render(f.label + ":")
		val := f.value
		if val == "" {
			val = s.DescMuted.Render("—")
		}
		rightLines = append(rightLines, fmt.Sprintf("%-14s %s", label, val))
	}

	// Pad columns to same length
	for len(leftLines) < len(rightLines) {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < len(leftLines) {
		rightLines = append(rightLines, "")
	}

	// Join columns side by side
	var fieldRows []string
	for i := range leftLines {
		left := leftLines[i]
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		// Pad left column to fixed width
		leftRendered := left
		leftWidth := lipgloss.Width(leftRendered)
		if leftWidth < colWidth {
			leftRendered += strings.Repeat(" ", colWidth-leftWidth)
		}
		fieldRows = append(fieldRows, leftRendered+"  "+right)
	}
	fieldStr := strings.Join(fieldRows, "\n")

	// Path picker (shown below fields when in selectingPath mode)
	var pathPickerSection string
	if detailMode == "selectingPath" && len(filteredPaths) > 0 {
		maxVisible := 8
		startIdx := 0
		if pathCursor >= maxVisible {
			startIdx = pathCursor - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(filteredPaths) {
			endIdx = len(filteredPaths)
		}

		var pathLines []string
		for i := startIdx; i < endIdx; i++ {
			path := filteredPaths[i]
			// Show just the last 2 path components for brevity
			displayPath := path
			if len(displayPath) > innerWidth-8 {
				displayPath = "..." + displayPath[len(displayPath)-(innerWidth-11):]
			}
			if i == pathCursor {
				pathLines = append(pathLines, lipgloss.NewStyle().
					Background(s.ColorCyan).
					Foreground(lipgloss.Color("#000000")).
					Bold(true).
					Padding(0, 1).
					Render(displayPath))
			} else {
				pathLines = append(pathLines, "  "+s.DescMuted.Render(displayPath))
			}
		}

		if startIdx > 0 {
			pathLines = append([]string{s.CalendarTime.Render(fmt.Sprintf("  ▲ %d more", startIdx))}, pathLines...)
		}
		if endIdx < len(filteredPaths) {
			pathLines = append(pathLines, s.CalendarTime.Render(fmt.Sprintf("  ▼ %d more", len(filteredPaths)-endIdx)))
		}

		pickerHint := s.Hint.Render("  j/k navigate · type to filter · enter select · esc cancel")
		pathLines = append(pathLines, pickerHint)

		pathPickerSection = "\n" + lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(s.ColorCyan).
			Width(innerWidth - 4).
			Padding(0, 1).
			Render(strings.Join(pathLines, "\n"))
	} else if detailMode == "selectingPath" && len(filteredPaths) == 0 {
		pathPickerSection = "\n  " + s.DescMuted.Render("No paths match filter")
	}

	// Detail section
	var detailSection string
	if todo.Detail != "" {
		detailHeader := s.SectionHeader.Render("  DETAIL")
		wrapped := wrapText(todo.Detail, innerWidth-6)
		var detailLines []string
		for _, line := range strings.Split(wrapped, "\n") {
			detailLines = append(detailLines, "   "+line)
		}
		detailBody := lipgloss.NewStyle().Foreground(s.ColorWhite).Render(strings.Join(detailLines, "\n"))
		detailSection = lipgloss.JoinVertical(lipgloss.Left, "", detailHeader, "", detailBody)
	}

	// Session status indicator (for active/review sessions)
	var sessionSection string
	if todo.SessionStatus == "active" {
		sessionIndicator := lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("● Session: running")
		sessionSection = "\n  " + sessionIndicator
	} else if todo.SessionStatus == "review" || todo.SessionStatus == "failed" {
		statusLabel := "completed"
		statusColor := s.ColorGreen
		if todo.SessionStatus == "failed" {
			statusLabel = "failed"
			statusColor = s.ColorYellow
		}
		sessionIndicator := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render("● Session: " + statusLabel)
		sessionSection = "\n  " + sessionIndicator
	}

	// Session summary section (shown when agent has completed work)
	var summarySection string
	if todo.SessionSummary != "" {
		summaryHeader := s.SectionHeader.Render("  SESSION SUMMARY")
		wrapped := wrapText(todo.SessionSummary, innerWidth-6)
		var summaryLines []string
		for _, line := range strings.Split(wrapped, "\n") {
			summaryLines = append(summaryLines, "   "+line)
		}
		summaryBody := lipgloss.NewStyle().Foreground(s.ColorWhite).Render(strings.Join(summaryLines, "\n"))
		summarySection = lipgloss.JoinVertical(lipgloss.Left, "", summaryHeader, "", summaryBody)
	}

	// Prompt section (read-only, not editable — prompts are managed in the task runner)
	var promptSection string
	promptText := todo.ProposedPrompt
	if promptText != "" {
		promptHeader := s.SectionHeader.Render("  PROMPT")
		// Truncate to ~3 lines
		promptLines := strings.Split(promptText, "\n")
		if len(promptLines) > 3 {
			promptLines = promptLines[:3]
			promptLines = append(promptLines, "...")
		}
		var styledLines []string
		for _, line := range promptLines {
			truncated := truncateToWidth(line, innerWidth-6)
			styledLines = append(styledLines, "   "+truncated)
		}
		promptBody := lipgloss.NewStyle().Foreground(s.ColorWhite).Render(strings.Join(styledLines, "\n"))
		promptSection = lipgloss.JoinVertical(lipgloss.Left, "", promptHeader, "", promptBody)
	} else {
		promptSection = "\n  " + s.SectionHeader.Render("PROMPT") + "  " + s.DescMuted.Render("(no prompt set)")
	}

	// Command input section (when in commandInput mode)
	var commandSection string
	if detailMode == "commandInput" {
		divider := s.DescMuted.Render(strings.Repeat("\u2500", innerWidth-2))
		inputLabel := s.DescMuted.Render("Tell me what changed:")
		commandSection = lipgloss.JoinVertical(lipgloss.Left,
			"",
			"  "+divider,
			"  "+inputLabel,
			"  "+commandInputView,
		)
	}

	// Notice banner (shown after done/remove)
	var noticeBanner string
	if notice != "" {
		bgColor := s.ColorGreen
		icon := "\u2713"
		if noticeType == "removed" {
			bgColor = s.ColorYellow
			icon = "\u2717"
		}
		noticeBanner = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(bgColor).
			Bold(true).
			Padding(0, 1).
			Render(icon + " " + notice)
	}

	// Footer hints based on mode
	var hints string
	switch detailMode {
	case "viewing":
		hints = s.Hint.Render("j/k prev/next \u00b7 x done \u00b7 X remove \u00b7 tab cycle \u00b7 enter edit \u00b7 o launch \u00b7 c command \u00b7 esc back")
	case "editingField":
		hints = s.Hint.Render("enter confirm \u00b7 esc cancel")
	case "selectingStatus":
		hints = s.Hint.Render("\u2190/\u2192 select \u00b7 enter confirm \u00b7 esc cancel")
	case "selectingPath":
		hints = s.Hint.Render("j/k navigate \u00b7 type to filter \u00b7 enter select \u00b7 esc cancel")
	case "commandInput":
		hints = s.Hint.Render("enter submit to AI \u00b7 esc cancel")
	}

	parts := []string{
		"  " + title,
		"",
	}
	if noticeBanner != "" {
		parts = append(parts, "  "+noticeBanner, "")
	}
	parts = append(parts,
		"  "+todoTitle,
		"",
		fieldStr,
	)
	if pathPickerSection != "" {
		parts = append(parts, pathPickerSection)
	}
	if sessionSection != "" {
		parts = append(parts, sessionSection)
	}
	if summarySection != "" {
		parts = append(parts, summarySection)
	}
	if detailSection != "" {
		parts = append(parts, detailSection)
	}
	parts = append(parts, promptSection)
	if commandSection != "" {
		parts = append(parts, commandSection)
	}
	parts = append(parts, "", "  "+hints)

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return s.PanelBorder.Width(innerWidth).Render(content)
}

// refreshSpinner renders a small fixed-width (1 char) braille dot spinner with
// shifting colors. The dots cycle through braille patterns while the color
// smoothly rotates through a palette.
func refreshSpinner(frame int) string {
	// Braille dot patterns that create a rotating appearance
	patterns := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	// Muted color palette that shifts smoothly
	colors := []string{"#5f87af", "#5f87d7", "#5f5fd7", "#875fd7", "#875faf", "#5f5faf"}

	p := patterns[(frame/3)%len(patterns)]
	c := colors[(frame/5)%len(colors)]
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render(string(p))
}

func renderCCFooter(s *ccStyles, generatedAt time.Time, width int, refreshing bool, frame int, lastRefreshError string) string {
	var left string
	if refreshing {
		left = refreshSpinner(frame) + " "
	} else if lastRefreshError != "" {
		errMsg := lastRefreshError
		if len(errMsg) > 60 {
			errMsg = errMsg[:57] + "..."
		}
		left = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6666")).Render("refresh failed: " + errMsg)
	} else {
		left = s.RefreshInfo.Render("refreshed " + db.RelativeTime(generatedAt))
	}
	right := s.RefreshInfo.Render("\u2191\u2193 navigate \u00b7 enter detail \u00b7 space expand \u00b7 x done \u00b7 u undo \u00b7 t add \u00b7 c command \u00b7 ? help")

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		gap = 2
	}

	return left + strings.Repeat(" ", gap) + right
}

func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-1]) + "~"
}

// displayContext returns a compact display string for a todo's Context field.
// Slack URLs like "https://foo.slack.com/archives/C01ABC/p123..." become "Slack"
// or "Slack: #channel-name" if the channel name can be resolved (future).
// Other URLs are shortened to their hostname. Non-URL values pass through unchanged.
func displayContext(ctx string) string {
	if ctx == "" {
		return ""
	}
	// Detect Slack archive URLs: https://<workspace>.slack.com/archives/...
	if strings.Contains(ctx, ".slack.com/archives/") {
		return "Slack"
	}
	// Detect other Slack URLs
	if strings.Contains(ctx, ".slack.com/") {
		return "Slack"
	}
	// Detect GitHub URLs
	if strings.Contains(ctx, "github.com/") {
		return "GitHub"
	}
	// Detect Slack channel names: #channel-name – description text
	if strings.HasPrefix(ctx, "#") {
		channel := ctx
		for _, sep := range []string{" – ", " - ", " — "} {
			if idx := strings.Index(channel, sep); idx != -1 {
				channel = channel[:idx]
			}
		}
		if idx := strings.Index(channel, " "); idx != -1 {
			channel = channel[:idx]
		}
		ctx = "Slack: " + channel
	}
	// Truncate long strings to ~40 chars
	if len(ctx) > 40 {
		return ctx[:37] + "..."
	}
	return ctx
}

func flattenTitle(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func renderExpandedTodoItem(s *ccStyles, g *gradientColors, todo db.Todo, num int, isCursor bool, maxWidth int, frame int, isLoading bool) string {
	prefix := fmt.Sprintf("%d. ", num)
	prefixWidth := 2 + len(prefix)
	titleMax := maxWidth - prefixWidth
	if titleMax < 10 {
		titleMax = 10
	}

	title := flattenTitle(todo.Title)
	if title == "" {
		title = "(untitled)"
	}
	title = truncateToWidth(title, titleMax)

	numStr := fmt.Sprintf("%d", num)
	if isLoading {
		numStr = loadingSpinnerChar(frame)
	}
	var line1 string
	if isCursor {
		pointer := ui.PulsingPointerStyle(g, frame).Render("> ")
		if isLoading {
			line1 = pointer + lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render(numStr) +
				lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(". "+title)
		} else {
			line1 = pointer + lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(numStr+". "+title)
		}
	} else {
		if isLoading {
			line1 = "  " + lipgloss.NewStyle().Foreground(s.ColorCyan).Render(numStr) + ". " + title
		} else {
			line1 = "  " + numStr + ". " + title
		}
	}

	indent := strings.Repeat(" ", prefixWidth)
	detailMax := maxWidth - prefixWidth
	var detailParts []string
	if todo.Due != "" {
		detailParts = append(detailParts, db.FormatDueLabel(todo.Due))
	}
	if todo.WhoWaiting != "" {
		detailParts = append(detailParts, todo.WhoWaiting+" waiting")
	}
	if todo.Effort != "" {
		detailParts = append(detailParts, "~"+todo.Effort)
	}

	var line2 string
	if len(detailParts) > 0 {
		remaining := detailMax
		var styledParts []string
		for j, part := range detailParts {
			if remaining <= 0 {
				break
			}
			if j > 0 {
				remaining -= 3
			}
			display := truncateToWidth(part, remaining)
			remaining -= len([]rune(display))

			if j == 0 && todo.Due != "" {
				urgency := db.DueUrgency(todo.Due)
				styledParts = append(styledParts, s.DueStyle(urgency).Render(display))
			} else {
				styledParts = append(styledParts, s.CalendarTime.Render(display))
			}
		}
		line2 = indent + strings.Join(styledParts, s.CalendarTime.Render(" \u00b7 "))
	} else {
		line2 = " "
	}

	return line1 + "\n" + line2
}

func renderExpandedTodoView(s *ccStyles, g *gradientColors, todos []db.Todo, cursor, offset, rowsPerCol, numCols, width, height int, frame int, loadingTodoID string, refreshing bool, activeFilter string, counts map[string]int) string {
	tabBar := renderTriageTabBar(s, activeFilter, counts, width)

	pageSize := rowsPerCol * numCols
	totalPages := (len(todos) + pageSize - 1) / pageSize
	currentPage := offset/pageSize + 1

	header := s.SectionHeader.Render(fmt.Sprintf("TODOS (%d active)", len(todos)))
	hints := s.RefreshInfo.Render("tab filter \u00b7 y accept \u00b7 \u2191\u2193 navigate \u00b7 \u2190\u2192 columns/page \u00b7 space cycle/collapse \u00b7 enter detail \u00b7 x done \u00b7 ? help")

	if len(todos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, tabBar, "", s.CalendarFree.Render("  No active todos"), "", hints)
	}

	sepWidth := 3
	colWidth := (width - sepWidth*(numCols-1)) / numCols
	if colWidth < 30 {
		colWidth = 30
	}

	colStyle := lipgloss.NewStyle().Width(colWidth).MaxWidth(colWidth)
	colHeight := rowsPerCol * 2

	var columns []string
	for col := 0; col < numCols; col++ {
		startIdx := offset + col*rowsPerCol
		endIdx := startIdx + rowsPerCol
		if startIdx >= len(todos) {
			columns = append(columns, colStyle.Height(colHeight).Render(""))
			continue
		}
		if endIdx > len(todos) {
			endIdx = len(todos)
		}

		var items []string
		for i := startIdx; i < endIdx; i++ {
			isLoading := loadingTodoID != "" && todos[i].ID == loadingTodoID
			item := renderExpandedTodoItem(s, g, todos[i], i+1, i == cursor, colWidth, frame, isLoading)
			items = append(items, item)
		}

		colContent := strings.Join(items, "\n")
		columns = append(columns, colStyle.Height(colHeight).Render(colContent))
	}

	sep := s.CalendarTime.Render(" \u2502 ")
	joined := lipgloss.JoinHorizontal(lipgloss.Top, columns[0])
	for i := 1; i < len(columns); i++ {
		joined = lipgloss.JoinHorizontal(lipgloss.Top, joined, sep, columns[i])
	}

	// Footer: spinner (left) and page info (right-aligned)
	var footerLeft string
	if refreshing {
		footerLeft = refreshSpinner(frame)
	}
	pageInfo := s.RefreshInfo.Render(fmt.Sprintf("page %d/%d", currentPage, totalPages))
	footerGap := width - lipgloss.Width(footerLeft) - lipgloss.Width(pageInfo)
	if footerGap < 1 {
		footerGap = 1
	}
	footer := footerLeft + strings.Repeat(" ", footerGap) + pageInfo

	return lipgloss.JoinVertical(lipgloss.Left, header, tabBar, "", joined, "", hints, footer)
}

// renderTriageTabBar renders the filter tab bar for the expanded todo view.
func renderTriageTabBar(s *ccStyles, activeFilter string, counts map[string]int, width int) string {
	type tabDef struct {
		key   string
		label string
	}
	tabs := []tabDef{
		{"accepted", "Accepted"},
		{"new", "New"},
		{"review", "Review"},
		{"blocked", "Blocked"},
		{"active", "Active"},
		{"all", "All"},
	}

	var parts []string
	for _, tab := range tabs {
		count := counts[tab.key]
		label := fmt.Sprintf("%s (%d)", tab.label, count)
		if tab.key == activeFilter {
			// Active tab: bold cyan
			parts = append(parts, lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render(label))
		} else if count > 0 {
			// Non-zero count: normal white
			parts = append(parts, lipgloss.NewStyle().Foreground(s.ColorWhite).Render(label))
		} else {
			// Zero count: muted
			parts = append(parts, s.DescMuted.Render(label))
		}
	}

	return "  " + strings.Join(parts, "  ")
}

// renderTriageStatusBar renders a compact status bar for the normal (collapsed) todo view.
func renderTriageStatusBar(s *ccStyles, counts map[string]int, width int) string {
	type item struct {
		key        string
		label      string
		shortLabel string
	}
	items := []item{
		{"new", "New", "N"},
		{"review", "Review", "R"},
		{"blocked", "Blocked", "B"},
		{"active", "Active", "A"},
	}

	// Show the bar whenever there are active todos (any triage state).
	// counts["all"] includes every active todo regardless of triage status.
	if counts["all"] == 0 {
		return ""
	}

	useShort := width < 45
	var parts []string
	for _, it := range items {
		count := counts[it.key]
		label := it.label
		if useShort {
			label = it.shortLabel
		}
		text := fmt.Sprintf("%s(%d)", label, count)
		if count > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(s.ColorCyan).Render(text))
		} else {
			parts = append(parts, s.DescMuted.Render(text))
		}
	}

	bar := strings.Join(parts, s.DescMuted.Render(" \u00b7 "))
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, bar)
}

func renderHelpOverlay(s *ccStyles, subView string, width, height int) string {
	title := s.SectionHeader.Render("KEYBOARD SHORTCUTS")
	dismiss := s.CalendarTime.Render("Press any key to dismiss")

	var sections []string
	sections = append(sections, title, "", dismiss, "")

	global := []struct{ key, desc string }{
		{"tab / shift+tab", "Switch tabs"},
		{"esc", "Quit / cancel"},
		{"?", "Toggle this help"},
	}
	sections = append(sections, s.SectionHeader.Render("  Global"), "")
	for _, sh := range global {
		sections = append(sections, fmt.Sprintf("    %-20s %s",
			lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(sh.key),
			s.CalendarTime.Render(sh.desc)))
	}

	switch subView {
	case "command":
		cmds := []struct{ key, desc string }{
			{"\u2191\u2193 / k j", "Navigate todos"},
			{"shift+\u2191\u2193", "Move todo up/down"},
			{"enter", "View todo detail"},
			{"space", "Cycle expanded view (2-col / 1-col / collapse)"},
			{"o", "Launch Claude session for todo"},
			{"x", "Mark todo done"},
			{"X", "Dismiss todo (won't come back)"},
			{"u", "Undo last done/dismiss"},
			{"d", "Defer todo to bottom of list"},
			{"p", "Promote todo to top of list"},
			{"c", "Command — tell Claude what to do"},
			{"t", "Quick add todos (one per line)"},
			{"s", "Schedule time block for todo"},
			{"/", "Search/filter todos"},
			{"y", "Accept todo (triage)"},
			{"Y", "Accept + open task runner"},
			{"tab", "Cycle triage filter (expanded view)"},
			{"b", "Toggle completed backlog"},
			{"r", "Refresh from all sources"},
		}
		sections = append(sections, "", s.SectionHeader.Render("  Command Center"), "")
		for _, sh := range cmds {
			sections = append(sections, fmt.Sprintf("    %-20s %s",
				lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(sh.key),
				s.CalendarTime.Render(sh.desc)))
		}

	case "threads":
		cmds := []struct{ key, desc string }{
			{"\u2191\u2193 / k j", "Navigate threads"},
			{"enter", "Launch Claude session for thread"},
			{"a", "Add new thread"},
			{"p", "Pause active thread"},
			{"s", "Start paused thread"},
			{"x", "Close thread"},
		}
		sections = append(sections, "", s.SectionHeader.Render("  Threads"), "")
		for _, sh := range cmds {
			sections = append(sections, fmt.Sprintf("    %-20s %s",
				lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(sh.key),
				s.CalendarTime.Render(sh.desc)))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	box := s.PanelBorder.Width(50).Padding(1, 2).Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// renderTaskRunner renders the task runner launch configuration screen.
func renderTaskRunner(s *ccStyles, todo db.Todo, mode string, budget float64,
	step int, promptVP viewport.Model, width, height int,
	projectDir string, launchCursor int,
	pickingPath bool, filteredPaths []string, pathCursor int, pathFilter string,
	refining bool) string {

	innerWidth := width - 4
	if innerWidth < 40 {
		innerWidth = 40
	}

	// Header with truncated title
	titleMax := innerWidth - len("TASK RUNNER — ") - 2
	if titleMax < 10 {
		titleMax = 10
	}
	title := truncateToWidth(flattenTitle(todo.Title), titleMax)
	header := s.SectionHeader.Render("TASK RUNNER — " + title)

	switch step {
	case 1:
		return renderTaskRunnerStep1(s, header, projectDir, pickingPath, filteredPaths, pathCursor, pathFilter, innerWidth)
	case 2:
		return renderTaskRunnerStep2(s, header, projectDir, mode, innerWidth)
	case 3:
		return renderTaskRunnerStep3(s, header, projectDir, mode, promptVP, launchCursor, refining, innerWidth)
	default:
		return s.PanelBorder.Width(innerWidth).Render(header + "\n\n" + s.DescMuted.Render("  Unknown step"))
	}
}

// renderTaskRunnerStep1 renders Step 1/3: Project selection.
func renderTaskRunnerStep1(s *ccStyles, header, projectDir string, pickingPath bool, filteredPaths []string, pathCursor int, pathFilter string, innerWidth int) string {
	stepLabel := lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("Step 1/3: Project")

	dirName := shortDirName(projectDir)
	if dirName == "" {
		dirName = s.DescMuted.Render("(not set)")
	} else {
		dirName = lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true).Render(dirName)
	}

	fullPath := s.DescMuted.Render(projectDir)

	parts := []string{
		header,
		"",
		"  " + stepLabel,
		"",
		"  " + dirName,
		"  " + fullPath,
	}

	// Path picker overlay
	if pickingPath {
		filterDisplay := pathFilter
		if filterDisplay == "" {
			filterDisplay = s.DescMuted.Render("type to filter...")
		} else {
			filterDisplay = lipgloss.NewStyle().Foreground(s.ColorCyan).Render(filterDisplay)
		}
		parts = append(parts, "", "  "+filterDisplay)

		pickerSection := renderPathPickerOverlay(s, filteredPaths, pathCursor, innerWidth)
		if pickerSection != "" {
			parts = append(parts, pickerSection)
		}
	}

	// Hints
	var hint string
	if pickingPath {
		hint = s.Hint.Render("  j/k navigate \u00b7 type to filter \u00b7 enter select \u00b7 esc cancel")
	} else {
		hint = s.Hint.Render("  enter pick project · enter next step · esc back")
	}
	parts = append(parts, "", hint)

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return s.PanelBorder.Width(innerWidth).Render(content)
}

// renderTaskRunnerStep2 renders Step 2/3: Mode selection.
func renderTaskRunnerStep2(s *ccStyles, header, projectDir, mode string, innerWidth int) string {
	stepLabel := lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("Step 2/3: Mode")

	// Project reminder
	projectReminder := s.DescMuted.Render("  Project: " + shortDirName(projectDir))

	// Mode selector using renderTaskRunnerOptionRow (always highlighted since it's the active step)
	modeOptions := []string{"Normal", "Worktree", "Sandbox"}
	modeLine := renderTaskRunnerOptionRow(s, "Mode", modeOptions, mode, true, innerWidth)

	hint := s.Hint.Render("  ←/→ select mode · enter next step · esc back")

	parts := []string{
		header,
		"",
		"  " + stepLabel,
		projectReminder,
		"",
		modeLine,
		"",
		hint,
	}

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return s.PanelBorder.Width(innerWidth).Render(content)
}

// renderTaskRunnerStep3 renders Step 3/3: Prompt review and launch.
func renderTaskRunnerStep3(s *ccStyles, header, projectDir, mode string, promptVP viewport.Model, launchCursor int, refining bool, innerWidth int) string {
	stepLabel := lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("Step 3/3: Prompt")

	// Project + mode reminder
	reminder := s.DescMuted.Render(fmt.Sprintf("  Project: %s \u00b7 Mode: %s", shortDirName(projectDir), mode))

	// Prompt viewport
	divider := s.DescMuted.Render("  " + strings.Repeat("\u2500", innerWidth-4))
	promptHeader := s.SectionHeader.Render("  PROMPT")

	parts := []string{
		header,
		"",
		"  " + stepLabel,
		reminder,
		"",
		divider,
		promptHeader,
		"",
		promptVP.View(),
	}

	// Refining spinner
	if refining {
		refiningLine := "  " + lipgloss.NewStyle().Foreground(s.ColorCyan).Render("\u25cf") + " Refining prompt..."
		parts = append(parts, refiningLine)
	}

	// Launch selector: [ Queue ] Run Now
	queueLabel := "Queue"
	runNowLabel := "Run Now"
	if launchCursor == 0 {
		queueLabel = lipgloss.NewStyle().
			Background(s.ColorCyan).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1).
			Render(queueLabel)
		runNowLabel = s.DescMuted.Render(runNowLabel)
	} else {
		queueLabel = s.DescMuted.Render(queueLabel)
		runNowLabel = lipgloss.NewStyle().
			Background(s.ColorCyan).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1).
			Render(runNowLabel)
	}
	selector := "  " + queueLabel + "   " + runNowLabel

	hint := s.Hint.Render("  e edit prompt · r refine with AI · ←/→ launch option · enter launch · esc back")

	parts = append(parts, "", selector, hint)

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return s.PanelBorder.Width(innerWidth).Render(content)
}

// renderPathPickerOverlay renders the scrollable path picker list.
func renderPathPickerOverlay(s *ccStyles, filteredPaths []string, pathCursor int, innerWidth int) string {
	if len(filteredPaths) == 0 {
		return "  " + s.DescMuted.Render("No paths match filter")
	}

	maxVisible := 8
	startIdx := 0
	if pathCursor >= maxVisible {
		startIdx = pathCursor - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(filteredPaths) {
		endIdx = len(filteredPaths)
	}

	var pathLines []string
	for i := startIdx; i < endIdx; i++ {
		path := filteredPaths[i]
		displayPath := path
		if len(displayPath) > innerWidth-8 {
			displayPath = "..." + displayPath[len(displayPath)-(innerWidth-11):]
		}
		if i == pathCursor {
			pathLines = append(pathLines, lipgloss.NewStyle().
				Background(s.ColorCyan).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Padding(0, 1).
				Render(displayPath))
		} else {
			pathLines = append(pathLines, "  "+s.DescMuted.Render(displayPath))
		}
	}

	if startIdx > 0 {
		pathLines = append([]string{s.CalendarTime.Render(fmt.Sprintf("  \u25b2 %d more", startIdx))}, pathLines...)
	}
	if endIdx < len(filteredPaths) {
		pathLines = append(pathLines, s.CalendarTime.Render(fmt.Sprintf("  \u25bc %d more", len(filteredPaths)-endIdx)))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.ColorCyan).
		Width(innerWidth - 4).
		Padding(0, 1).
		Render(strings.Join(pathLines, "\n"))
}

// renderTaskRunnerOptionRow renders a single config row with selectable options.
func renderTaskRunnerOptionRow(s *ccStyles, label string, options []string, current string, isRowSelected bool, width int) string {
	labelStyle := s.SectionHeader
	if isRowSelected {
		labelStyle = lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true)
	}
	renderedLabel := labelStyle.Render(label + ":")

	var optParts []string
	for _, opt := range options {
		isActive := strings.EqualFold(opt, current)
		if isActive && isRowSelected {
			// Selected row + active option: cyan inverse
			optParts = append(optParts, lipgloss.NewStyle().
				Background(s.ColorCyan).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Padding(0, 1).
				Render(opt))
		} else if isActive {
			// Active option on non-selected row: bold white with brackets
			optParts = append(optParts, lipgloss.NewStyle().
				Foreground(s.ColorWhite).
				Bold(true).
				Render("["+opt+"]"))
		} else if isRowSelected {
			// Non-active option on selected row: muted with brackets
			optParts = append(optParts, s.DescMuted.Render("["+opt+"]"))
		} else {
			// Non-active option on non-selected row: muted
			optParts = append(optParts, s.DescMuted.Render(opt))
		}
	}

	return fmt.Sprintf("  %-14s %s", renderedLabel, strings.Join(optParts, " "))
}
