package tui

import (
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func testSetup(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	return &config.Config{
		Name:    "Test Center",
		Palette: "aurora",
		Todos:   config.TodosConfig{Enabled: true},
	}
}

func TestNewModel(t *testing.T) {
	cfg := testSetup(t)
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger(), llm.NoopLLM{})

	if m.cfg.Name != "Test Center" {
		t.Errorf("expected name 'Test Center', got %q", m.cfg.Name)
	}
	if m.activeTab != tabNew {
		t.Errorf("expected initial tab to be tabNew")
	}
	if m.Launch != nil {
		t.Error("expected Launch to be nil initially")
	}
	if len(m.tabs) != 6 {
		t.Errorf("expected 6 tabs, got %d", len(m.tabs))
	}
}

func TestTabNavigationWithKeyTab(t *testing.T) {
	cfg := testSetup(t)
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger(), llm.NoopLLM{})

	// Tab forward through all 6 tabs: Active(0), New Session(1), Resume(2), Command(3), PRs(4), Settings(5)
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabLaunch {
		t.Errorf("expected tabLaunch after one tab, got %d", m.activeTab)
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabResume {
		t.Errorf("expected tabResume after two tabs, got %d", m.activeTab)
	}

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabCommand {
		t.Errorf("expected tabCommand after three tabs, got %d", m.activeTab)
	}

	// PRs tab (index 4)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != 4 {
		t.Errorf("expected tab 4 (PRs) after four tabs, got %d", m.activeTab)
	}

	// Settings tab (index 5)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != 5 {
		t.Errorf("expected tab 5 (Settings) after five tabs, got %d", m.activeTab)
	}

	// Wrap back to tabNew (0)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabNew {
		t.Errorf("expected tabNew after six tabs (wrap), got %d", m.activeTab)
	}
}

func TestWindowResize(t *testing.T) {
	cfg := testSetup(t)
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger(), llm.NoopLLM{})

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	if m.width != 120 || m.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", m.width, m.height)
	}
}

func TestViewDoesNotPanic(t *testing.T) {
	cfg := testSetup(t)
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger(), llm.NoopLLM{})
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
	cfg := testSetup(t)
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger(), llm.NoopLLM{})

	// Default sub-tab is "sessions". First Esc navigates sessions→new (not quit).
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Error("first esc should navigate to new tab, not quit")
	}

	// Second Esc from the new sub-tab should quit.
	_, cmd = newM.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Error("expected non-nil cmd (tea.Quit) on second esc")
	}
}

func TestPluginTabMapping(t *testing.T) {
	cfg := testSetup(t)
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, cfg, plugin.NewBus(), plugin.NewMemoryLogger(), llm.NoopLLM{})

	// First three tabs should be sessions plugin (Active, New Session, Resume)
	for i := 0; i < 3; i++ {
		if m.tabs[i].plugin.Slug() != "sessions" {
			t.Errorf("expected tab %d to be sessions, got %s", i, m.tabs[i].plugin.Slug())
		}
	}
	// Next should be commandcenter
	if m.tabs[3].plugin.Slug() != "commandcenter" {
		t.Errorf("expected tab 3 to be commandcenter, got %s", m.tabs[3].plugin.Slug())
	}
	// Next should be prs
	if m.tabs[4].plugin.Slug() != "prs" {
		t.Errorf("expected tab 4 to be prs, got %s", m.tabs[4].plugin.Slug())
	}
	// Last tab should be settings
	if m.tabs[5].plugin.Slug() != "settings" {
		t.Errorf("expected tab 5 to be settings, got %s", m.tabs[5].plugin.Slug())
	}
}
