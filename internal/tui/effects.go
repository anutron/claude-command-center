package tui

import (
	"math"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// Animation constants
const (
	tickFPS      = 18
	fadeInFrames = 9  // ~0.5 seconds at 18 FPS
	shimmerSpeed = 0.006
	pulsePeriod  = 54 // frames per pulse cycle (~3 seconds at 18 FPS)
)

// GradientColors holds the parsed gradient colors from a palette.
type GradientColors struct {
	Start  colorful.Color
	Mid    colorful.Color
	End    colorful.Color
	BgDark colorful.Color
	DimCyan    colorful.Color
	BrightCyan colorful.Color
}

// NewGradientColors creates gradient colors from a palette.
func NewGradientColors(p config.Palette) GradientColors {
	start, _ := colorful.Hex(p.GradStart)
	mid, _ := colorful.Hex(p.GradMid)
	end, _ := colorful.Hex(p.GradEnd)
	bg, _ := colorful.Hex(p.BgDark)
	// Dim/bright variants for pulsing pointer
	dim, _ := colorful.Hex(p.Pointer)
	bright, _ := colorful.Hex(p.Pointer)
	// Make dim version by blending toward bg
	dim = bg.BlendLab(dim, 0.6)
	return GradientColors{
		Start:      start,
		Mid:        mid,
		End:        end,
		BgDark:     bg,
		DimCyan:    dim,
		BrightCyan: bright,
	}
}

// tickMsg drives all frame-based animation.
type tickMsg time.Time

// tickCmd returns a command that fires tickMsg at the configured FPS.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/tickFPS, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// gradientColor returns the interpolated color at position t in [0, 1]
// using a ping-pong pattern.
func gradientColor(g *GradientColors, t float64) colorful.Color {
	t = math.Mod(t, 2.0)
	if t < 0 {
		t += 2.0
	}
	if t > 1.0 {
		t = 2.0 - t
	}
	if t < 0.5 {
		return g.Start.BlendLab(g.Mid, t*2)
	}
	return g.Mid.BlendLab(g.End, (t-0.5)*2)
}

// fadeMultiplier returns a brightness factor [0, 1] using smoothstep easing.
func fadeMultiplier(frame int) float64 {
	if frame >= fadeInFrames {
		return 1.0
	}
	t := float64(frame) / float64(fadeInFrames)
	return t * t * (3.0 - 2.0*t)
}

// applyFade blends a color toward the dark background based on frame count.
func applyFade(g *GradientColors, c colorful.Color, frame int) colorful.Color {
	fade := fadeMultiplier(frame)
	return g.BgDark.BlendLab(c, fade)
}

// pulsingPointerStyle returns a lipgloss style with breathing brightness.
func pulsingPointerStyle(g *GradientColors, frame int) lipgloss.Style {
	phase := float64(frame) / float64(pulsePeriod) * 2.0 * math.Pi
	brightness := 0.7 + 0.3*math.Sin(phase)
	c := g.DimCyan.BlendLab(g.BrightCyan, brightness)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
}
