package tui

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

func testConfig() *config.Config {
	return &config.Config{
		Name:    "Test Center",
		Palette: "aurora",
		Todos:   config.TodosConfig{Enabled: true},
	}
}

func testCC() *db.CommandCenter {
	return &db.CommandCenter{
		GeneratedAt: time.Now(),
		Todos: []db.Todo{
			{ID: "t1", Title: "First todo", Status: "active", Source: "manual", CreatedAt: time.Now()},
			{ID: "t2", Title: "Second todo", Status: "active", Source: "manual", Due: "2025-01-01", CreatedAt: time.Now()},
			{ID: "t3", Title: "Third todo", Status: "completed", Source: "manual", CreatedAt: time.Now()},
		},
		Threads: []db.Thread{
			{ID: "th1", Title: "Active thread", Status: "active", Type: "manual", CreatedAt: time.Now()},
			{ID: "th2", Title: "Paused thread", Status: "paused", Type: "pr", CreatedAt: time.Now()},
		},
		Calendar: db.CalendarData{
			Today: []db.CalendarEvent{
				{Title: "Standup", Start: time.Now(), End: time.Now().Add(30 * time.Minute)},
			},
		},
	}
}

func TestNewModel(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := testConfig()
	m := NewModel(database, cfg)

	if m.cfg.Name != "Test Center" {
		t.Errorf("expected name 'Test Center', got %q", m.cfg.Name)
	}
	if m.activeTab != tabNew {
		t.Errorf("expected initial tab to be tabNew")
	}
	if m.Launch != nil {
		t.Error("expected Launch to be nil initially")
	}
}

func TestTabNavigation(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig())

	// Tab forward
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = result.(Model)
	// Note: "tab" key in bubbletea is tea.KeyTab, not runes. Use proper key type.
}

func TestTabNavigationWithKeyTab(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig())

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

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	if m.activeTab != tabNew {
		t.Errorf("expected tabNew after four tabs (wrap), got %d", m.activeTab)
	}
}

func TestWindowResize(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig())

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

	m := NewModel(database, testConfig())
	m.width = 120
	m.height = 40

	// View with nil cc
	v := m.View()
	if v == "" {
		t.Error("expected non-empty view")
	}

	// View with cc loaded
	m.cc = testCC()
	m.activeTab = tabCommand
	v = m.View()
	if v == "" {
		t.Error("expected non-empty view for command tab")
	}

	// Threads tab
	m.activeTab = tabThreads
	v = m.View()
	if v == "" {
		t.Error("expected non-empty view for threads tab")
	}
}

func TestCCLoadedMsg(t *testing.T) {
	database, err := db.OpenDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := NewModel(database, testConfig())
	cc := testCC()

	result, _ := m.Update(ccLoadedMsg{cc: cc})
	m = result.(Model)
	if m.cc == nil {
		t.Error("expected cc to be loaded")
	}
	if len(m.cc.ActiveTodos()) != 2 {
		t.Errorf("expected 2 active todos, got %d", len(m.cc.ActiveTodos()))
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
	// Just make sure it doesn't panic and produces valid colors
	c := gradientColor(&g, 0.5)
	hex := c.Hex()
	if hex == "" {
		t.Error("expected non-empty hex color")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"foo": "bar"}`, `{"foo": "bar"}`},
		{"```json\n{\"foo\": \"bar\"}\n```", `{"foo": "bar"}`},
		{`some text {"a": 1} more`, `{"a": 1}`},
	}
	for _, tt := range tests {
		got := extractJSON(tt.input)
		if got != tt.want {
			t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSubtitleFromName(t *testing.T) {
	got := subtitleFromName("CCC")
	if got != "C C C" {
		t.Errorf("expected 'C C C', got %q", got)
	}

	got = subtitleFromName("")
	if got != defaultSubtitle {
		t.Errorf("expected default subtitle, got %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{time.Hour, "1h"},
		{90 * time.Minute, "1h30m"},
		{0, ""},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTruncateToWidth(t *testing.T) {
	got := truncateToWidth("hello world", 5)
	if got != "hell~" {
		t.Errorf("expected 'hell~', got %q", got)
	}
	got = truncateToWidth("hi", 5)
	if got != "hi" {
		t.Errorf("expected 'hi', got %q", got)
	}
}

func TestFlattenTitle(t *testing.T) {
	got := flattenTitle("hello\nworld  foo")
	if got != "hello world foo" {
		t.Errorf("expected 'hello world foo', got %q", got)
	}
}

func TestCalendarColorMap(t *testing.T) {
	calendars := []config.CalendarEntry{
		{ID: "cal1", Label: "Personal", Color: "#00ff00"},
		{ID: "cal2", Label: "Work", Color: ""},
	}
	m := calendarColorMap(calendars)
	if m["cal1"] != "#00ff00" {
		t.Errorf("expected #00ff00, got %q", m["cal1"])
	}
	if _, ok := m["cal2"]; ok {
		t.Error("expected cal2 to be absent (empty color)")
	}
}
