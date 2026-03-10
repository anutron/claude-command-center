package calendar

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Settings implements plugin.SettingsProvider for the calendar data source.
type Settings struct {
	cfg    *config.Config
	styles settingsStyles
}

type settingsStyles struct {
	header   lipgloss.Style
	muted    lipgloss.Style
	enabled  lipgloss.Style
	disabled lipgloss.Style
	itemName lipgloss.Style
	logError lipgloss.Style
}

func newSettingsStyles(pal config.Palette) settingsStyles {
	return settingsStyles{
		header:   lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Cyan)).Bold(true),
		muted:    lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Muted)),
		enabled:  lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Green)),
		disabled: lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Muted)),
		itemName: lipgloss.NewStyle().Foreground(lipgloss.Color(pal.White)),
		logError: lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")),
	}
}

// NewSettings creates a calendar SettingsProvider.
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
	if s.cfg.Calendar.Enabled {
		statusText = "[on] "
		statusStyle = s.styles.enabled
	}

	lines = append(lines, fmt.Sprintf("  %s %s",
		s.styles.muted.Render("Enabled:"),
		statusStyle.Render(statusText+" (space to toggle)")))

	credStatus := s.styles.enabled.Render("Configured")
	if err := config.ValidateCalendar(); err != nil {
		credStatus = s.styles.logError.Render("Not configured")
	}
	lines = append(lines, fmt.Sprintf("  %s %s",
		s.styles.muted.Render("Credentials:"),
		credStatus))

	lines = append(lines, "")
	lines = append(lines, s.styles.header.Render("  CALENDARS"))
	if len(s.cfg.Calendar.Calendars) == 0 {
		lines = append(lines, s.styles.muted.Render("  No calendars configured"))
	} else {
		for _, cal := range s.cfg.Calendar.Calendars {
			label := cal.ID
			if cal.Label != "" {
				label = cal.Label
			}
			lines = append(lines, fmt.Sprintf("    %s", s.styles.itemName.Render(label)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, s.styles.muted.Render("  Run 'ccc setup' to add or modify calendars"))

	return strings.Join(lines, "\n")
}

func (s *Settings) HandleSettingsKey(msg tea.KeyMsg) plugin.Action {
	return plugin.Action{Type: plugin.ActionUnhandled}
}
