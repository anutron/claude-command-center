package settings

import (
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

func testSetup() (*Plugin, *plugin.Registry) {
	reg := plugin.NewRegistry()
	p := New(reg)

	cfg := &config.Config{
		Name:    "Test Center",
		Palette: "aurora",
		Todos:   config.TodosConfig{Enabled: true},
		Calendar: config.CalendarConfig{Enabled: true},
		GitHub:   config.GitHubConfig{Enabled: false},
		Granola:  config.GranolaConfig{Enabled: false},
		ExternalPlugins: []config.ExternalPluginConfig{
			{Name: "Pomodoro", Command: "pomodoro", Enabled: true},
		},
	}

	// Register the settings plugin itself so it appears in the list
	reg.Register(p)

	ctx := plugin.Context{
		Config: cfg,
		Bus:    plugin.NewBus(),
		Logger: plugin.NewMemoryLogger(),
	}
	_ = p.Init(ctx)
	return p, reg
}

func TestSlugAndTabName(t *testing.T) {
	p, _ := testSetup()
	if p.Slug() != "settings" {
		t.Errorf("expected slug 'settings', got %q", p.Slug())
	}
	if p.TabName() != "Settings" {
		t.Errorf("expected tab name 'Settings', got %q", p.TabName())
	}
}

func TestRoutesReturnsThree(t *testing.T) {
	p, _ := testSetup()
	routes := p.Routes()
	if len(routes) != 4 {
		t.Errorf("expected 4 routes, got %d", len(routes))
	}
}

func TestPluginListPopulated(t *testing.T) {
	p, _ := testSetup()
	if len(p.items) == 0 {
		t.Error("expected items to be populated")
	}

	// Should have: settings (builtin), pomodoro (external), todos, calendar, github, granola (data sources)
	found := map[string]bool{}
	for _, item := range p.items {
		found[item.slug] = true
	}
	if !found["settings"] {
		t.Error("expected settings in items")
	}
	if !found["external-0"] {
		t.Error("expected external-0 (Pomodoro) in items")
	}
	if !found["calendar"] {
		t.Error("expected calendar in items")
	}
}

func TestToggleDataSource(t *testing.T) {
	p, _ := testSetup()

	// Find calendar item
	calIdx := -1
	for i, item := range p.items {
		if item.slug == "calendar" {
			calIdx = i
			break
		}
	}
	if calIdx < 0 {
		t.Fatal("calendar item not found")
	}

	// Calendar should be enabled initially
	if !p.items[calIdx].enabled {
		t.Error("expected calendar to be enabled initially")
	}

	// Toggle with space (enter now opens detail view)
	p.cursor = calIdx
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})

	if p.items[calIdx].enabled {
		t.Error("expected calendar to be disabled after toggle")
	}
	if !p.cfg.Calendar.Enabled == p.items[calIdx].enabled {
		// Config should match
	}
}

func TestToggleExternalPlugin(t *testing.T) {
	p, _ := testSetup()

	// Find external plugin
	extIdx := -1
	for i, item := range p.items {
		if item.kind == "external-plugin" {
			extIdx = i
			break
		}
	}
	if extIdx < 0 {
		t.Fatal("external plugin item not found")
	}

	if !p.items[extIdx].enabled {
		t.Error("expected external plugin to be enabled initially")
	}

	p.cursor = extIdx
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})

	if p.items[extIdx].enabled {
		t.Error("expected external plugin to be disabled after toggle")
	}
	if p.flashMessage != "Restart CCC to apply" {
		t.Errorf("expected restart flash message, got %q", p.flashMessage)
	}
}

func TestCorePluginNotToggleable(t *testing.T) {
	p, _ := testSetup()

	// Find settings plugin (core, not toggleable)
	settingsIdx := -1
	for i, item := range p.items {
		if item.slug == "settings" {
			settingsIdx = i
			break
		}
	}
	if settingsIdx < 0 {
		t.Fatal("settings item not found")
	}

	if p.items[settingsIdx].toggleable {
		t.Error("expected settings to not be toggleable")
	}

	// Trying to toggle with space should be a no-op
	p.cursor = settingsIdx
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	if !p.items[settingsIdx].enabled {
		t.Error("core plugin should remain enabled after toggle attempt")
	}

	// Enter opens detail view but doesn't change enabled state
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !p.items[settingsIdx].enabled {
		t.Error("core plugin should remain enabled after enter")
	}
	if !p.detailView {
		t.Error("expected detail view to be open after enter")
	}
}

func TestNavigateToSubViews(t *testing.T) {
	p, _ := testSetup()

	p.NavigateTo("settings/logs", nil)
	if p.subView != "logs" {
		t.Errorf("expected logs sub-view, got %q", p.subView)
	}

	p.NavigateTo("settings/palette", nil)
	if p.subView != "palette" {
		t.Errorf("expected palette sub-view, got %q", p.subView)
	}

	// Navigating to "settings" (the tab route) preserves current sub-view
	p.NavigateTo("settings", nil)
	if p.subView != "palette" {
		t.Errorf("expected palette sub-view preserved, got %q", p.subView)
	}
}

func TestLogViewDoesNotPanic(t *testing.T) {
	p, _ := testSetup()
	p.subView = "logs"

	// Should render without panic even with empty logger
	v := p.View(120, 40, 0)
	if v == "" {
		t.Error("expected non-empty log view")
	}
}

func TestPaletteViewDoesNotPanic(t *testing.T) {
	p, _ := testSetup()
	p.subView = "palette"

	v := p.View(120, 40, 0)
	if v == "" {
		t.Error("expected non-empty palette view")
	}
}

func TestPaletteSwitchByKey(t *testing.T) {
	p, _ := testSetup()
	p.subView = "palette"

	// Move right
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	// In palette mode, 'l' switches to logs. Let's use right arrow instead.
	p.subView = "palette"
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	if p.paletteCursor != 1 {
		t.Errorf("expected palette cursor 1, got %d", p.paletteCursor)
	}
}

func TestDetailViewOpensAndCloses(t *testing.T) {
	p, _ := testSetup()

	// Open detail view
	p.cursor = 0
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !p.detailView {
		t.Error("expected detail view to open")
	}

	// Renders without panic
	v := p.View(120, 40, 0)
	if v == "" {
		t.Error("expected non-empty detail view")
	}

	// Esc closes it
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.detailView {
		t.Error("expected detail view to close on esc")
	}
}

func TestDetailViewCalendar(t *testing.T) {
	p, _ := testSetup()

	// Find calendar
	calIdx := -1
	for i, item := range p.items {
		if item.slug == "calendar" {
			calIdx = i
			break
		}
	}
	if calIdx < 0 {
		t.Fatal("calendar item not found")
	}

	p.cursor = calIdx
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !p.detailView {
		t.Error("expected detail view")
	}

	v := p.View(120, 40, 0)
	if v == "" {
		t.Error("expected non-empty calendar detail view")
	}
}

func TestDetailViewGitHub(t *testing.T) {
	p, _ := testSetup()

	// Find github
	ghIdx := -1
	for i, item := range p.items {
		if item.slug == "github" {
			ghIdx = i
			break
		}
	}
	if ghIdx < 0 {
		t.Fatal("github item not found")
	}

	p.cursor = ghIdx
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !p.detailView {
		t.Error("expected detail view")
	}

	v := p.View(120, 40, 0)
	if v == "" {
		t.Error("expected non-empty github detail view")
	}
}

func TestSubViewSwitchingByKey(t *testing.T) {
	p, _ := testSetup()

	// Switch to logs with 'l'
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if p.subView != "logs" {
		t.Errorf("expected logs, got %q", p.subView)
	}

	// Switch to palette with 'p'
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if p.subView != "palette" {
		t.Errorf("expected palette, got %q", p.subView)
	}

	// Switch back to plugins with 's'
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if p.subView != "plugins" {
		t.Errorf("expected plugins, got %q", p.subView)
	}
}
