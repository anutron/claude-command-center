package commandcenter

import (
	"github.com/anutron/claude-command-center/internal/ui"
)

// ccStyles is an alias for ui.Styles so render code compiles unchanged.
type ccStyles = ui.Styles

// gradientColors is an alias for ui.GradientColors.
type gradientColors = ui.GradientColors

// loadingSpinnerChar returns a braille spinner character for the given frame.
func loadingSpinnerChar(frame int) string {
	chars := []string{"\u280b", "\u2819", "\u2839", "\u2838", "\u283c", "\u2834", "\u2826", "\u2827", "\u2807", "\u280f"}
	return chars[(frame/2)%len(chars)]
}
