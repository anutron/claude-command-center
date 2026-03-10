package tui

import (
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/ui"
)

// Styles is an alias for ui.Styles so existing tui code compiles unchanged.
type Styles = ui.Styles

// NewStyles delegates to ui.NewStyles.
func NewStyles(p config.Palette) Styles {
	return ui.NewStyles(p)
}
