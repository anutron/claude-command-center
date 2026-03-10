package tui

import (
	"strings"

	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// blockFont maps uppercase characters to 6-line block art.
// Each character is a slice of 6 strings (one per row).
var blockFont = map[rune][6]string{
	'A': {
		" █████╗ ",
		"██╔══██╗",
		"███████║",
		"██╔══██║",
		"██║  ██║",
		"╚═╝  ╚═╝",
	},
	'B': {
		"██████╗ ",
		"██╔══██╗",
		"██████╔╝",
		"██╔══██╗",
		"██████╔╝",
		"╚═════╝ ",
	},
	'C': {
		" ██████╗",
		"██╔════╝",
		"██║     ",
		"██║     ",
		"╚██████╗",
		" ╚═════╝",
	},
	'D': {
		"██████╗ ",
		"██╔══██╗",
		"██║  ██║",
		"██║  ██║",
		"██████╔╝",
		"╚═════╝ ",
	},
	'E': {
		"███████╗",
		"██╔════╝",
		"█████╗  ",
		"██╔══╝  ",
		"███████╗",
		"╚══════╝",
	},
	'F': {
		"███████╗",
		"██╔════╝",
		"█████╗  ",
		"██╔══╝  ",
		"██║     ",
		"╚═╝     ",
	},
	'G': {
		" ██████╗ ",
		"██╔════╝ ",
		"██║  ███╗",
		"██║   ██║",
		"╚██████╔╝",
		" ╚═════╝ ",
	},
	'H': {
		"██╗  ██╗",
		"██║  ██║",
		"███████║",
		"██╔══██║",
		"██║  ██║",
		"╚═╝  ╚═╝",
	},
	'I': {
		"██╗",
		"██║",
		"██║",
		"██║",
		"██║",
		"╚═╝",
	},
	'J': {
		"     ██╗",
		"     ██║",
		"     ██║",
		"██   ██║",
		"╚█████╔╝",
		" ╚════╝ ",
	},
	'K': {
		"██╗  ██╗",
		"██║ ██╔╝",
		"█████╔╝ ",
		"██╔═██╗ ",
		"██║  ██╗",
		"╚═╝  ╚═╝",
	},
	'L': {
		"██╗     ",
		"██║     ",
		"██║     ",
		"██║     ",
		"███████╗",
		"╚══════╝",
	},
	'M': {
		"███╗   ███╗",
		"████╗ ████║",
		"██╔████╔██║",
		"██║╚██╔╝██║",
		"██║ ╚═╝ ██║",
		"╚═╝     ╚═╝",
	},
	'N': {
		"███╗   ██╗",
		"████╗  ██║",
		"██╔██╗ ██║",
		"██║╚██╗██║",
		"██║ ╚████║",
		"╚═╝  ╚═══╝",
	},
	'O': {
		" ██████╗ ",
		"██╔═══██╗",
		"██║   ██║",
		"██║   ██║",
		"╚██████╔╝",
		" ╚═════╝ ",
	},
	'P': {
		"██████╗ ",
		"██╔══██╗",
		"██████╔╝",
		"██╔═══╝ ",
		"██║     ",
		"╚═╝     ",
	},
	'Q': {
		" ██████╗ ",
		"██╔═══██╗",
		"██║   ██║",
		"██║▄▄ ██║",
		"╚██████╔╝",
		" ╚══▀▀═╝ ",
	},
	'R': {
		"██████╗ ",
		"██╔══██╗",
		"██████╔╝",
		"██╔══██╗",
		"██║  ██║",
		"╚═╝  ╚═╝",
	},
	'S': {
		"███████╗",
		"██╔════╝",
		"███████╗",
		"╚════██║",
		"███████║",
		"╚══════╝",
	},
	'T': {
		"████████╗",
		"╚══██╔══╝",
		"   ██║   ",
		"   ██║   ",
		"   ██║   ",
		"   ╚═╝   ",
	},
	'U': {
		"██╗   ██╗",
		"██║   ██║",
		"██║   ██║",
		"██║   ██║",
		"╚██████╔╝",
		" ╚═════╝ ",
	},
	'V': {
		"██╗   ██╗",
		"██║   ██║",
		"██║   ██║",
		"╚██╗ ██╔╝",
		" ╚████╔╝ ",
		"  ╚═══╝  ",
	},
	'W': {
		"██╗    ██╗",
		"██║    ██║",
		"██║ █╗ ██║",
		"██║███╗██║",
		"╚███╔███╔╝",
		" ╚══╝╚══╝ ",
	},
	'X': {
		"██╗  ██╗",
		"╚██╗██╔╝",
		" ╚███╔╝ ",
		" ██╔██╗ ",
		"██╔╝ ██╗",
		"╚═╝  ╚═╝",
	},
	'Y': {
		"██╗   ██╗",
		"╚██╗ ██╔╝",
		" ╚████╔╝ ",
		"  ╚██╔╝  ",
		"   ██║   ",
		"   ╚═╝   ",
	},
	'Z': {
		"███████╗",
		"╚══███╔╝",
		"  ███╔╝ ",
		" ███╔╝  ",
		"███████╗",
		"╚══════╝",
	},
	'0': {
		" ██████╗ ",
		"██╔═══██╗",
		"██║   ██║",
		"██║   ██║",
		"╚██████╔╝",
		" ╚═════╝ ",
	},
	'1': {
		" ██╗",
		"███║",
		"╚██║",
		" ██║",
		" ██║",
		" ╚═╝",
	},
	'2': {
		"██████╗ ",
		"╚════██╗",
		" █████╔╝",
		"██╔═══╝ ",
		"███████╗",
		"╚══════╝",
	},
	'3': {
		"██████╗ ",
		"╚════██╗",
		" █████╔╝",
		" ╚═══██╗",
		"██████╔╝",
		"╚═════╝ ",
	},
	'4': {
		"██╗  ██╗",
		"██║  ██║",
		"███████║",
		"╚════██║",
		"     ██║",
		"     ╚═╝",
	},
	'5': {
		"███████╗",
		"██╔════╝",
		"███████╗",
		"╚════██║",
		"███████║",
		"╚══════╝",
	},
	'6': {
		" ██████╗",
		"██╔════╝",
		"██████╗ ",
		"██╔══██╗",
		"╚█████╔╝",
		" ╚════╝ ",
	},
	'7': {
		"███████╗",
		"╚════██║",
		"    ██╔╝",
		"   ██╔╝ ",
		"   ██║  ",
		"   ╚═╝  ",
	},
	'8': {
		" █████╗ ",
		"██╔══██╗",
		"╚█████╔╝",
		"██╔══██╗",
		"╚█████╔╝",
		" ╚════╝ ",
	},
	'9': {
		" █████╗ ",
		"██╔══██╗",
		"╚██████║",
		" ╚═══██║",
		" █████╔╝",
		" ╚════╝ ",
	},
	'-': {
		"        ",
		"        ",
		"███████╗",
		"╚══════╝",
		"        ",
		"        ",
	},
	' ': {
		"   ",
		"   ",
		"   ",
		"   ",
		"   ",
		"   ",
	},
}

const bannerRows = 6

// textToBanner converts a string into 6-line block art.
// Unknown characters are skipped.
func textToBanner(text string) string {
	upper := strings.ToUpper(text)
	rows := make([]strings.Builder, bannerRows)
	for _, r := range upper {
		glyph, ok := blockFont[r]
		if !ok {
			continue
		}
		for i := 0; i < bannerRows; i++ {
			rows[i].WriteString(glyph[i])
		}
	}
	lines := make([]string, bannerRows)
	for i := 0; i < bannerRows; i++ {
		lines[i] = rows[i].String()
	}
	return strings.Join(lines, "\n")
}

// subtitleFromText generates a spaced-out uppercase subtitle.
// Returns empty string if text is empty.
func subtitleFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	upper := strings.ToUpper(text)
	var spaced strings.Builder
	for i, r := range upper {
		if i > 0 {
			spaced.WriteRune(' ')
		}
		spaced.WriteRune(r)
	}
	return spaced.String()
}

// renderBanner renders the static fallback banner.
func renderBanner(s *Styles, name, subtitle string, width int) string {
	bannerText := textToBanner(name)
	b := s.Banner.Render(bannerText)

	parts := []string{b}
	sub := subtitleFromText(subtitle)
	if sub != "" {
		parts = append(parts, s.Subtitle.Render(sub))
	}

	block := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
}

// renderGradientBanner renders dynamically-generated block art with an animated gradient.
func renderGradientBanner(g *GradientColors, name, subtitle string, width int, frame int) string {
	bannerText := textToBanner(name)
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

	parts := []string{banner}
	sub := subtitleFromText(subtitle)
	if sub != "" {
		subtitleColor := ui.ApplyFade(g, g.Mid, frame)
		s := lipgloss.NewStyle().Foreground(lipgloss.Color(subtitleColor.Hex())).Render(sub)
		parts = append(parts, s)
	}

	block := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
}
