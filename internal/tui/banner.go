package tui

import (
	"strings"

	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// Generic ASCII banner using block characters
const asciiBanner = `
 ██████╗ ██████╗ ██████╗
██╔════╝██╔════╝██╔════╝
██║     ██║     ██║
██║     ██║     ██║
╚██████╗╚██████╗╚██████╗
 ╚═════╝ ╚═════╝ ╚═════╝`

const defaultSubtitle = "C O M M A N D   C E N T E R"

// renderBanner renders the static fallback banner.
func renderBanner(s *Styles, name string, width int) string {
	b := s.Banner.Render(strings.TrimPrefix(asciiBanner, "\n"))
	sub := subtitleFromName(name)
	st := s.Subtitle.Render(sub)
	block := lipgloss.JoinVertical(lipgloss.Center, b, st)
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
}

// renderGradientBanner renders the ASCII art with an animated gradient and fade-in.
func renderGradientBanner(g *GradientColors, name string, width int, frame int) string {
	bannerText := strings.TrimPrefix(asciiBanner, "\n")
	lines := strings.Split(bannerText, "\n")

	maxWidth := 0
	for _, line := range lines {
		runes := []rune(line)
		if len(runes) > maxWidth {
			maxWidth = len(runes)
		}
	}
	if maxWidth == 0 {
		maxWidth = 1
	}

	offset := float64(frame) * ui.ShimmerSpeed

	var bannerLines []string
	for _, line := range lines {
		runes := []rune(line)
		var styled strings.Builder
		for x, r := range runes {
			if r == ' ' {
				styled.WriteRune(' ')
				continue
			}
			t := float64(x)/float64(maxWidth) + offset
			c := ui.GradientColor(g, t)
			c = ui.ApplyFade(g, c, frame)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
			styled.WriteString(style.Render(string(r)))
		}
		bannerLines = append(bannerLines, styled.String())
	}

	banner := strings.Join(bannerLines, "\n")

	sub := subtitleFromName(name)
	subtitleColor := ui.ApplyFade(g, g.Mid, frame)
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(subtitleColor.Hex())).Render(sub)

	block := lipgloss.JoinVertical(lipgloss.Center, banner, s)
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
}

// subtitleFromName generates a spaced-out subtitle from the config name.
func subtitleFromName(name string) string {
	if name == "" {
		return defaultSubtitle
	}
	upper := strings.ToUpper(name)
	var spaced strings.Builder
	for i, r := range upper {
		if i > 0 {
			spaced.WriteRune(' ')
		}
		spaced.WriteRune(r)
	}
	return spaced.String()
}
