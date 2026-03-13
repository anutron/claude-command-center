package calendar

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Preset colors for per-calendar color picker.
var presetColors = []struct {
	Name string
	Hex  string
}{
	{"Red", "#f7768e"},
	{"Blue", "#7aa2f7"},
	{"Green", "#9ece6a"},
	{"Yellow", "#e0af68"},
	{"Purple", "#bb9af7"},
	{"Cyan", "#7dcfff"},
	{"Orange", "#ff9e64"},
	{"Pink", "#f7768e"},
}

// CalendarFetchResultMsg is a tea.Msg carrying the result of fetching available calendars.
type CalendarFetchResultMsg struct {
	Calendars []CalendarInfo
	Err       error
}

// Settings implements plugin.SettingsProvider for the calendar data source.
type Settings struct {
	cfg    *config.Config
	logger plugin.Logger
	styles settingsStyles

	cursor int // cursor within the calendar list

	// Add mode state
	addMode    bool
	addPhase   int // 0=id, 1=label
	idInput    textinput.Model
	labelInput textinput.Model

	// Color picker state
	colorPicking bool
	colorCursor  int

	// Fetched Google calendars state
	fetchedCalendars []CalendarInfo
	fetchLoading     bool
	fetchError       string
	fetchMode        bool // browsing fetched calendars
	fetchCursor      int
}

type settingsStyles struct {
	header   lipgloss.Style
	muted    lipgloss.Style
	enabled  lipgloss.Style
	disabled lipgloss.Style
	itemName lipgloss.Style
	logError lipgloss.Style
	pointer  lipgloss.Style
}

func newSettingsStyles(pal config.Palette) settingsStyles {
	return settingsStyles{
		header:   lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Cyan)).Bold(true),
		muted:    lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Muted)),
		enabled:  lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Green)),
		disabled: lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Muted)),
		itemName: lipgloss.NewStyle().Foreground(lipgloss.Color(pal.White)),
		logError: lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")),
		pointer:  lipgloss.NewStyle().Foreground(lipgloss.Color(pal.Pointer)),
	}
}

// NewSettings creates a calendar SettingsProvider.
func NewSettings(cfg *config.Config, pal config.Palette, logger plugin.Logger) *Settings {
	idInput := textinput.New()
	idInput.Placeholder = "calendar-id@group.calendar.google.com"
	idInput.CharLimit = 120
	idInput.Width = 50

	labelInput := textinput.New()
	labelInput.Placeholder = "Work"
	labelInput.CharLimit = 30
	labelInput.Width = 20

	return &Settings{
		cfg:        cfg,
		logger:     logger,
		styles:     newSettingsStyles(pal),
		idInput:    idInput,
		labelInput: labelInput,
	}
}

// FetchLoading returns whether the calendar list is currently being fetched.
func (s *Settings) FetchLoading() bool { return s.fetchLoading }

// FetchError returns the last fetch error message, or empty string if none.
func (s *Settings) FetchError() string { return s.fetchError }

// FetchedCalendars returns the fetched calendar list.
func (s *Settings) FetchedCalendars() []CalendarInfo { return s.fetchedCalendars }

// ResetEditing resets editing state when the detail view is opened.
func (s *Settings) ResetEditing() {
	s.cursor = 0
	s.addMode = false
	s.colorPicking = false
	s.fetchMode = false
	s.idInput.Blur()
	s.labelInput.Blur()
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
		statusStyle.Render(statusText)))

	credResult := ValidateCalendarResult()
	credStatus := s.styles.enabled.Render("Configured")
	if credResult.Status != "ok" {
		credStatus = s.styles.logError.Render("Not configured")
	}
	lines = append(lines, fmt.Sprintf("  %s %s",
		s.styles.muted.Render("Credentials:"),
		credStatus))

	lines = append(lines, "")
	lines = append(lines, s.styles.header.Render("  CALENDARS"))

	// Color picker overlay
	if s.colorPicking {
		lines = append(lines, s.viewColorPicker()...)
		return strings.Join(lines, "\n")
	}

	// Add mode overlay
	if s.addMode {
		lines = append(lines, s.viewAddMode()...)
		return strings.Join(lines, "\n")
	}

	// Fetch mode: show available Google calendars with add/remove toggles
	if s.fetchMode {
		lines = append(lines, s.viewFetchMode()...)
		return strings.Join(lines, "\n")
	}

	if len(s.cfg.Calendar.Calendars) == 0 {
		lines = append(lines, s.styles.muted.Render("  No calendars configured"))
	} else {
		for i, cal := range s.cfg.Calendar.Calendars {
			cursor := "  "
			if i == s.cursor {
				cursor = s.styles.pointer.Render("> ")
			}

			// Per-calendar enabled status
			calEnabled := cal.IsEnabled()
			toggle := s.styles.enabled.Render("[on] ")
			if !calEnabled {
				toggle = s.styles.disabled.Render("[off]")
			}

			// Color swatch
			colorSwatch := ""
			if cal.Color != "" {
				colorSwatch = lipgloss.NewStyle().
					Foreground(lipgloss.Color(cal.Color)).
					Render("██") + " "
			}

			label := cal.ID
			if cal.Label != "" {
				label = cal.Label
			}
			nameStyle := s.styles.itemName
			if !calEnabled {
				nameStyle = s.styles.muted
			}

			lines = append(lines, fmt.Sprintf("  %s%s %s%s", cursor, toggle, colorSwatch, nameStyle.Render(label)))
		}
	}

	lines = append(lines, "")

	// Show fetch status hint
	if s.fetchLoading {
		lines = append(lines, s.styles.muted.Render("  Fetching calendars from Google..."))
		lines = append(lines, "")
	} else if s.fetchError != "" {
		lines = append(lines, s.styles.logError.Render("  Fetch error: "+s.fetchError))
		lines = append(lines, "")
	}

	hintParts := "  ↑↓ navigate · space toggle · c color · a add manually · x remove"
	if len(s.fetchedCalendars) > 0 {
		hintParts += " · f browse Google calendars"
	} else if !s.fetchLoading {
		hintParts += " · f fetch from Google"
	}
	lines = append(lines, s.styles.muted.Render(hintParts))

	return strings.Join(lines, "\n")
}

func (s *Settings) viewColorPicker() []string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, s.styles.header.Render("  PICK COLOR"))

	for i, c := range presetColors {
		cursor := "  "
		if i == s.colorCursor {
			cursor = s.styles.pointer.Render("> ")
		}
		swatch := lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Hex)).
			Render("██")
		lines = append(lines, fmt.Sprintf("  %s%s %s", cursor, swatch, s.styles.itemName.Render(c.Name)))
	}

	// "None" option to remove color
	cursor := "  "
	if s.colorCursor == len(presetColors) {
		cursor = s.styles.pointer.Render("> ")
	}
	lines = append(lines, fmt.Sprintf("  %s%s", cursor, s.styles.muted.Render("(none)")))

	lines = append(lines, "")
	lines = append(lines, s.styles.muted.Render("  ↑↓ navigate · enter select · esc cancel"))

	return lines
}

func (s *Settings) viewAddMode() []string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, s.styles.header.Render("  ADD CALENDAR"))
	if s.addPhase == 0 {
		lines = append(lines, "  ID: "+s.idInput.View())
	} else {
		lines = append(lines, fmt.Sprintf("  ID: %s", s.styles.itemName.Render(s.idInput.Value())))
		lines = append(lines, "  Label: "+s.labelInput.View())
	}
	lines = append(lines, "")
	lines = append(lines, s.styles.muted.Render("  enter confirm · esc cancel"))
	return lines
}

func (s *Settings) HandleSettingsKey(msg tea.KeyMsg) plugin.Action {
	// Color picker mode
	if s.colorPicking {
		return s.handleColorPickerKey(msg)
	}

	// Add mode
	if s.addMode {
		return s.handleAddKey(msg)
	}

	// Fetch/browse mode
	if s.fetchMode {
		return s.handleFetchKey(msg)
	}

	switch msg.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
		return plugin.NoopAction()
	case "down", "j":
		if s.cursor < len(s.cfg.Calendar.Calendars)-1 {
			s.cursor++
		}
		return plugin.NoopAction()
	case " ":
		// Toggle per-calendar enabled (handled by settings plugin for the
		// data source toggle; this handles per-calendar toggle)
		if s.cursor < len(s.cfg.Calendar.Calendars) {
			cal := &s.cfg.Calendar.Calendars[s.cursor]
			newState := !cal.IsEnabled()
			cal.SetEnabled(newState)
			if err := config.Save(s.cfg); err != nil {
				return plugin.Action{Type: plugin.ActionFlash, Payload: "Failed to save: " + err.Error()}
			}
			label := cal.ID
			if cal.Label != "" {
				label = cal.Label
			}
			state := "enabled"
			if !newState {
				state = "disabled"
			}
			return plugin.Action{Type: plugin.ActionFlash, Payload: fmt.Sprintf("%s %s", label, state)}
		}
		// Let the parent handle space for the data source toggle
		return plugin.Action{Type: plugin.ActionUnhandled}
	case "c":
		if s.cursor < len(s.cfg.Calendar.Calendars) {
			s.colorPicking = true
			s.colorCursor = 0
			// Pre-select current color if set
			cal := s.cfg.Calendar.Calendars[s.cursor]
			if cal.Color != "" {
				for i, c := range presetColors {
					if c.Hex == cal.Color {
						s.colorCursor = i
						break
					}
				}
			}
		}
		return plugin.NoopAction()
	case "a":
		s.addMode = true
		s.addPhase = 0
		s.idInput.SetValue("")
		s.labelInput.SetValue("")
		s.idInput.Focus()
		return plugin.NoopAction()
	case "f":
		// Enter fetch/browse mode — fetch from Google or browse already-fetched calendars
		if len(s.fetchedCalendars) > 0 {
			s.fetchMode = true
			s.fetchCursor = 0
			return plugin.NoopAction()
		}
		// Trigger a fresh fetch
		credResult := ValidateCalendarResult()
		if credResult.Status != "ok" {
			s.log("fetch skipped: credentials not configured (status=%s)", credResult.Status)
			return plugin.Action{Type: plugin.ActionFlash, Payload: "Calendar credentials not configured"}
		}
		s.fetchLoading = true
		s.fetchError = ""
		s.log("fetch started (manual)")
		cmd := func() tea.Msg {
			cals, err := ListAvailableCalendars()
			return CalendarFetchResultMsg{Calendars: cals, Err: err}
		}
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}

	case "x", "d":
		if s.cursor < len(s.cfg.Calendar.Calendars) {
			removed := s.cfg.Calendar.Calendars[s.cursor]
			s.cfg.Calendar.Calendars = append(
				s.cfg.Calendar.Calendars[:s.cursor],
				s.cfg.Calendar.Calendars[s.cursor+1:]...,
			)
			if s.cursor >= len(s.cfg.Calendar.Calendars) && s.cursor > 0 {
				s.cursor--
			}
			config.Save(s.cfg)
			label := removed.ID
			if removed.Label != "" {
				label = removed.Label
			}
			return plugin.Action{Type: plugin.ActionFlash, Payload: "Removed: " + label}
		}
		return plugin.NoopAction()
	}

	return plugin.Action{Type: plugin.ActionUnhandled}
}

func (s *Settings) handleColorPickerKey(msg tea.KeyMsg) plugin.Action {
	totalOptions := len(presetColors) + 1 // +1 for "none"

	switch msg.String() {
	case "up", "k":
		if s.colorCursor > 0 {
			s.colorCursor--
		}
		return plugin.NoopAction()
	case "down", "j":
		if s.colorCursor < totalOptions-1 {
			s.colorCursor++
		}
		return plugin.NoopAction()
	case "enter":
		if s.cursor < len(s.cfg.Calendar.Calendars) {
			cal := &s.cfg.Calendar.Calendars[s.cursor]
			if s.colorCursor < len(presetColors) {
				cal.Color = presetColors[s.colorCursor].Hex
			} else {
				cal.Color = "" // "none" selected
			}
			config.Save(s.cfg)
			s.colorPicking = false
			label := cal.ID
			if cal.Label != "" {
				label = cal.Label
			}
			if cal.Color != "" {
				return plugin.Action{Type: plugin.ActionFlash, Payload: fmt.Sprintf("%s color set", label)}
			}
			return plugin.Action{Type: plugin.ActionFlash, Payload: fmt.Sprintf("%s color removed", label)}
		}
		s.colorPicking = false
		return plugin.NoopAction()
	case "esc":
		s.colorPicking = false
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

func (s *Settings) SettingsOpenCmd() tea.Cmd {
	// Auto-fetch available calendars when credentials are configured.
	credResult := ValidateCalendarResult()
	if credResult.Status != "ok" {
		return nil
	}
	s.fetchLoading = true
	s.fetchError = ""
	s.log("fetch started (auto on pane open)")
	return func() tea.Msg {
		cals, err := ListAvailableCalendars()
		return CalendarFetchResultMsg{Calendars: cals, Err: err}
	}
}

func (s *Settings) HandleSettingsMsg(msg tea.Msg) (bool, plugin.Action) {
	if result, ok := msg.(CalendarFetchResultMsg); ok {
		s.fetchLoading = false
		if result.Err != nil {
			s.fetchError = result.Err.Error()
			s.log("fetch failed: %s", result.Err)
		} else {
			s.fetchedCalendars = result.Calendars
			s.fetchError = ""
			// Auto-enter browse mode when calendars arrive
			s.fetchMode = true
			s.fetchCursor = 0
			s.log("fetch succeeded: %d calendars", len(result.Calendars))
		}
		return true, plugin.NoopAction()
	}
	return false, plugin.NoopAction()
}

// log writes an info message to the plugin logger if available.
func (s *Settings) log(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Info("calendar", fmt.Sprintf(format, args...))
	}
}

// isCalendarConfigured returns true if the given calendar ID is already in the config.
func (s *Settings) isCalendarConfigured(id string) bool {
	for _, cal := range s.cfg.Calendar.Calendars {
		if cal.ID == id {
			return true
		}
	}
	return false
}

func (s *Settings) viewFetchMode() []string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, s.styles.header.Render("  GOOGLE CALENDARS"))

	if len(s.fetchedCalendars) == 0 {
		lines = append(lines, s.styles.muted.Render("  No calendars found"))
	} else {
		for i, cal := range s.fetchedCalendars {
			cursor := "  "
			if i == s.fetchCursor {
				cursor = s.styles.pointer.Render("> ")
			}

			configured := s.isCalendarConfigured(cal.ID)
			toggle := s.styles.disabled.Render("[ ] ")
			if configured {
				toggle = s.styles.enabled.Render("[+] ")
			}

			label := cal.Summary
			if label == "" {
				label = cal.ID
			}
			if cal.Primary {
				label += s.styles.muted.Render(" (primary)")
			}

			nameStyle := s.styles.itemName
			if configured {
				nameStyle = s.styles.enabled
			}

			lines = append(lines, fmt.Sprintf("  %s%s%s", cursor, toggle, nameStyle.Render(label)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, s.styles.muted.Render("  ↑↓ navigate · space toggle · esc back"))

	return lines
}

func (s *Settings) handleFetchKey(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "up", "k":
		if s.fetchCursor > 0 {
			s.fetchCursor--
		}
		return plugin.NoopAction()
	case "down", "j":
		if s.fetchCursor < len(s.fetchedCalendars)-1 {
			s.fetchCursor++
		}
		return plugin.NoopAction()
	case " ", "enter":
		if s.fetchCursor >= len(s.fetchedCalendars) {
			return plugin.NoopAction()
		}
		cal := s.fetchedCalendars[s.fetchCursor]
		if s.isCalendarConfigured(cal.ID) {
			// Remove from config
			out := s.cfg.Calendar.Calendars[:0]
			for _, c := range s.cfg.Calendar.Calendars {
				if c.ID != cal.ID {
					out = append(out, c)
				}
			}
			s.cfg.Calendar.Calendars = out
			if s.cursor >= len(s.cfg.Calendar.Calendars) && s.cursor > 0 {
				s.cursor = len(s.cfg.Calendar.Calendars) - 1
			}
			config.Save(s.cfg)
			label := cal.Summary
			if label == "" {
				label = cal.ID
			}
			return plugin.Action{Type: plugin.ActionFlash, Payload: "Removed: " + label}
		}
		// Add to config
		label := cal.Summary
		if label == "" {
			label = cal.ID
		}
		s.cfg.Calendar.Calendars = append(s.cfg.Calendar.Calendars, config.CalendarEntry{
			ID:    cal.ID,
			Label: label,
		})
		config.Save(s.cfg)
		return plugin.Action{Type: plugin.ActionFlash, Payload: "Added: " + label}
	case "esc":
		s.fetchMode = false
		return plugin.NoopAction()
	}
	return plugin.NoopAction()
}

func (s *Settings) handleAddKey(msg tea.KeyMsg) plugin.Action {
	switch msg.Type {
	case tea.KeyEsc:
		s.addMode = false
		s.idInput.Blur()
		s.labelInput.Blur()
		return plugin.NoopAction()
	case tea.KeyEnter:
		if s.addPhase == 0 {
			val := strings.TrimSpace(s.idInput.Value())
			if val == "" {
				return plugin.NoopAction()
			}
			s.addPhase = 1
			s.idInput.Blur()
			s.labelInput.Focus()
			return plugin.NoopAction()
		}
		// Phase 1: done entering label
		id := strings.TrimSpace(s.idInput.Value())
		label := strings.TrimSpace(s.labelInput.Value())
		if label == "" {
			label = id
		}
		s.cfg.Calendar.Calendars = append(s.cfg.Calendar.Calendars, config.CalendarEntry{
			ID:    id,
			Label: label,
		})
		s.cursor = len(s.cfg.Calendar.Calendars) - 1
		s.addMode = false
		s.idInput.Blur()
		s.labelInput.Blur()
		config.Save(s.cfg)
		return plugin.Action{Type: plugin.ActionFlash, Payload: "Added: " + label}
	default:
		if s.addPhase == 0 {
			s.idInput, _ = s.idInput.Update(msg)
		} else {
			s.labelInput, _ = s.labelInput.Update(msg)
		}
		return plugin.NoopAction()
	}
}
