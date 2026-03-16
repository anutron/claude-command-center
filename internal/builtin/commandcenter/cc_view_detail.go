package commandcenter

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/lipgloss"
)

func renderDetailView(s *ccStyles, todo db.Todo, detailMode string, selectedField int, fieldInputView string, commandInputView string, width, height int, notice string, noticeType string, statusCursor int, filteredPaths []string, pathCursor int, pathFilter string, frame int, hasActiveSession bool) string {
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
	if todo.LaunchMode != "" {
		rightFields = append(rightFields, roField{"Mode", todo.LaunchMode})
	}
	rightFields = append(rightFields, roField{"Created", todo.CreatedAt.Format("Jan 2, 2006")})

	// Build left column lines
	var leftLines []string
	for _, f := range editableFields {
		label := s.SectionHeader.Render(f.label + ":")
		val := f.value
		if val == "" {
			val = s.DescMuted.Render("\u2014")
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
			val = s.DescMuted.Render("\u2014")
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
			pathLines = append([]string{s.CalendarTime.Render(fmt.Sprintf("  \u25b2 %d more", startIdx))}, pathLines...)
		}
		if endIdx < len(filteredPaths) {
			pathLines = append(pathLines, s.CalendarTime.Render(fmt.Sprintf("  \u25bc %d more", len(filteredPaths)-endIdx)))
		}

		pickerHint := s.Hint.Render("  j/k navigate \u00b7 type to filter \u00b7 enter select \u00b7 esc cancel")
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
		spinnerChar := refreshSpinner(frame)
		sessionIndicator := spinnerChar + " " + lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("Agent updating \u2014 edits blocked")
		sessionSection = "\n  " + sessionIndicator
	} else if todo.SessionStatus == "review" || todo.SessionStatus == "failed" {
		statusLabel := "completed"
		statusColor := s.ColorGreen
		if todo.SessionStatus == "failed" {
			statusLabel = "failed"
			statusColor = s.ColorYellow
		}
		sessionIndicator := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render("\u25cf Session: " + statusLabel)
		sessionSection = "\n  " + sessionIndicator
	}

	// Session summary section (shown when agent has completed work)
	// Note: summaryBodyLines is calculated below alongside prompt allocation;
	// we build the section content later after the height budget is computed.
	var summarySection string

	// Command input section (when in commandInput mode)
	var commandSection string
	if detailMode == "commandInput" {
		divider := s.DescMuted.Render(strings.Repeat("\u2500", innerWidth-2))
		inputLabel := s.DescMuted.Render("Tell me what changed:")
		// Use PaddingLeft to indent all textarea lines consistently (not just the first).
		// String concatenation ("  " + multiLineStr) only indents the first line.
		indentedInput := lipgloss.NewStyle().PaddingLeft(2).Render(commandInputView)
		commandSection = lipgloss.JoinVertical(lipgloss.Left,
			"",
			"  "+divider,
			"  "+inputLabel,
			indentedInput,
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

	// Calculate fixed chrome height to determine how much space prompt/summary get.
	// Fixed lines: TODO #N (1) + blank (1) + title (1) + blank (1) + field rows (len(fieldRows))
	// + blank before hints (1) + hints (1) + panel border (2)
	fixedLines := 8 + len(fieldRows) // header, blanks, title, field rows, footer hint, border
	if noticeBanner != "" {
		fixedLines += 2 // notice + blank
	}
	if sessionSection != "" {
		fixedLines += lipgloss.Height(sessionSection)
	}
	if pathPickerSection != "" {
		fixedLines += lipgloss.Height(pathPickerSection)
	}
	if detailSection != "" {
		fixedLines += lipgloss.Height(detailSection)
	}
	if commandSection != "" {
		fixedLines += lipgloss.Height(commandSection)
	}

	// Count lines used by summary section header/blanks (not body — body is flexible)
	summaryHeaderLines := 0
	if todo.SessionSummary != "" {
		summaryHeaderLines = 3 // blank + header + blank before body
	}

	// Count lines used by prompt section header/blanks (not body — body is flexible)
	promptHeaderLines := 0
	if todo.ProposedPrompt != "" {
		promptHeaderLines = 3 // blank + header + blank before body
	} else {
		fixedLines += 1 // "(no prompt set)" line
	}

	// Available lines for prompt + summary body content
	availableForContent := height - fixedLines - summaryHeaderLines - promptHeaderLines
	if availableForContent < 6 {
		availableForContent = 6
	}

	// Split available space between summary and prompt bodies
	summaryBodyLines := 0
	promptBodyMax := availableForContent
	if todo.SessionSummary != "" && todo.ProposedPrompt != "" {
		// Both present: count actual lines, split proportionally
		summaryWrapped := wrapText(todo.SessionSummary, innerWidth-6)
		summaryTotal := len(strings.Split(summaryWrapped, "\n"))
		promptTotal := len(strings.Split(todo.ProposedPrompt, "\n"))
		total := summaryTotal + promptTotal
		if total <= availableForContent {
			// Both fit — no truncation needed
			summaryBodyLines = summaryTotal
			promptBodyMax = promptTotal
		} else {
			// Split proportionally, giving each at least 3 lines
			summaryBodyLines = availableForContent * summaryTotal / total
			if summaryBodyLines < 3 {
				summaryBodyLines = 3
			}
			promptBodyMax = availableForContent - summaryBodyLines
			if promptBodyMax < 3 {
				promptBodyMax = 3
				summaryBodyLines = availableForContent - promptBodyMax
			}
		}
	} else if todo.SessionSummary != "" {
		summaryBodyLines = availableForContent
	}

	// Prompt section (read-only, not editable — prompts are managed in the task runner)
	var promptSection string
	promptText := todo.ProposedPrompt
	if promptText != "" {
		promptHeader := s.SectionHeader.Render("  PROMPT")
		promptLines := strings.Split(promptText, "\n")
		truncated := false
		if len(promptLines) > promptBodyMax {
			promptLines = promptLines[:promptBodyMax]
			truncated = true
		}
		if truncated {
			promptLines = append(promptLines, s.DescMuted.Render("... (press o to see full prompt)"))
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

	// Build session summary with dynamic height
	if todo.SessionSummary != "" {
		summaryHeader := s.SectionHeader.Render("  SESSION SUMMARY")
		wrapped := wrapText(todo.SessionSummary, innerWidth-6)
		allLines := strings.Split(wrapped, "\n")
		if summaryBodyLines > 0 && len(allLines) > summaryBodyLines {
			allLines = allLines[:summaryBodyLines]
			allLines = append(allLines, s.DescMuted.Render("... (truncated)"))
		}
		var summaryLines []string
		for _, line := range allLines {
			summaryLines = append(summaryLines, "   "+line)
		}
		summaryBody := lipgloss.NewStyle().Foreground(s.ColorWhite).Render(strings.Join(summaryLines, "\n"))
		summarySection = lipgloss.JoinVertical(lipgloss.Left, "", summaryHeader, "", summaryBody)
	}

	// Footer hints based on mode
	var hints string
	switch detailMode {
	case "viewing":
		baseHints := "j/k prev/next \u00b7 x done \u00b7 X remove \u00b7 tab cycle \u00b7 enter edit \u00b7 o launch"
		if todo.SessionID != "" && todo.SessionStatus != "active" && todo.SessionStatus != "queued" {
			baseHints += " \u00b7 r resume"
		}
		if hasActiveSession {
			baseHints += " \u00b7 w watch"
		}
		baseHints += " \u00b7 c command \u00b7 esc back"
		hints = s.Hint.Render(baseHints)
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
