package tui

import (
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/ui"
)

// GradientColors is an alias for ui.GradientColors.
type GradientColors = ui.GradientColors

// NewGradientColors delegates to ui.NewGradientColors.
func NewGradientColors(p config.Palette) GradientColors {
	return ui.NewGradientColors(p)
}
