package settings

import (
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// settingsStyles holds styles for the settings plugin.
type settingsStyles struct {
	header    lipgloss.Style
	muted     lipgloss.Style
	pointer   lipgloss.Style
	enabled   lipgloss.Style
	disabled  lipgloss.Style
	itemName  lipgloss.Style
	flash     lipgloss.Style
	panel     lipgloss.Style
	activeTab lipgloss.Style
	logError  lipgloss.Style
	logWarn   lipgloss.Style
	logPlugin lipgloss.Style

	// Sidebar layout styles
	sidebarFocused   lipgloss.Style
	sidebarUnfocused lipgloss.Style
	contentFocused   lipgloss.Style
	contentUnfocused lipgloss.Style
	categoryHeader   lipgloss.Style
	navSelected      lipgloss.Style
	navUnselected    lipgloss.Style
	navEnabled       lipgloss.Style
	navDisabled      lipgloss.Style

	// huh form theme
	huhTheme *huh.Theme
}

// huhThemeFromPalette creates a huh Theme that maps palette colors to form styles.
// The theme removes the default left-border on focused fields since the settings
// panel already provides its own border chrome.
func huhThemeFromPalette(pal config.Palette) *huh.Theme {
	t := huh.ThemeBase()

	cyan := lipgloss.Color(pal.Cyan)
	green := lipgloss.Color(pal.Green)
	white := lipgloss.Color(pal.White)
	fg := lipgloss.Color(pal.Fg)
	muted := lipgloss.Color(pal.Muted)
	red := lipgloss.Color("#f7768e")

	// Focused field styles
	t.Focused.Base = lipgloss.NewStyle().PaddingLeft(1)
	t.Focused.Title = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	t.Focused.Description = lipgloss.NewStyle().Foreground(muted)
	t.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(red)
	t.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(red)
	t.Focused.SelectSelector = lipgloss.NewStyle().SetString("> ").Foreground(green)
	t.Focused.Option = lipgloss.NewStyle().Foreground(fg)
	t.Focused.SelectedOption = lipgloss.NewStyle().Foreground(green)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().SetString("[•] ").Foreground(green)
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().SetString("[ ] ").Foreground(muted)
	t.Focused.FocusedButton = lipgloss.NewStyle().Foreground(white).Background(cyan).Padding(0, 1)
	t.Focused.BlurredButton = lipgloss.NewStyle().Foreground(fg).Padding(0, 1)
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(cyan)
	t.Focused.TextInput.CursorText = lipgloss.NewStyle().Foreground(white)
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(muted)
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(cyan)
	t.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(white)

	// Blurred field styles — dimmed versions
	t.Blurred.Base = lipgloss.NewStyle().PaddingLeft(1)
	t.Blurred.SelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.Title = lipgloss.NewStyle().Foreground(muted)
	t.Blurred.Description = lipgloss.NewStyle().Foreground(muted)
	t.Blurred.TextInput.Cursor = lipgloss.NewStyle().Foreground(muted)
	t.Blurred.TextInput.CursorText = lipgloss.NewStyle().Foreground(fg)
	t.Blurred.TextInput.Placeholder = lipgloss.NewStyle().Foreground(muted)
	t.Blurred.TextInput.Prompt = lipgloss.NewStyle().Foreground(muted)
	t.Blurred.TextInput.Text = lipgloss.NewStyle().Foreground(fg)

	return t
}

func newSettingsStyles(p config.Palette) settingsStyles {
	borderDim := lipgloss.Color("#3b4261")
	borderBright := lipgloss.Color(p.Cyan)

	return settingsStyles{
		header:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan)).Bold(true).PaddingLeft(2),
		muted:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted)),
		pointer:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.Pointer)),
		enabled:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.Green)),
		disabled:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted)),
		itemName:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.White)),
		flash:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.Green)),
		activeTab: lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan)).Bold(true),
		panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderDim),
		logError:  lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")),
		logWarn:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.Yellow)),
		logPlugin: lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan)),

		// Sidebar layout styles
		sidebarFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderBright).
			PaddingLeft(1),
		sidebarUnfocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderDim).
			PaddingLeft(1),
		contentFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderBright).
			PaddingLeft(1),
		contentUnfocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderDim).
			PaddingLeft(1),
		categoryHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Cyan)).
			Bold(true),
		navSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.White)).
			Background(lipgloss.Color(p.SelectedBg)),
		navUnselected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Fg)),
		navEnabled: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Green)),
		navDisabled: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Muted)),

		huhTheme: huhThemeFromPalette(p),
	}
}
