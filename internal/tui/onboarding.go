package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/refresh/sources/calendar"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// onboardingStep tracks which screen the user is on.
type onboardingStep int

const (
	stepWelcome      onboardingStep = iota
	stepPalette
	stepSources      // hub screen
	stepSourceDetail // per-source sub-flow
	stepDone
)

// sourceItem represents a data source in the onboarding hub.
type sourceItem struct {
	name    string
	slug    string // "calendar", "github", "granola"
	valid   bool
	enabled bool
	hint    string // error message from validation
}

// onboardingState holds all state for the onboarding flow.
type onboardingState struct {
	step          onboardingStep
	nameInput     textinput.Model
	subtitleInput textinput.Model
	activeField   int // 0=name, 1=subtitle
	paletteCursor int
	sources       []sourceItem
	sourceCursor  int

	// Per-source detail state
	activeSource  string // slug of source being configured
	calendarState *calendarSetupState
	githubState   *githubSetupState

	// Done step
	saved       bool
	mcpBuilding bool
	mcpResult   string
	mcpServers  []string
	mcpSpinner  spinner.Model
}

// calendarColorPresets are the available colors for per-calendar color coding.
var calendarColorPresets = []struct {
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

type calendarSetupState struct {
	cursor   int
	addMode  bool
	editMode bool
	idInput  textinput.Model
	labelInput textinput.Model
	phase    int // 0=id, 1=label (during add)
	fetching bool
	fetchErr string

	// Color picker mode.
	colorPicking bool
	colorCursor  int

	// Selection mode after fetch.
	selectMode       bool
	fetchedCalendars []calendar.CalendarInfo
	selectCursor     int
	selectChecked    []bool
}

type githubSetupState struct {
	cursor          int
	repoInput       textinput.Model
	repoEditing     bool
	usernameInput   textinput.Model
	usernameEditing bool
}

// calendarListMsg carries the result of ListAvailableCalendars.
type calendarListMsg struct {
	calendars []calendar.CalendarInfo
	err       error
}

// githubUsernameMsg carries the result of auto-fetching the GitHub username.
type githubUsernameMsg struct {
	username string
	err      error
}

// mcpResultMsg carries the result of BuildAndConfigureMCP.
type mcpResultMsg struct {
	servers []string
	err     error
}

// newOnboardingState initializes onboarding state from the current config.
func newOnboardingState(cfg *config.Config) *onboardingState {
	ni := textinput.New()
	ni.Placeholder = "Claude Command"
	ni.CharLimit = 20
	ni.Width = 20
	ni.SetValue(cfg.Name)
	ni.Focus()

	si := textinput.New()
	si.Placeholder = "Center"
	si.CharLimit = 30
	si.Width = 30
	si.SetValue(cfg.Subtitle)

	s := spinner.New()
	s.Spinner = spinner.Dot

	// Set palette cursor to current palette.
	palCursor := 0
	for i, name := range config.PaletteNames() {
		if name == cfg.Palette {
			palCursor = i
			break
		}
	}

	// Initialize calendar setup state.
	calIDInput := textinput.New()
	calIDInput.Placeholder = "calendar-id@group.calendar.google.com"
	calIDInput.CharLimit = 120
	calIDInput.Width = 50

	calLabelInput := textinput.New()
	calLabelInput.Placeholder = "Work"
	calLabelInput.CharLimit = 30
	calLabelInput.Width = 20

	// Initialize github setup state.
	repoInput := textinput.New()
	repoInput.Placeholder = "owner/repo"
	repoInput.CharLimit = 80
	repoInput.Width = 40

	usernameInput := textinput.New()
	usernameInput.Placeholder = "github-username"
	usernameInput.CharLimit = 40
	usernameInput.Width = 30
	usernameInput.SetValue(cfg.GitHub.Username)

	return &onboardingState{
		step:          stepWelcome,
		nameInput:     ni,
		subtitleInput: si,
		paletteCursor: palCursor,
		sources: []sourceItem{
			{name: "Google Calendar", slug: "calendar", enabled: cfg.Calendar.Enabled},
			{name: "GitHub", slug: "github", enabled: cfg.GitHub.Enabled},
			{name: "Granola", slug: "granola", enabled: cfg.Granola.Enabled},
		},
		calendarState: &calendarSetupState{
			idInput:    calIDInput,
			labelInput: calLabelInput,
		},
		githubState: &githubSetupState{
			repoInput:     repoInput,
			usernameInput: usernameInput,
		},
		mcpSpinner: s,
	}
}

// validateSources checks credentials for each source and updates status.
func (o *onboardingState) validateSources() {
	for i := range o.sources {
		var err error
		switch o.sources[i].slug {
		case "calendar":
			err = config.ValidateCalendar()
		case "github":
			err = config.ValidateGitHub()
		case "granola":
			err = config.ValidateGranola()
		}
		if err != nil {
			o.sources[i].valid = false
			o.sources[i].hint = err.Error()
		} else {
			o.sources[i].valid = true
			o.sources[i].hint = ""
		}
	}
}

// autoEnableSources enables sources that have valid credentials.
// It only promotes sources from disabled to enabled — it never disables
// a source that was already enabled in the config.
func (o *onboardingState) autoEnableSources() {
	for i := range o.sources {
		if o.sources[i].valid && !o.sources[i].enabled {
			o.sources[i].enabled = true
		}
	}
}

// totalSourceItems returns the count of sources + the "Continue" button.
func (o *onboardingState) totalSourceItems() int {
	return len(o.sources) + 1 // +1 for "Continue →"
}

// updateOnboarding handles all messages during the onboarding flow.
func (m *Model) updateOnboarding(msg tea.Msg) (tea.Model, tea.Cmd) {
	o := m.onboardingState

	switch msg := msg.(type) {
	case ui.TickMsg:
		m.frame++
		return m, ui.TickCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if o.mcpBuilding {
			var cmd tea.Cmd
			o.mcpSpinner, cmd = o.mcpSpinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case calendarListMsg:
		cs := o.calendarState
		cs.fetching = false
		if msg.err != nil {
			cs.fetchErr = msg.err.Error()
		} else {
			cs.fetchErr = ""
			// Filter out calendars already configured.
			existing := map[string]bool{}
			for _, c := range m.cfg.Calendar.Calendars {
				existing[c.ID] = true
			}
			var available []calendar.CalendarInfo
			for _, cal := range msg.calendars {
				if !existing[cal.ID] {
					available = append(available, cal)
				}
			}
			if len(available) > 0 {
				// Enter selection mode.
				cs.fetchedCalendars = available
				cs.selectMode = true
				cs.selectCursor = 0
				cs.selectChecked = make([]bool, len(available))
				// Auto-check primary calendar.
				for i, cal := range available {
					if cal.Primary {
						cs.selectChecked[i] = true
					}
				}
			}
		}
		return m, nil

	case githubUsernameMsg:
		if msg.err == nil && msg.username != "" {
			m.cfg.GitHub.Username = msg.username
			o.githubState.usernameInput.SetValue(msg.username)
			m.saveConfig()
		}
		return m, nil

	case mcpResultMsg:
		o.mcpBuilding = false
		if msg.err != nil {
			o.mcpResult = msg.err.Error()
		} else {
			o.mcpServers = msg.servers
			o.mcpResult = fmt.Sprintf("configured %d server(s)", len(msg.servers))
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleOnboardingKey(msg)
	}

	return m, nil
}

// handleOnboardingKey routes key messages to the current step handler.
func (m *Model) handleOnboardingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState

	// ctrl+c always quits.
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch o.step {
	case stepWelcome:
		return m.handleWelcomeKey(msg)
	case stepPalette:
		return m.handlePaletteKey(msg)
	case stepSources:
		return m.handleSourcesKey(msg)
	case stepSourceDetail:
		return m.handleSourceDetailKey(msg)
	case stepDone:
		return m.handleDoneKey(msg)
	}
	return m, nil
}

// --- Step 1: Welcome + Name ---

func (m *Model) handleWelcomeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState

	switch msg.Type {
	case tea.KeyEnter:
		if o.activeField == 0 {
			// Move from banner title to subtitle field.
			o.activeField = 1
			o.nameInput.Blur()
			o.subtitleInput.Focus()
			return m, textinput.Blink
		}
		// Confirm both fields and advance.
		val := strings.TrimSpace(o.nameInput.Value())
		if val == "" {
			val = "Claude Command"
		}
		m.cfg.Name = val
		m.cfg.Subtitle = strings.TrimSpace(o.subtitleInput.Value())
		o.step = stepPalette
		o.nameInput.Blur()
		o.subtitleInput.Blur()
		return m, nil
	case tea.KeyTab, tea.KeyShiftTab:
		// Switch between the two fields.
		if o.activeField == 0 {
			o.activeField = 1
			o.nameInput.Blur()
			o.subtitleInput.Focus()
		} else {
			o.activeField = 0
			o.subtitleInput.Blur()
			o.nameInput.Focus()
		}
		return m, textinput.Blink
	case tea.KeyEsc:
		if o.activeField == 1 {
			// Go back to name field.
			o.activeField = 0
			o.subtitleInput.Blur()
			o.nameInput.Focus()
			return m, textinput.Blink
		}
		return m, tea.Quit
	default:
		// Toggle banner visibility with ctrl+b.
		if msg.String() == "ctrl+b" {
			m.cfg.SetShowBanner(!m.cfg.BannerVisible())
			return m, nil
		}
		var cmd tea.Cmd
		if o.activeField == 0 {
			o.nameInput, cmd = o.nameInput.Update(msg)
			m.cfg.Name = o.nameInput.Value()
		} else {
			o.subtitleInput, cmd = o.subtitleInput.Update(msg)
			m.cfg.Subtitle = o.subtitleInput.Value()
		}
		return m, cmd
	}
}

// --- Step 2: Palette ---

func (m *Model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState
	names := config.PaletteNames()

	switch msg.String() {
	case "up", "k":
		if o.paletteCursor > 0 {
			o.paletteCursor--
			m.applyPalettePreview(names[o.paletteCursor])
		}
	case "down", "j":
		if o.paletteCursor < len(names)-1 {
			o.paletteCursor++
			m.applyPalettePreview(names[o.paletteCursor])
		}
	case "enter":
		m.cfg.Palette = names[o.paletteCursor]
		m.applyPalettePreview(names[o.paletteCursor])
		o.step = stepSources
		o.validateSources()
		o.autoEnableSources()
	case "esc":
		o.step = stepWelcome
		o.nameInput.Focus()
	}
	return m, nil
}

// applyPalettePreview rebuilds styles and gradient from the given palette name.
func (m *Model) applyPalettePreview(name string) {
	pal := config.GetPalette(name, m.cfg.Colors)
	m.styles = NewStyles(pal)
	m.grad = NewGradientColors(pal)
}

// --- Step 3: Data Sources hub ---

func (m *Model) handleSourcesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState
	total := o.totalSourceItems()

	switch msg.Type {
	case tea.KeyTab:
		// Hotkey to continue without scrolling to the button.
		m.applySourceSelections()
		m.saveConfig()
		o.step = stepDone
		return m.enterDoneStep()
	default:
		switch msg.String() {
		case "up", "k":
			if o.sourceCursor > 0 {
				o.sourceCursor--
			}
		case "down", "j":
			if o.sourceCursor < total-1 {
				o.sourceCursor++
			}
		case " ":
			// Toggle enable/disable (only on source items, not Continue).
			if o.sourceCursor < len(o.sources) {
				o.sources[o.sourceCursor].enabled = !o.sources[o.sourceCursor].enabled
				m.applySourceSelections()
				m.saveConfig()
			}
		case "enter":
			if o.sourceCursor == len(o.sources) {
				// "Continue →" selected — apply enabled sources to config and advance.
				m.applySourceSelections()
				m.saveConfig()
				o.step = stepDone
				return m.enterDoneStep()
			}
			// Open per-source sub-flow.
			o.activeSource = o.sources[o.sourceCursor].slug
			o.step = stepSourceDetail
			// Auto-fetch GitHub username when entering the GitHub sub-flow.
			if o.activeSource == "github" {
				return m, m.maybeAutoFetchGitHubUsername()
			}
			return m, nil
		case "esc":
			o.step = stepPalette
		}
	}
	return m, nil
}

// applySourceSelections writes the enabled state into cfg.
func (m *Model) applySourceSelections() {
	o := m.onboardingState
	for _, src := range o.sources {
		switch src.slug {
		case "calendar":
			m.cfg.Calendar.Enabled = src.enabled
		case "github":
			m.cfg.GitHub.Enabled = src.enabled
		case "granola":
			m.cfg.Granola.Enabled = src.enabled
		}
	}
}

// --- Step 3b: Per-source detail ---

func (m *Model) handleSourceDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState

	switch o.activeSource {
	case "calendar":
		return m.handleCalendarDetailKey(msg)
	case "github":
		return m.handleGithubDetailKey(msg)
	case "granola":
		return m.handleGranolaDetailKey(msg)
	}

	// Fallback: esc returns to hub.
	if msg.String() == "esc" {
		o.step = stepSources
	}
	return m, nil
}

// --- Calendar sub-flow ---

func (m *Model) handleCalendarDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState
	cs := o.calendarState

	// If in color picker mode, handle color selection.
	if cs.colorPicking {
		return m.handleCalendarColorPicker(msg)
	}

	// If in selection mode (after fetch), handle selection keys.
	if cs.selectMode {
		return m.handleCalendarSelectMode(msg)
	}

	// If in add mode, handle textinput.
	if cs.addMode {
		return m.handleCalendarAddMode(msg)
	}

	// If in edit mode, handle textinput.
	if cs.editMode {
		return m.handleCalendarEditMode(msg)
	}

	switch msg.String() {
	case "esc":
		o.step = stepSources
	case "up", "k":
		if cs.cursor > 0 {
			cs.cursor--
		}
	case "down", "j":
		if cs.cursor < len(m.cfg.Calendar.Calendars)-1 {
			cs.cursor++
		}
	case "a":
		cs.addMode = true
		cs.phase = 0
		cs.idInput.SetValue("")
		cs.labelInput.SetValue("")
		cs.idInput.Focus()
		return m, textinput.Blink
	case "x":
		if len(m.cfg.Calendar.Calendars) > 0 && cs.cursor < len(m.cfg.Calendar.Calendars) {
			m.cfg.Calendar.Calendars = append(
				m.cfg.Calendar.Calendars[:cs.cursor],
				m.cfg.Calendar.Calendars[cs.cursor+1:]...,
			)
			if cs.cursor >= len(m.cfg.Calendar.Calendars) && cs.cursor > 0 {
				cs.cursor--
			}
			m.saveConfig()
		}
	case "e":
		if len(m.cfg.Calendar.Calendars) > 0 && cs.cursor < len(m.cfg.Calendar.Calendars) {
			cs.editMode = true
			cs.labelInput.SetValue(m.cfg.Calendar.Calendars[cs.cursor].Label)
			cs.labelInput.Focus()
			return m, textinput.Blink
		}
	case "c":
		// Open color picker for selected calendar.
		if len(m.cfg.Calendar.Calendars) > 0 && cs.cursor < len(m.cfg.Calendar.Calendars) {
			cs.colorPicking = true
			cs.colorCursor = 0
			// Pre-select current color if set.
			cal := m.cfg.Calendar.Calendars[cs.cursor]
			if cal.Color != "" {
				for i, c := range calendarColorPresets {
					if c.Hex == cal.Color {
						cs.colorCursor = i
						break
					}
				}
			}
		}
	case "r":
		// Re-check credentials.
		o.validateSources()
	case "f":
		// Fetch calendars from Google.
		cs.fetching = true
		cs.fetchErr = ""
		return m, func() tea.Msg {
			cals, err := calendar.ListAvailableCalendars()
			return calendarListMsg{calendars: cals, err: err}
		}
	}
	return m, nil
}

func (m *Model) handleCalendarColorPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cs := m.onboardingState.calendarState
	totalOptions := len(calendarColorPresets) + 1 // +1 for "none"

	switch msg.String() {
	case "esc":
		cs.colorPicking = false
	case "up", "k":
		if cs.colorCursor > 0 {
			cs.colorCursor--
		}
	case "down", "j":
		if cs.colorCursor < totalOptions-1 {
			cs.colorCursor++
		}
	case "enter":
		if cs.cursor < len(m.cfg.Calendar.Calendars) {
			if cs.colorCursor < len(calendarColorPresets) {
				m.cfg.Calendar.Calendars[cs.cursor].Color = calendarColorPresets[cs.colorCursor].Hex
			} else {
				m.cfg.Calendar.Calendars[cs.cursor].Color = ""
			}
			m.saveConfig()
		}
		cs.colorPicking = false
	}
	return m, nil
}

func (m *Model) handleCalendarSelectMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cs := m.onboardingState.calendarState

	switch msg.String() {
	case "esc":
		// Cancel selection, discard fetched calendars.
		cs.selectMode = false
		cs.fetchedCalendars = nil
		cs.selectChecked = nil
		return m, nil
	case "up", "k":
		if cs.selectCursor > 0 {
			cs.selectCursor--
		}
	case "down", "j":
		if cs.selectCursor < len(cs.fetchedCalendars)-1 {
			cs.selectCursor++
		}
	case " ":
		if cs.selectCursor < len(cs.selectChecked) {
			cs.selectChecked[cs.selectCursor] = !cs.selectChecked[cs.selectCursor]
		}
	case "enter":
		// Add selected calendars to config.
		for i, cal := range cs.fetchedCalendars {
			if cs.selectChecked[i] {
				label := cal.Summary
				if cal.Primary {
					label = cal.Summary + " (primary)"
				}
				m.cfg.Calendar.Calendars = append(m.cfg.Calendar.Calendars, config.CalendarEntry{
					ID:    cal.ID,
					Label: label,
				})
			}
		}
		m.saveConfig()
		cs.selectMode = false
		cs.fetchedCalendars = nil
		cs.selectChecked = nil
		return m, nil
	}
	return m, nil
}

func (m *Model) handleCalendarAddMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cs := m.onboardingState.calendarState

	switch msg.Type {
	case tea.KeyEsc:
		cs.addMode = false
		cs.idInput.Blur()
		cs.labelInput.Blur()
		return m, nil
	case tea.KeyEnter:
		if cs.phase == 0 {
			// Done entering ID, move to label.
			val := strings.TrimSpace(cs.idInput.Value())
			if val == "" {
				return m, nil
			}
			cs.phase = 1
			cs.idInput.Blur()
			cs.labelInput.Focus()
			return m, textinput.Blink
		}
		// Phase 1: done entering label.
		id := strings.TrimSpace(cs.idInput.Value())
		label := strings.TrimSpace(cs.labelInput.Value())
		if label == "" {
			label = id
		}
		m.cfg.Calendar.Calendars = append(m.cfg.Calendar.Calendars, config.CalendarEntry{
			ID:    id,
			Label: label,
		})
		cs.addMode = false
		cs.idInput.Blur()
		cs.labelInput.Blur()
		cs.cursor = len(m.cfg.Calendar.Calendars) - 1
		m.saveConfig()
		return m, nil
	default:
		var cmd tea.Cmd
		if cs.phase == 0 {
			cs.idInput, cmd = cs.idInput.Update(msg)
		} else {
			cs.labelInput, cmd = cs.labelInput.Update(msg)
		}
		return m, cmd
	}
}

func (m *Model) handleCalendarEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cs := m.onboardingState.calendarState

	switch msg.Type {
	case tea.KeyEsc:
		cs.editMode = false
		cs.labelInput.Blur()
		return m, nil
	case tea.KeyEnter:
		label := strings.TrimSpace(cs.labelInput.Value())
		if label != "" && cs.cursor < len(m.cfg.Calendar.Calendars) {
			m.cfg.Calendar.Calendars[cs.cursor].Label = label
			m.saveConfig()
		}
		cs.editMode = false
		cs.labelInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		cs.labelInput, cmd = cs.labelInput.Update(msg)
		return m, cmd
	}
}

// --- GitHub sub-flow ---

func (m *Model) handleGithubDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState
	gs := o.githubState

	// If editing repo input.
	if gs.repoEditing {
		switch msg.Type {
		case tea.KeyEsc:
			gs.repoEditing = false
			gs.repoInput.Blur()
			return m, nil
		case tea.KeyEnter:
			val := strings.TrimSpace(gs.repoInput.Value())
			if val != "" {
				m.cfg.GitHub.Repos = append(m.cfg.GitHub.Repos, val)
				m.saveConfig()
			}
			gs.repoEditing = false
			gs.repoInput.Blur()
			gs.repoInput.SetValue("")
			return m, nil
		default:
			var cmd tea.Cmd
			gs.repoInput, cmd = gs.repoInput.Update(msg)
			return m, cmd
		}
	}

	// If editing username input.
	if gs.usernameEditing {
		switch msg.Type {
		case tea.KeyEsc:
			gs.usernameEditing = false
			gs.usernameInput.Blur()
			return m, nil
		case tea.KeyEnter:
			val := strings.TrimSpace(gs.usernameInput.Value())
			if val != "" {
				m.cfg.GitHub.Username = val
				m.saveConfig()
			}
			gs.usernameEditing = false
			gs.usernameInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			gs.usernameInput, cmd = gs.usernameInput.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "esc":
		o.step = stepSources
	case "up", "k":
		if gs.cursor > 0 {
			gs.cursor--
		}
	case "down", "j":
		if gs.cursor < len(m.cfg.GitHub.Repos)-1 {
			gs.cursor++
		}
	case "a":
		gs.repoEditing = true
		gs.repoInput.SetValue("")
		gs.repoInput.Focus()
		return m, textinput.Blink
	case "x":
		if len(m.cfg.GitHub.Repos) > 0 && gs.cursor < len(m.cfg.GitHub.Repos) {
			m.cfg.GitHub.Repos = append(
				m.cfg.GitHub.Repos[:gs.cursor],
				m.cfg.GitHub.Repos[gs.cursor+1:]...,
			)
			if gs.cursor >= len(m.cfg.GitHub.Repos) && gs.cursor > 0 {
				gs.cursor--
			}
			m.saveConfig()
		}
	case "u":
		gs.usernameEditing = true
		gs.usernameInput.Focus()
		return m, textinput.Blink
	case "r":
		o.validateSources()
		// Auto-fetch username if auth is valid and username is empty.
		return m, m.maybeAutoFetchGitHubUsername()
	}
	return m, nil
}

// maybeAutoFetchGitHubUsername returns a Cmd that fetches the GitHub username
// via `gh api user` if auth is valid and the username is not yet set.
func (m *Model) maybeAutoFetchGitHubUsername() tea.Cmd {
	ghSrc := m.onboardingState.findSource("github")
	if ghSrc == nil || !ghSrc.valid || m.cfg.GitHub.Username != "" {
		return nil
	}
	return func() tea.Msg {
		out, err := exec.Command("gh", "api", "user", "-q", ".login").Output()
		if err != nil {
			return githubUsernameMsg{err: err}
		}
		return githubUsernameMsg{username: strings.TrimSpace(string(out))}
	}
}

// --- Granola sub-flow ---

func (m *Model) handleGranolaDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState

	switch msg.String() {
	case "esc":
		o.step = stepSources
	case "r":
		o.validateSources()
	}
	return m, nil
}

// --- Step 4: Done ---

func (m *Model) enterDoneStep() (tea.Model, tea.Cmd) {
	o := m.onboardingState
	o.saved = false
	o.mcpBuilding = true
	o.mcpResult = ""
	o.mcpServers = nil

	// Save config synchronously.
	if err := config.Save(m.cfg); err != nil {
		o.mcpResult = "config save failed: " + err.Error()
		o.mcpBuilding = false
		o.saved = false
	} else {
		o.saved = true
	}

	// Fire MCP build in background.
	return m, tea.Batch(
		o.mcpSpinner.Tick,
		func() tea.Msg {
			servers, err := config.BuildAndConfigureMCP()
			return mcpResultMsg{servers: servers, err: err}
		},
	)
}

func (m *Model) handleDoneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := m.onboardingState

	switch msg.String() {
	case "enter":
		// Complete onboarding regardless of MCP status.
		m.onboarding = false
		return m, m.deferredPluginInit()
	case "esc":
		o.step = stepSources
		o.mcpBuilding = false
	}
	return m, nil
}

// deferredPluginInit returns a batch of StartCmds + Refresh for all plugins.
func (m *Model) deferredPluginInit() tea.Cmd {
	var cmds []tea.Cmd

	seen := map[string]bool{}
	for _, p := range m.allPlugins {
		if seen[p.Slug()] {
			continue
		}
		seen[p.Slug()] = true
		if starter, ok := p.(interface{ StartCmds() tea.Cmd }); ok {
			if cmd := starter.StartCmds(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	// Initial data load for sessions.
	if m.db != nil {
		for _, p := range m.allPlugins {
			if p.RefreshInterval() == 0 && p.Slug() == "sessions" {
				cmds = append(cmds, p.Refresh())
			}
		}
	}

	return tea.Batch(cmds...)
}

// ==================== View functions ====================

// onboardingView renders the full onboarding screen.
func (o *onboardingState) view(width, height int, styles *Styles, grad *GradientColors, cfg *config.Config, frame int) string {
	viewWidth := ui.ContentMaxWidth
	if width > 0 && width < viewWidth {
		viewWidth = width
	}

	var content string
	switch o.step {
	case stepWelcome:
		content = o.viewWelcome(viewWidth, styles, cfg)
	case stepPalette:
		content = o.viewPalette(viewWidth, styles)
	case stepSources:
		content = o.viewSources(viewWidth, styles)
	case stepSourceDetail:
		content = o.viewSourceDetail(viewWidth, styles, cfg)
	case stepDone:
		content = o.viewDone(viewWidth, styles)
	}

	return content
}

func (o *onboardingState) viewWelcome(width int, styles *Styles, cfg *config.Config) string {
	var lines []string

	lines = append(lines, styles.TitleBoldC.Render("WELCOME"))
	lines = append(lines, "")

	// Banner title field.
	nameLabel := styles.DescMuted.Render("Banner title:")
	if o.activeField == 0 {
		nameLabel = styles.TitleBoldW.Render("Banner title:")
	}
	lines = append(lines, "  "+nameLabel)
	lines = append(lines, "  "+o.nameInput.View())
	lines = append(lines, styles.Hint.Render("    Rendered as large block letters above"))
	lines = append(lines, "")

	// Subtitle field.
	subLabel := styles.DescMuted.Render("Subtitle:")
	if o.activeField == 1 {
		subLabel = styles.TitleBoldW.Render("Subtitle:")
	}
	lines = append(lines, "  "+subLabel)
	lines = append(lines, "  "+o.subtitleInput.View())
	lines = append(lines, styles.Hint.Render("    Spaced text below the banner (leave empty to hide)"))
	lines = append(lines, "")

	bannerStatus := "on"
	if !cfg.BannerVisible() {
		bannerStatus = "off"
	}
	lines = append(lines, styles.Hint.Render(fmt.Sprintf("  Show banner: [%s]  (ctrl+b to toggle)", bannerStatus)))
	lines = append(lines, "")
	lines = append(lines, styles.Hint.Render("  tab switch field · enter next/continue · ctrl+b toggle banner · esc quit"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return styles.PanelBorder.Width(width - 4).Render(content)
}

func (o *onboardingState) viewPalette(width int, styles *Styles) string {
	var lines []string

	lines = append(lines, styles.TitleBoldC.Render("CHOOSE A PALETTE"))
	lines = append(lines, "")

	names := config.PaletteNames()
	for i, name := range names {
		pal := config.GetPalette(name, nil)
		cursor := "  "
		if i == o.paletteCursor {
			cursor = styles.Pointer.Render("> ")
		}

		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(pal.White))
		if i == o.paletteCursor {
			nameStyle = nameStyle.Bold(true)
		}

		swatches := renderOnboardingSwatches(pal)
		lines = append(lines, fmt.Sprintf("%s%s  %s", cursor, nameStyle.Render(name), swatches))
	}

	lines = append(lines, "")
	lines = append(lines, styles.Hint.Render("  up/down navigate · enter select · esc back"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return styles.PanelBorder.Width(width - 4).Render(content)
}

func (o *onboardingState) viewSources(width int, styles *Styles) string {
	var lines []string

	lines = append(lines, styles.TitleBoldC.Render("DATA SOURCES"))
	lines = append(lines, "")
	lines = append(lines, styles.DescMuted.Render("  Toggle sources and configure credentials."))
	lines = append(lines, "")

	for i, src := range o.sources {
		cursor := "  "
		if i == o.sourceCursor {
			cursor = styles.Pointer.Render("> ")
		}

		var status string
		if src.valid {
			status = lipgloss.NewStyle().Foreground(styles.ColorGreen).Render("✓")
		} else {
			status = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("✗")
		}

		toggle := lipgloss.NewStyle().Foreground(styles.ColorGreen).Render("[on] ")
		if !src.enabled {
			toggle = styles.DescMuted.Render("[off]")
		}

		nameStyle := styles.NormalItem
		if !src.enabled {
			nameStyle = styles.DescMuted
		}

		line := fmt.Sprintf("%s%s %s %s", cursor, status, toggle, nameStyle.Render(src.name))
		if src.hint != "" && !src.valid {
			line += "  " + styles.Hint.Render(src.hint)
		}
		lines = append(lines, line)
	}

	// "Continue →" button.
	lines = append(lines, "")
	cursor := "  "
	if o.sourceCursor == len(o.sources) {
		cursor = styles.Pointer.Render("> ")
	}
	continueStyle := styles.TitleBoldW
	if o.sourceCursor != len(o.sources) {
		continueStyle = styles.NormalItem
	}
	lines = append(lines, cursor+continueStyle.Render("Continue →"))

	lines = append(lines, "")
	lines = append(lines, styles.Hint.Render("  up/down navigate · space toggle · enter configure · tab continue · esc back"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return styles.PanelBorder.Width(width - 4).Render(content)
}

func (o *onboardingState) viewSourceDetail(width int, styles *Styles, cfg *config.Config) string {
	switch o.activeSource {
	case "calendar":
		return o.viewCalendarDetail(width, styles, cfg)
	case "github":
		return o.viewGithubDetail(width, styles, cfg)
	case "granola":
		return o.viewGranolaDetail(width, styles)
	}
	return ""
}

func (o *onboardingState) viewCalendarDetail(width int, styles *Styles, cfg *config.Config) string {
	cs := o.calendarState
	var lines []string

	lines = append(lines, styles.TitleBoldC.Render("GOOGLE CALENDAR"))
	lines = append(lines, "")

	// Credential status.
	var credStatus string
	calSrc := o.findSource("calendar")
	if calSrc != nil && calSrc.valid {
		credStatus = lipgloss.NewStyle().Foreground(styles.ColorGreen).Render("✓ credentials found")
	} else {
		credStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("✗ credentials not found")
		lines = append(lines, "  "+credStatus)
		lines = append(lines, "")
		lines = append(lines, styles.Hint.Render("  Place credentials.json in ~/.config/google-calendar-mcp/"))
		lines = append(lines, "")
		lines = append(lines, styles.Hint.Render("  r re-check · esc back"))
		content := lipgloss.JoinVertical(lipgloss.Left, lines...)
		return styles.PanelBorder.Width(width - 4).Render(content)
	}
	lines = append(lines, "  "+credStatus)
	lines = append(lines, "")

	// Fetching indicator.
	if cs.fetching {
		lines = append(lines, styles.Hint.Render("  Fetching calendars..."))
		lines = append(lines, "")
	}
	if cs.fetchErr != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("  "+cs.fetchErr))
		lines = append(lines, "")
	}

	// Selection mode after fetch.
	if cs.selectMode {
		lines = append(lines, styles.TitleBoldW.Render("  Select Calendars to Add"))
		lines = append(lines, "")
		for i, cal := range cs.fetchedCalendars {
			cursor := "    "
			if i == cs.selectCursor {
				cursor = "  " + styles.Pointer.Render("> ")
			}
			check := "[ ]"
			if cs.selectChecked[i] {
				check = "[x]"
			}
			label := cal.Summary
			if cal.Primary {
				label += " (primary)"
			}
			lines = append(lines, fmt.Sprintf("%s%s %s  %s",
				cursor,
				check,
				styles.NormalItem.Render(label),
				styles.Hint.Render(cal.ID)))
		}
		lines = append(lines, "")
		lines = append(lines, styles.Hint.Render("  space toggle · up/down navigate · enter add selected · esc cancel"))
		content := lipgloss.JoinVertical(lipgloss.Left, lines...)
		return styles.PanelBorder.Width(width - 4).Render(content)
	}

	// Add mode.
	if cs.addMode {
		lines = append(lines, styles.TitleBoldW.Render("  Add Calendar"))
		if cs.phase == 0 {
			lines = append(lines, "  ID: "+cs.idInput.View())
		} else {
			lines = append(lines, "  ID: "+styles.NormalItem.Render(cs.idInput.Value()))
			lines = append(lines, "  Label: "+cs.labelInput.View())
		}
		lines = append(lines, "")
		lines = append(lines, styles.Hint.Render("  enter confirm · esc cancel"))
		content := lipgloss.JoinVertical(lipgloss.Left, lines...)
		return styles.PanelBorder.Width(width - 4).Render(content)
	}

	// Edit mode.
	if cs.editMode {
		lines = append(lines, styles.TitleBoldW.Render("  Edit Label"))
		lines = append(lines, "  "+cs.labelInput.View())
		lines = append(lines, "")
		lines = append(lines, styles.Hint.Render("  enter confirm · esc cancel"))
		content := lipgloss.JoinVertical(lipgloss.Left, lines...)
		return styles.PanelBorder.Width(width - 4).Render(content)
	}

	// Color picker mode.
	if cs.colorPicking && cs.cursor < len(cfg.Calendar.Calendars) {
		cal := cfg.Calendar.Calendars[cs.cursor]
		lines = append(lines, styles.TitleBoldW.Render("  Color for "+cal.Label))
		lines = append(lines, "")
		for i, c := range calendarColorPresets {
			pointer := "    "
			if i == cs.colorCursor {
				pointer = "  " + styles.Pointer.Render("> ")
			}
			swatch := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex)).Render("██")
			lines = append(lines, fmt.Sprintf("%s%s %s", pointer, swatch, c.Name))
		}
		// "None" option
		noneIdx := len(calendarColorPresets)
		pointer := "    "
		if cs.colorCursor == noneIdx {
			pointer = "  " + styles.Pointer.Render("> ")
		}
		lines = append(lines, fmt.Sprintf("%s%s", pointer, styles.Hint.Render("None")))
		lines = append(lines, "")
		lines = append(lines, styles.Hint.Render("  enter select · esc cancel"))
		content := lipgloss.JoinVertical(lipgloss.Left, lines...)
		return styles.PanelBorder.Width(width - 4).Render(content)
	}

	// Calendar list.
	lines = append(lines, styles.TitleBoldW.Render("  Calendars"))
	if len(cfg.Calendar.Calendars) == 0 {
		lines = append(lines, styles.Hint.Render("  No calendars configured"))
	} else {
		for i, cal := range cfg.Calendar.Calendars {
			cursor := "    "
			if i == cs.cursor {
				cursor = "  " + styles.Pointer.Render("> ")
			}
			colorSwatch := ""
			if cal.Color != "" {
				colorSwatch = lipgloss.NewStyle().Foreground(lipgloss.Color(cal.Color)).Render("●") + " "
			}
			lines = append(lines, fmt.Sprintf("%s%s%s  %s",
				cursor,
				colorSwatch,
				styles.NormalItem.Render(cal.Label),
				styles.Hint.Render(cal.ID)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, styles.Hint.Render("  a add · x remove · e edit · c color · f fetch · r re-check · esc back"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return styles.PanelBorder.Width(width - 4).Render(content)
}

func (o *onboardingState) viewGithubDetail(width int, styles *Styles, cfg *config.Config) string {
	gs := o.githubState
	var lines []string

	lines = append(lines, styles.TitleBoldC.Render("GITHUB"))
	lines = append(lines, "")

	// Auth status.
	ghSrc := o.findSource("github")
	if ghSrc != nil && ghSrc.valid {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(styles.ColorGreen).Render("✓ gh CLI authenticated"))
	} else {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("✗ gh CLI not authenticated"))
		lines = append(lines, styles.Hint.Render("  Run 'gh auth login' to authenticate"))
	}
	lines = append(lines, "")

	// Username.
	if gs.usernameEditing {
		lines = append(lines, "  Username: "+gs.usernameInput.View())
	} else {
		uname := cfg.GitHub.Username
		if uname == "" {
			uname = styles.Hint.Render("(not set)")
		}
		lines = append(lines, fmt.Sprintf("  %s %s", styles.DescMuted.Render("Username:"), styles.NormalItem.Render(uname)))
	}
	lines = append(lines, "")

	// Repo input (when adding).
	if gs.repoEditing {
		lines = append(lines, "  Add repo: "+gs.repoInput.View())
		lines = append(lines, "")
		lines = append(lines, styles.Hint.Render("  enter confirm · esc cancel"))
		content := lipgloss.JoinVertical(lipgloss.Left, lines...)
		return styles.PanelBorder.Width(width - 4).Render(content)
	}

	// Repo list.
	lines = append(lines, styles.TitleBoldW.Render("  Repos"))
	if len(cfg.GitHub.Repos) == 0 {
		lines = append(lines, styles.Hint.Render("  No repos configured"))
	} else {
		for i, repo := range cfg.GitHub.Repos {
			cursor := "    "
			if i == gs.cursor {
				cursor = "  " + styles.Pointer.Render("> ")
			}
			lines = append(lines, cursor+styles.NormalItem.Render(repo))
		}
	}

	lines = append(lines, "")
	lines = append(lines, styles.Hint.Render("  a add repo · x remove · u edit username · r re-check · esc back"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return styles.PanelBorder.Width(width - 4).Render(content)
}

func (o *onboardingState) viewGranolaDetail(width int, styles *Styles) string {
	var lines []string

	lines = append(lines, styles.TitleBoldC.Render("GRANOLA"))
	lines = append(lines, "")

	lines = append(lines, styles.DescMuted.Render("  Granola records and summarizes your meetings."))
	lines = append(lines, "")

	grSrc := o.findSource("granola")
	if grSrc != nil && grSrc.valid {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(styles.ColorGreen).Render("✓ Granola configured"))
	} else {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("✗ Granola not configured"))
		lines = append(lines, "")
		lines = append(lines, styles.DescMuted.Render("  To set up Granola:"))
		lines = append(lines, styles.Hint.Render("  1. Install from granola.ai"))
		lines = append(lines, styles.Hint.Render("  2. Open Granola and sign in"))
		lines = append(lines, styles.Hint.Render("  3. CCC reads Granola's local data automatically"))
		lines = append(lines, "")
		lines = append(lines, styles.Hint.Render("  Looks for: ~/Library/Application Support/Granola/stored-accounts.json"))
	}

	lines = append(lines, "")
	lines = append(lines, styles.Hint.Render("  r re-check · esc back"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return styles.PanelBorder.Width(width - 4).Render(content)
}

func (o *onboardingState) viewDone(width int, styles *Styles) string {
	var lines []string

	lines = append(lines, styles.TitleBoldC.Render("SETUP COMPLETE"))
	lines = append(lines, "")

	if o.saved {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(styles.ColorGreen).Render("✓ Configuration saved"))
	} else {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("✗ Configuration not saved"))
	}

	lines = append(lines, "")

	if o.mcpBuilding {
		lines = append(lines, "  "+o.mcpSpinner.View()+" Building MCP servers...")
	} else if len(o.mcpServers) > 0 {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(styles.ColorGreen).Render("✓ MCP servers ready: "+strings.Join(o.mcpServers, ", ")))
	} else if o.mcpResult != "" {
		lines = append(lines, "  "+styles.Hint.Render("- "+o.mcpResult))
	}

	lines = append(lines, "")
	lines = append(lines, styles.Hint.Render("  enter launch command center · esc back"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return styles.PanelBorder.Width(width - 4).Render(content)
}

// saveConfig persists the current config to disk. Errors are silently ignored
// during onboarding — the Done step will do a final save with error reporting.
func (m *Model) saveConfig() {
	_ = config.Save(m.cfg)
}

// findSource returns a pointer to the sourceItem with the given slug, or nil.
func (o *onboardingState) findSource(slug string) *sourceItem {
	for i := range o.sources {
		if o.sources[i].slug == slug {
			return &o.sources[i]
		}
	}
	return nil
}

// renderOnboardingSwatches renders color swatches for a palette.
func renderOnboardingSwatches(pal config.Palette) string {
	colors := []string{pal.Cyan, pal.Yellow, pal.Purple, pal.Green, pal.White}
	var parts []string
	for _, c := range colors {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("██"))
	}
	return strings.Join(parts, " ")
}
