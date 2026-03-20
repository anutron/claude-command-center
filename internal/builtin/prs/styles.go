package prs

import (
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// prsStyles is an alias for ui.Styles so render code compiles unchanged.
type prsStyles = ui.Styles

// prsGrad is an alias for ui.GradientColors.
type prsGrad = ui.GradientColors

// prRowStyle holds styles specific to PR row rendering.
type prRowStyle struct {
	success lipgloss.Style
	failure lipgloss.Style
	pending lipgloss.Style
	draft   lipgloss.Style
}

func newPRRowStyle(s *prsStyles) prRowStyle {
	return prRowStyle{
		success: lipgloss.NewStyle().Foreground(s.ColorGreen),
		failure: lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")),
		pending: lipgloss.NewStyle().Foreground(s.ColorYellow),
		draft:   lipgloss.NewStyle().Foreground(s.ColorMuted),
	}
}
