package settings

import (
	"github.com/anutron/claude-command-center/internal/config"
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
}

func newSettingsStyles(p config.Palette) settingsStyles {
	borderDim := lipgloss.Color("#3b4261")
	borderBright := lipgloss.Color(p.Cyan)

	return settingsStyles{
		header:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cyan)).Bold(true),
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
			BorderForeground(borderBright),
		sidebarUnfocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderDim),
		contentFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderBright),
		contentUnfocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderDim),
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
	}
}
