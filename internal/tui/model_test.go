package tui

import (
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func testConfig() *config.Config {
	return &config.Config{
		Name:    "Test Center",
		Palette: "aurora",
		Todos:   config.TodosConfig{Enabled: true},
		Threads: config.ThreadsConfig{Enabled: true},
	}
}

func TestNewModel(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := testConfig()
	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger())

	if m.cfg.Name != "Test Center" {
		t.Errorf("expected name 'Test Center', got %q", m.cfg.Name)
	}
	if m.activeTab != tabNew {
		t.Errorf("expected initial tab to be tabNew")
	}
	if m.Launch != nil {
		t.Error("expected Launch to be nil initially")
	}
	if len(m.tabs) != 5 {
		t.Errorf("expected 5 tabs, got %d", len(m.tabs))
	}
}

func TestTabNavigationWithKeyTab(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig(), plugin.NewBus(), plugin.NewMemoryLogger())

	// Tab forward
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabResume {
		t.Errorf("expected tabResume after one tab, got %d", m.activeTab)
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabCommand {
		t.Errorf("expected tabCommand after two tabs, got %d", m.activeTab)
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabThreads {
		t.Errorf("expected tabThreads after three tabs, got %d", m.activeTab)
	}

	// Settings tab (index 4)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != 4 {
		t.Errorf("expected tab 4 (Settings) after four tabs, got %d", m.activeTab)
	}

	// Wrap back to tabNew
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabNew {
		t.Errorf("expected tabNew after five tabs (wrap), got %d", m.activeTab)
	}
}

func TestWindowResize(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig(), plugin.NewBus(), plugin.NewMemoryLogger())

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	if m.width != 120 || m.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", m.width, m.height)
	}
}

func TestViewDoesNotPanic(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig(), plugin.NewBus(), plugin.NewMemoryLogger())
	m.width = 120
	m.height = 40

	// View with default tab (New Session)
	v := m.View()
	if v == "" {
		t.Error("expected non-empty view")
	}

	// Command Center tab
	prev := m.activeTab
	m.activeTab = tabCommand
	m.activateTab(prev)
	v = m.View()
	if v == "" {
		t.Error("expected non-empty view for command tab")
	}

	// Threads tab
	prev = m.activeTab
	m.activeTab = tabThreads
	m.activateTab(prev)
	v = m.View()
	if v == "" {
		t.Error("expected non-empty view for threads tab")
	}
}

func TestStylesFromPalette(t *testing.T) {
	for _, name := range config.PaletteNames() {
		pal := config.GetPalette(name, nil)
		styles := NewStyles(pal)
		if styles.ColorCyan == "" {
			t.Errorf("palette %q produced empty ColorCyan", name)
		}
	}
}

func TestGradientColorsFromPalette(t *testing.T) {
	pal := config.GetPalette("aurora", nil)
	g := NewGradientColors(pal)
	c := ui.GradientColor(&g, 0.5)
	hex := c.Hex()
	if hex == "" {
		t.Error("expected non-empty hex color")
	}
}

func TestSubtitleFromText(t *testing.T) {
	got := subtitleFromText("CCC")
	if got != "C C C" {
		t.Errorf("expected 'C C C', got %q", got)
	}

	got = subtitleFromText("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}

	got = subtitleFromText("Center")
	if got != "C E N T E R" {
		t.Errorf("expected 'C E N T E R', got %q", got)
	}
}

func TestEscQuits(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig(), plugin.NewBus(), plugin.NewMemoryLogger())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Error("expected non-nil cmd (tea.Quit) on esc")
	}
}

func TestPluginTabMapping(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig(), plugin.NewBus(), plugin.NewMemoryLogger())

	// First two tabs should be sessions plugin
	if m.tabs[0].plugin.Slug() != "sessions" {
		t.Errorf("expected tab 0 to be sessions, got %s", m.tabs[0].plugin.Slug())
	}
	if m.tabs[1].plugin.Slug() != "sessions" {
		t.Errorf("expected tab 1 to be sessions, got %s", m.tabs[1].plugin.Slug())
	}
	// Next two should be commandcenter
	if m.tabs[2].plugin.Slug() != "commandcenter" {
		t.Errorf("expected tab 2 to be commandcenter, got %s", m.tabs[2].plugin.Slug())
	}
	if m.tabs[3].plugin.Slug() != "commandcenter" {
		t.Errorf("expected tab 3 to be commandcenter, got %s", m.tabs[3].plugin.Slug())
	}
	// Last tab should be settings
	if m.tabs[4].plugin.Slug() != "settings" {
		t.Errorf("expected tab 4 to be settings, got %s", m.tabs[4].plugin.Slug())
	}
}
