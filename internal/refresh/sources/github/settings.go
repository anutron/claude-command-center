package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Settings implements plugin.SettingsProvider for the GitHub data source.
type Settings struct {
	cfg    *config.Config
	styles settingsStyles

	cursor          int
	repoInput       textinput.Model
	repoEditing     bool
	usernameInput   textinput.Model
	usernameEditing bool
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

// NewSettings creates a GitHub SettingsProvider.
func NewSettings(cfg *config.Config, pal config.Palette) *Settings {
	ri := textinput.New()
	ri.Placeholder = "owner/repo"
	ri.CharLimit = 100

	ui := textinput.New()
	ui.Placeholder = "GitHub username"
	ui.CharLimit = 50
	ui.SetValue(cfg.GitHub.Username)

	return &Settings{
		cfg:           cfg,
		styles:        newSettingsStyles(pal),
		repoInput:     ri,
		usernameInput: ui,
	}
}

// ResetEditing resets editing state when the detail view is opened.
func (s *Settings) ResetEditing() {
	s.cursor = 0
	s.repoEditing = false
	s.usernameEditing = false
	s.usernameInput.SetValue(s.cfg.GitHub.Username)
}

func (s *Settings) SettingsView(width, height int) string {
	var lines []string

	statusText := "[off]"
	statusStyle := s.styles.disabled
	if s.cfg.GitHub.Enabled {
		statusText = "[on] "
		statusStyle = s.styles.enabled
	}

	lines = append(lines, fmt.Sprintf("  %s %s",
		s.styles.muted.Render("Enabled:"),
		statusStyle.Render(statusText+" (space to toggle)")))

	checks := s.DoctorChecks(plugin.DoctorOpts{})
	credStatus := s.styles.enabled.Render("Authenticated")
	if len(checks) > 0 && checks[0].Result.Status != "ok" {
		credStatus = s.styles.logError.Render("Not authenticated")
	}
	lines = append(lines, fmt.Sprintf("  %s %s",
		s.styles.muted.Render("gh CLI:"),
		credStatus))

	// Username
	username := s.cfg.GitHub.Username
	if username == "" {
		username = "(not set)"
	}
	lines = append(lines, fmt.Sprintf("  %s %s %s",
		s.styles.muted.Render("Username:"),
		s.styles.itemName.Render(username),
		s.styles.muted.Render("(u to edit)")))
	if s.usernameEditing {
		lines = append(lines, "  "+s.usernameInput.View())
	}

	// Repos
	lines = append(lines, "")
	lines = append(lines, s.styles.header.Render("  REPOS"))
	if len(s.cfg.GitHub.Repos) == 0 {
		lines = append(lines, s.styles.muted.Render("  No repos configured"))
	} else {
		for i, repo := range s.cfg.GitHub.Repos {
			cursor := "  "
			if i == s.cursor {
				cursor = s.styles.pointer.Render("> ")
			}
			lines = append(lines, fmt.Sprintf("  %s%s", cursor, s.styles.itemName.Render(repo)))
		}
	}

	if s.repoEditing {
		lines = append(lines, "  "+s.repoInput.View())
	}

	lines = append(lines, "")
	lines = append(lines, s.styles.muted.Render("  a add repo · x remove · u edit username"))
	lines = append(lines, s.styles.muted.Render("  Run 'gh auth login' to authenticate"))

	return strings.Join(lines, "\n")
}

// ghUserFetchResult is the message returned by the async username fetch.
type ghUserFetchResult struct {
	Login string
	Err   error
}

// fetchGHUsername is a variable for testability.
var fetchGHUsername = func() (string, error) {
	out, err := exec.Command("gh", "api", "/user").Output()
	if err != nil {
		return "", err
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &user); err != nil {
		return "", fmt.Errorf("parse /user response: %w", err)
	}
	if user.Login == "" {
		return "", fmt.Errorf("empty login returned")
	}
	return user.Login, nil
}

func (s *Settings) SettingsOpenCmd() tea.Cmd {
	// Only fetch if authenticated and username is not already set.
	if s.cfg.GitHub.Username != "" {
		return nil
	}
	// Quick check: is gh authenticated?
	if err := exec.Command("gh", "auth", "token").Run(); err != nil {
		return nil
	}
	return func() tea.Msg {
		login, err := fetchGHUsername()
		return ghUserFetchResult{Login: login, Err: err}
	}
}

func (s *Settings) HandleSettingsMsg(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case ghUserFetchResult:
		if msg.Err != nil {
			return true, plugin.Action{Type: plugin.ActionFlash, Payload: "Could not auto-fetch GitHub username: " + msg.Err.Error()}
		}
		// Only set if still empty (user may have typed one in the meantime).
		if s.cfg.GitHub.Username == "" {
			s.cfg.GitHub.Username = msg.Login
			s.usernameInput.SetValue(msg.Login)
			config.Save(s.cfg)
			return true, plugin.Action{Type: plugin.ActionFlash, Payload: "GitHub username auto-detected: " + msg.Login}
		}
		return true, plugin.NoopAction()
	}
	return false, plugin.NoopAction()
}

// DoctorChecks implements plugin.DoctorProvider for GitHub.
func (s *Settings) DoctorChecks(opts plugin.DoctorOpts) []plugin.DoctorCheck {
	check := plugin.DoctorCheck{Name: "GitHub CLI"}

	cmd := exec.Command("gh", "auth", "token")
	if err := cmd.Run(); err != nil {
		check.Result = plugin.ValidationResult{
			Status:  "missing",
			Message: "GitHub CLI not authenticated",
			Hint:    "Run 'gh auth login' to authenticate",
		}
	} else {
		check.Result = plugin.ValidationResult{
			Status:  "ok",
			Message: "GitHub CLI authenticated",
		}
	}

	return []plugin.DoctorCheck{check}
}

func (s *Settings) HandleSettingsKey(msg tea.KeyMsg) plugin.Action {
	// If editing a text input, route keys there
	if s.repoEditing {
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(s.repoInput.Value())
			if val != "" {
				s.cfg.GitHub.Repos = append(s.cfg.GitHub.Repos, val)
				config.Save(s.cfg)
			}
			s.repoInput.SetValue("")
			s.repoEditing = false
			s.repoInput.Blur()
			if val != "" {
				return plugin.Action{Type: plugin.ActionFlash, Payload: "Added repo: " + val}
			}
			return plugin.NoopAction()
		case "esc":
			s.repoEditing = false
			s.repoInput.Blur()
			return plugin.NoopAction()
		}
		s.repoInput, _ = s.repoInput.Update(msg)
		return plugin.NoopAction()
	}

	if s.usernameEditing {
		switch msg.String() {
		case "enter":
			s.cfg.GitHub.Username = strings.TrimSpace(s.usernameInput.Value())
			config.Save(s.cfg)
			s.usernameEditing = false
			s.usernameInput.Blur()
			return plugin.Action{Type: plugin.ActionFlash, Payload: "Username saved"}
		case "esc":
			s.usernameEditing = false
			s.usernameInput.Blur()
			return plugin.NoopAction()
		}
		s.usernameInput, _ = s.usernameInput.Update(msg)
		return plugin.NoopAction()
	}

	switch msg.String() {
	case "a":
		s.repoEditing = true
		s.repoInput.Focus()
		return plugin.NoopAction()
	case "u":
		s.usernameEditing = true
		s.usernameInput.SetValue(s.cfg.GitHub.Username)
		s.usernameInput.Focus()
		return plugin.NoopAction()
	case "x", "d":
		if s.cursor < len(s.cfg.GitHub.Repos) {
			removed := s.cfg.GitHub.Repos[s.cursor]
			s.cfg.GitHub.Repos = append(
				s.cfg.GitHub.Repos[:s.cursor],
				s.cfg.GitHub.Repos[s.cursor+1:]...,
			)
			config.Save(s.cfg)
			if s.cursor >= len(s.cfg.GitHub.Repos) && s.cursor > 0 {
				s.cursor--
			}
			return plugin.Action{Type: plugin.ActionFlash, Payload: "Removed: " + removed}
		}
		return plugin.NoopAction()
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
		return plugin.NoopAction()
	case "down", "j":
		if s.cursor < len(s.cfg.GitHub.Repos)-1 {
			s.cursor++
		}
		return plugin.NoopAction()
	}

	return plugin.Action{Type: plugin.ActionUnhandled}
}
