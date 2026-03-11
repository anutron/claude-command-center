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

func TestRoutesReturnsOne(t *testing.T) {
	p, _ := testSetup()
	routes := p.Routes()
	if len(routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(routes))
	}
}

func TestNavCategoriesPopulated(t *testing.T) {
	p, _ := testSetup()
	if len(p.navCategories) == 0 {
		t.Error("expected nav categories to be populated")
	}

	// Should have categories: APPEARANCE, PLUGINS, DATA SOURCES, SYSTEM
	catLabels := map[string]bool{}
	for _, cat := range p.navCategories {
		catLabels[cat.Label] = true
	}
	if !catLabels["APPEARANCE"] {
		t.Error("expected APPEARANCE category")
	}
	if !catLabels["PLUGINS"] {
		t.Error("expected PLUGINS category")
	}
	if !catLabels["DATA SOURCES"] {
		t.Error("expected DATA SOURCES category")
	}
	if !catLabels["SYSTEM"] {
		t.Error("expected SYSTEM category")
	}
}

func TestNavHasExpectedItems(t *testing.T) {
	p, _ := testSetup()

	slugs := map[string]bool{}
	for _, cat := range p.navCategories {
		for _, item := range cat.Items {
			slugs[item.Slug] = true
		}
	}

	// Appearance items
	if !slugs["banner"] {
		t.Error("expected banner in nav")
	}
	if !slugs["palette"] {
		t.Error("expected palette in nav")
	}
	// External plugin
	if !slugs["external-0"] {
		t.Error("expected external-0 (Pomodoro) in nav")
	}
	// Data sources
	if !slugs["calendar"] {
		t.Error("expected calendar in nav")
	}
	// System
	if !slugs["system-logs"] {
		t.Error("expected system-logs in nav")
	}
}

func TestToggleDataSourceViaSidebar(t *testing.T) {
	p, _ := testSetup()

	// Navigate to calendar item in sidebar
	calIdx := -1
	idx := 0
	for _, cat := range p.navCategories {
		for _, item := range cat.Items {
			if item.Slug == "calendar" {
				calIdx = idx
			}
			idx++
		}
	}
	if calIdx < 0 {
		t.Fatal("calendar item not found in nav")
	}

	p.navCursor = calIdx
	p.focusZone = FocusNav

	// Calendar should be enabled initially
	item := p.selectedNavItem()
	if item == nil || item.Enabled == nil || !*item.Enabled {
		t.Error("expected calendar to be enabled initially")
	}

	// Toggle with space
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})

	item = p.selectedNavItem()
	if item == nil || item.Enabled == nil || *item.Enabled {
		t.Error("expected calendar to be disabled after toggle")
	}
}

func TestToggleExternalPluginViaSidebar(t *testing.T) {
	p, _ := testSetup()

	// Find external plugin in nav
	extIdx := -1
	idx := 0
	for _, cat := range p.navCategories {
		for _, item := range cat.Items {
			if item.Slug == "external-0" {
				extIdx = idx
			}
			idx++
		}
	}
	if extIdx < 0 {
		t.Fatal("external plugin not found in nav")
	}

	p.navCursor = extIdx
	p.focusZone = FocusNav

	item := p.selectedNavItem()
	if item == nil || item.Enabled == nil || !*item.Enabled {
		t.Error("expected external plugin to be enabled initially")
	}

	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})

	item = p.selectedNavItem()
	if item == nil || item.Enabled == nil || *item.Enabled {
		t.Error("expected external plugin to be disabled after toggle")
	}
	if p.flashMessage != "Restart CCC to apply" {
		t.Errorf("expected restart flash message, got %q", p.flashMessage)
	}
}

func TestFocusSwitching(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusNav

	// Enter should switch to content
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focusZone != FocusContent {
		t.Errorf("expected FocusContent, got %d", p.focusZone)
	}

	// Esc should go back to nav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after esc, got %d", p.focusZone)
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	p, _ := testSetup()

	v := p.View(120, 40, 0)
	if v == "" {
		t.Error("expected non-empty view")
	}
}

func TestViewContentPaneRendersForEachNavItem(t *testing.T) {
	p, _ := testSetup()

	// Iterate through all nav items and render each content pane
	total := p.navItemCount()
	for i := 0; i < total; i++ {
		p.navCursor = i
		p.focusZone = FocusContent
		v := p.View(120, 40, 0)
		if v == "" {
			item := p.selectedNavItem()
			slug := "nil"
			if item != nil {
				slug = item.Slug
			}
			t.Errorf("expected non-empty view for nav item %d (slug=%s)", i, slug)
		}
	}
}

func TestPaletteCursorNavigation(t *testing.T) {
	p, _ := testSetup()

	// Navigate to palette
	palIdx := -1
	idx := 0
	for _, cat := range p.navCategories {
		for _, item := range cat.Items {
			if item.Slug == "palette" {
				palIdx = idx
			}
			idx++
		}
	}
	if palIdx < 0 {
		t.Fatal("palette not found in nav")
	}

	p.navCursor = palIdx
	p.focusZone = FocusContent

	// Navigate down in palette content
	p.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if p.paletteCursor != 1 {
		t.Errorf("expected palette cursor 1, got %d", p.paletteCursor)
	}
}

func TestNavCursorUpDown(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusNav
	p.navCursor = 0

	// Navigate down
	p.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if p.navCursor != 1 {
		t.Errorf("expected nav cursor 1, got %d", p.navCursor)
	}

	// Navigate up
	p.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if p.navCursor != 0 {
		t.Errorf("expected nav cursor 0, got %d", p.navCursor)
	}

	// Up at 0 stays at 0
	p.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if p.navCursor != 0 {
		t.Errorf("expected nav cursor to stay at 0, got %d", p.navCursor)
	}
}
