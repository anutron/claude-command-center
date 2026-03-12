package granola

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Settings implements plugin.SettingsProvider for the Granola data source.
type Settings struct {
	cfg    *config.Config
	styles settingsStyles
}

type settingsStyles struct {
	muted    lipgloss.Style
	enabled  lipgloss.Style
	disabled lipgloss.Style
	logError lipgloss.Style
}

func newSettingsStyles(pal config.Palette) settingsStyles {
	return settingsStyles{
		muted:    lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Muted)),
		enabled:  lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Green)),
		disabled: lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Muted)),
		logError: lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")),
	}
}

// NewSettings creates a Granola SettingsProvider.
func NewSettings(cfg *config.Config, pal config.Palette) *Settings {
	return &Settings{
		cfg:    cfg,
		styles: newSettingsStyles(pal),
	}
}

func (s *Settings) SettingsView(width, height int) string {
	var lines []string

	statusText := "[off]"
	statusStyle := s.styles.disabled
	if s.cfg.Granola.Enabled {
		statusText = "[on] "
		statusStyle = s.styles.enabled
	}

	lines = append(lines, fmt.Sprintf("  %s %s",
		s.styles.muted.Render("Enabled:"),
		statusStyle.Render(statusText+" (space to toggle)")))

	credStatus := s.styles.enabled.Render("Token found")
	if err := config.ValidateGranola(); err != nil {
		credStatus = s.styles.logError.Render("Not configured")
	}
	lines = append(lines, fmt.Sprintf("  %s %s",
		s.styles.muted.Render("Credentials:"),
		credStatus))

	lines = append(lines, "")
	lines = append(lines, s.styles.muted.Render("  Open Granola app to refresh token"))

	return strings.Join(lines, "\n")
}

func (s *Settings) SettingsOpenCmd() tea.Cmd                          { return nil }
func (s *Settings) HandleSettingsMsg(msg tea.Msg) (bool, plugin.Action) { return false, plugin.NoopAction() }

func (s *Settings) HandleSettingsKey(msg tea.KeyMsg) plugin.Action {
	return plugin.Action{Type: plugin.ActionUnhandled}
}
