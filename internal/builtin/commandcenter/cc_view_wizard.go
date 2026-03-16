package commandcenter

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// renderTaskRunner renders the task runner launch configuration screen.
func renderTaskRunner(s *ccStyles, todo db.Todo, mode string, budget float64,
	step int, promptVP viewport.Model, width, height int,
	projectDir string, launchCursor int,
	pickingPath bool, filteredPaths []string, pathCursor int, pathFilter string,
	refining bool, reviewing bool, inputting bool, instructInput textarea.Model) string {

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
	header := "  " + s.SectionHeader.Render("TASK RUNNER — "+title)

	switch step {
	case 1:
		return renderTaskRunnerStep1(s, header, projectDir, pickingPath, filteredPaths, pathCursor, pathFilter, innerWidth)
	case 2:
		return renderTaskRunnerStep2(s, header, projectDir, mode, innerWidth)
	case 3:
		return renderTaskRunnerStep3(s, header, projectDir, mode, promptVP, launchCursor, refining, reviewing, inputting, instructInput, innerWidth)
	default:
		return s.PanelBorder.Width(innerWidth).Render("\n" + header + "\n\n" + s.DescMuted.Render("  Unknown step"))
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
		"",
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
		hint = s.Hint.Render("  / pick project \u00b7 enter accept \u00b7 esc exit")
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

	hint := s.Hint.Render("  \u2190/\u2192 select mode \u00b7 enter next step \u00b7 esc back")

	parts := []string{
		"",
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
func renderTaskRunnerStep3(s *ccStyles, header, projectDir, mode string, promptVP viewport.Model, launchCursor int, refining bool, reviewing bool, inputting bool, instructInput textarea.Model, innerWidth int) string {
	stepLabel := lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("Step 3/3: Prompt")

	// Project + mode reminder
	reminder := s.DescMuted.Render(fmt.Sprintf("  Project: %s \u00b7 Mode: %s", shortDirName(projectDir), mode))

	// Prompt viewport
	divider := s.DescMuted.Render("  " + strings.Repeat("\u2500", innerWidth-4))
	promptHeader := s.SectionHeader.Render("  PROMPT")

	parts := []string{
		"",
		header,
		"",
		"  " + stepLabel,
		reminder,
		"",
		divider,
		promptHeader,
		"",
		lipgloss.NewStyle().PaddingLeft(3).Render(promptVP.View()),
	}

	// Instruction input (c key)
	if inputting {
		inputLabel := lipgloss.NewStyle().Foreground(s.ColorCyan).Bold(true).Render("  Instructions:")
		parts = append(parts, "", inputLabel, "  "+instructInput.View())
		inputHint := s.Hint.Render("  enter send \u00b7 esc cancel")
		parts = append(parts, inputHint)

		content := lipgloss.JoinVertical(lipgloss.Left, parts...)
		return s.PanelBorder.Width(innerWidth).Render(content)
	}

	// Reviewing modal — Plannotator is open in browser
	if reviewing {
		reviewLine := "  " + lipgloss.NewStyle().Foreground(s.ColorCyan).Render("\u25cf") + " Reviewing prompt in browser..."
		reviewHint := s.Hint.Render("  esc cancel")
		parts = append(parts, "", reviewLine, reviewHint)
		content := lipgloss.JoinVertical(lipgloss.Left, parts...)
		return s.PanelBorder.Width(innerWidth).Render(content)
	}

	// Refining spinner
	if refining {
		refiningLine := "  " + lipgloss.NewStyle().Foreground(s.ColorCyan).Render("\u25cf") + " Refining prompt..."
		parts = append(parts, refiningLine)
	}

	// Launch selector: [ Run Claude ] Queue Agent  Run Agent Now
	labels := []string{"Run Claude", "Queue Agent", "Run Agent Now"}
	selectedStyle := lipgloss.NewStyle().
		Background(s.ColorCyan).
		Foreground(lipgloss.Color("#000000")).
		Bold(true).
		Padding(0, 1)
	var rendered []string
	for i, label := range labels {
		if i == launchCursor {
			rendered = append(rendered, selectedStyle.Render(label))
		} else {
			rendered = append(rendered, s.DescMuted.Render(label))
		}
	}
	selector := "  " + strings.Join(rendered, "   ")

	hint := s.Hint.Render("  e edit prompt \u00b7 r refine with AI \u00b7 \u2190/\u2192 launch option \u00b7 enter launch \u00b7 esc back")

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
