package commandcenter

import (
	"math"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// ccStyles holds all lipgloss styles needed by the command center plugin.
type ccStyles struct {
	SectionHeader lipgloss.Style
	CalendarTime  lipgloss.Style
	CalendarFree  lipgloss.Style
	DueOverdue    lipgloss.Style
	DueSoon       lipgloss.Style
	DueLater      lipgloss.Style
	PanelBorder   lipgloss.Style
	Hint          lipgloss.Style
	RefreshInfo   lipgloss.Style
	Suggestion    lipgloss.Style
	DescMuted     lipgloss.Style
	SelectedItem  lipgloss.Style
	TitleBoldC    lipgloss.Style
	TitleBoldW    lipgloss.Style
	ThreadActive  lipgloss.Style
	ThreadPaused  lipgloss.Style
	ActiveTab     lipgloss.Style
	InactiveTab   lipgloss.Style

	// Colors
	ColorWhite  lipgloss.Color
	ColorMuted  lipgloss.Color
	ColorCyan   lipgloss.Color
	ColorYellow lipgloss.Color
	ColorGreen  lipgloss.Color
	ColorPurple lipgloss.Color
}

// DueStyle returns the appropriate style for a due urgency level.
func (s *ccStyles) DueStyle(urgency string) lipgloss.Style {
	switch urgency {
	case "overdue":
		return s.DueOverdue
	case "soon":
		return s.DueSoon
	case "later":
		return s.DueLater
	default:
		return s.DueLater
	}
}

func newCCStyles(p config.Palette) ccStyles {
	colorMuted := lipgloss.Color(p.Muted)
	colorCyan := lipgloss.Color(p.Cyan)
	colorYellow := lipgloss.Color(p.Yellow)
	colorWhite := lipgloss.Color(p.White)
	colorPurple := lipgloss.Color(p.Purple)
	colorGreen := lipgloss.Color(p.Green)
	colorSelectedBg := lipgloss.Color(p.SelectedBg)

	return ccStyles{
		SectionHeader: lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		CalendarTime:  lipgloss.NewStyle().Foreground(colorMuted),
		CalendarFree:  lipgloss.NewStyle().Foreground(colorMuted).Faint(true),
		DueOverdue:    lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")),
		DueSoon:       lipgloss.NewStyle().Foreground(colorYellow),
		DueLater:      lipgloss.NewStyle().Foreground(colorMuted),
		Suggestion:    lipgloss.NewStyle().Foreground(colorPurple).Italic(true),
		PanelBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3b4261")),
		Hint:         lipgloss.NewStyle().Foreground(colorMuted),
		RefreshInfo:  lipgloss.NewStyle().Foreground(colorMuted),
		DescMuted:    lipgloss.NewStyle().Foreground(colorMuted),
		SelectedItem: lipgloss.NewStyle().Foreground(colorWhite).Background(colorSelectedBg),
		TitleBoldC:   lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		TitleBoldW:   lipgloss.NewStyle().Foreground(colorWhite).Bold(true),
		ThreadActive: lipgloss.NewStyle().Foreground(colorWhite),
		ThreadPaused: lipgloss.NewStyle().Foreground(colorMuted),
		ActiveTab:    lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		InactiveTab:  lipgloss.NewStyle().Foreground(colorMuted),

		ColorWhite:  colorWhite,
		ColorMuted:  colorMuted,
		ColorCyan:   colorCyan,
		ColorYellow: colorYellow,
		ColorGreen:  colorGreen,
		ColorPurple: colorPurple,
	}
}

// gradientColors holds parsed gradient colors for animations.
type gradientColors struct {
	Start      colorful.Color
	Mid        colorful.Color
	End        colorful.Color
	BgDark     colorful.Color
	DimCyan    colorful.Color
	BrightCyan colorful.Color
}

func newGradientColors(p config.Palette) gradientColors {
	start, _ := colorful.Hex(p.GradStart)
	mid, _ := colorful.Hex(p.GradMid)
	end, _ := colorful.Hex(p.GradEnd)
	bg, _ := colorful.Hex(p.BgDark)
	dim, _ := colorful.Hex(p.Pointer)
	bright, _ := colorful.Hex(p.Pointer)
	dim = bg.BlendLab(dim, 0.6)
	return gradientColors{
		Start:      start,
		Mid:        mid,
		End:        end,
		BgDark:     bg,
		DimCyan:    dim,
		BrightCyan: bright,
	}
}

// Animation constants.
const (
	pulsePeriod = 54 // frames per pulse cycle (~3 seconds at 18 FPS)
)

// pulsingPointerStyle returns a lipgloss style with breathing brightness.
func pulsingPointerStyle(g *gradientColors, frame int) lipgloss.Style {
	phase := float64(frame) / float64(pulsePeriod) * 2.0 * math.Pi
	brightness := 0.7 + 0.3*math.Sin(phase)
	c := g.DimCyan.BlendLab(g.BrightCyan, brightness)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
}

// loadingSpinnerChar returns a braille spinner character for the given frame.
func loadingSpinnerChar(frame int) string {
	chars := []string{"\u280b", "\u2819", "\u2839", "\u2838", "\u283c", "\u2834", "\u2826", "\u2827", "\u2807", "\u280f"}
	return chars[(frame/2)%len(chars)]
}
