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
		Gmail:    config.GmailConfig{Enabled: true},
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

// findNavIndex returns the flat item index for a given slug, or -1 if not found.
func findNavIndex(p *Plugin, slug string) int {
	idx := 0
	for _, cat := range p.navCategories {
		for _, item := range cat.Items {
			if item.Slug == slug {
				return idx
			}
			idx++
		}
	}
	return -1
}

// findNavItemInCategory checks if a slug exists under a specific category label.
func findNavItemInCategory(p *Plugin, categoryLabel, slug string) bool {
	for _, cat := range p.navCategories {
		if cat.Label != categoryLabel {
			continue
		}
		for _, item := range cat.Items {
			if item.Slug == slug {
				return true
			}
		}
	}
	return false
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

// --- Nav model tests ---

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
	for _, expected := range []string{"APPEARANCE", "PLUGINS", "DATA SOURCES", "SYSTEM"} {
		if !catLabels[expected] {
			t.Errorf("expected %s category", expected)
		}
	}
}

func TestNavCategoryCount(t *testing.T) {
	p, _ := testSetup()
	if len(p.navCategories) != 4 {
		t.Errorf("expected 4 categories, got %d", len(p.navCategories))
	}
}

func TestNavAppearanceItems(t *testing.T) {
	p, _ := testSetup()
	for _, cat := range p.navCategories {
		if cat.Label == "APPEARANCE" {
			if len(cat.Items) != 2 {
				t.Errorf("expected 2 APPEARANCE items, got %d", len(cat.Items))
			}
			slugs := map[string]bool{}
			for _, item := range cat.Items {
				slugs[item.Slug] = true
			}
			if !slugs["banner"] {
				t.Error("expected banner in APPEARANCE")
			}
			if !slugs["palette"] {
				t.Error("expected palette in APPEARANCE")
			}
			return
		}
	}
	t.Error("APPEARANCE category not found")
}

func TestNavDataSourcesItems(t *testing.T) {
	p, _ := testSetup()
	for _, cat := range p.navCategories {
		if cat.Label == "DATA SOURCES" {
			expected := []string{"calendar", "github", "granola", "slack", "gmail"}
			if len(cat.Items) != len(expected) {
				t.Errorf("expected %d DATA SOURCES items, got %d", len(expected), len(cat.Items))
			}
			slugs := map[string]bool{}
			for _, item := range cat.Items {
				slugs[item.Slug] = true
			}
			for _, slug := range expected {
				if !slugs[slug] {
					t.Errorf("expected %s in DATA SOURCES", slug)
				}
			}
			return
		}
	}
	t.Error("DATA SOURCES category not found")
}

func TestNavGmailInDataSources(t *testing.T) {
	p, _ := testSetup()
	if !findNavItemInCategory(p, "DATA SOURCES", "gmail") {
		t.Error("expected gmail in DATA SOURCES category")
	}
}

func TestNavSystemItems(t *testing.T) {
	p, _ := testSetup()
	for _, cat := range p.navCategories {
		if cat.Label == "SYSTEM" {
			expected := []string{"system-schedule", "system-mcp", "system-skills", "system-shell", "system-logs"}
			if len(cat.Items) != len(expected) {
				t.Errorf("expected %d SYSTEM items, got %d", len(expected), len(cat.Items))
			}
			slugs := map[string]bool{}
			for _, item := range cat.Items {
				slugs[item.Slug] = true
			}
			for _, slug := range expected {
				if !slugs[slug] {
					t.Errorf("expected %s in SYSTEM", slug)
				}
			}
			return
		}
	}
	t.Error("SYSTEM category not found")
}

func TestNavPluginsCategory(t *testing.T) {
	p, _ := testSetup()
	// With only settings registered (excluded) + threads + 1 external plugin, PLUGINS should have 2 items
	for _, cat := range p.navCategories {
		if cat.Label == "PLUGINS" {
			if len(cat.Items) != 2 {
				t.Errorf("expected 2 PLUGINS items (Threads + external Pomodoro), got %d", len(cat.Items))
			}
			slugs := map[string]bool{}
			for _, item := range cat.Items {
				slugs[item.Slug] = true
			}
			if !slugs["threads"] {
				t.Error("expected threads in PLUGINS")
			}
			if !slugs["external-0"] {
				t.Error("expected external-0 in PLUGINS")
			}
			return
		}
	}
	t.Error("PLUGINS category not found")
}

func TestNavThreadsInPlugins(t *testing.T) {
	p, _ := testSetup()
	if !findNavItemInCategory(p, "PLUGINS", "threads") {
		t.Error("expected Threads in PLUGINS category")
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

func TestNavItemCount(t *testing.T) {
	p, _ := testSetup()
	// APPEARANCE(2) + PLUGINS(2: threads + external) + DATA SOURCES(5) + SYSTEM(5) = 14
	expected := 14
	if got := p.navItemCount(); got != expected {
		t.Errorf("expected %d nav items, got %d", expected, got)
	}
}

func TestSelectedNavItemReturnsCorrectItem(t *testing.T) {
	p, _ := testSetup()

	// First item should be banner (first item of APPEARANCE)
	p.navCursor = 0
	item := p.selectedNavItem()
	if item == nil || item.Slug != "banner" {
		slug := "nil"
		if item != nil {
			slug = item.Slug
		}
		t.Errorf("expected banner at index 0, got %s", slug)
	}

	// Second item should be palette
	p.navCursor = 1
	item = p.selectedNavItem()
	if item == nil || item.Slug != "palette" {
		slug := "nil"
		if item != nil {
			slug = item.Slug
		}
		t.Errorf("expected palette at index 1, got %s", slug)
	}
}

func TestSelectedNavItemOutOfRange(t *testing.T) {
	p, _ := testSetup()
	p.navCursor = 999
	item := p.selectedNavItem()
	if item != nil {
		t.Errorf("expected nil for out-of-range cursor, got %q", item.Slug)
	}
}

// --- Navigation tests ---

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

func TestNavCursorClampsAtBottom(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusNav
	maxIdx := p.navItemCount() - 1
	p.navCursor = maxIdx

	// Down at bottom stays at bottom
	p.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if p.navCursor != maxIdx {
		t.Errorf("expected nav cursor to stay at %d, got %d", maxIdx, p.navCursor)
	}
}

func TestNavCursorJK(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusNav
	p.navCursor = 0

	// j = down
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if p.navCursor != 1 {
		t.Errorf("expected nav cursor 1 after j, got %d", p.navCursor)
	}

	// k = up
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if p.navCursor != 0 {
		t.Errorf("expected nav cursor 0 after k, got %d", p.navCursor)
	}
}

func TestNavSkipsCategoryHeaders(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusNav

	// Navigate from last APPEARANCE item (index 1 = palette) to first PLUGINS item (index 2)
	p.navCursor = 1
	p.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	// Should go to index 2 — no category header in the selectable items
	if p.navCursor != 2 {
		t.Errorf("expected cursor to move to 2 (skipping category header), got %d", p.navCursor)
	}
	item := p.selectedNavItem()
	if item == nil {
		t.Fatal("expected non-nil item at cursor 2")
	}
	// Item at index 2 should be the first plugin (threads)
	if item.Slug != "threads" {
		t.Errorf("expected threads at cursor 2, got %q", item.Slug)
	}
}

// --- Focus transition tests ---

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

func TestFocusRightToContent(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusNav

	// Right arrow should switch to content
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	if p.focusZone != FocusContent {
		t.Errorf("expected FocusContent after right, got %d", p.focusZone)
	}
}

func TestFocusLeftToNav(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusContent

	// Left arrow should switch back to nav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after left, got %d", p.focusZone)
	}
}

func TestFocusLToContent(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusNav

	// 'l' should switch to content (vim-style)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if p.focusZone != FocusContent {
		t.Errorf("expected FocusContent after 'l', got %d", p.focusZone)
	}
}

func TestFocusHToNav(t *testing.T) {
	p, _ := testSetup()
	p.focusZone = FocusContent

	// 'h' should switch back to nav (vim-style)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after 'h', got %d", p.focusZone)
	}
}

// --- Toggle tests ---

func TestToggleDataSourceViaSidebar(t *testing.T) {
	p, _ := testSetup()

	calIdx := findNavIndex(p, "calendar")
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

	extIdx := findNavIndex(p, "external-0")
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

func TestSpaceOnNonToggleable(t *testing.T) {
	p, _ := testSetup()

	// Banner is not toggleable
	bannerIdx := findNavIndex(p, "banner")
	if bannerIdx < 0 {
		t.Fatal("banner not found in nav")
	}

	p.navCursor = bannerIdx
	p.focusZone = FocusNav

	item := p.selectedNavItem()
	if item.Toggleable {
		t.Error("expected banner to not be toggleable")
	}

	// Space on a non-toggleable should be a no-op (no panic)
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
}

func TestToggleGmailDataSource(t *testing.T) {
	p, _ := testSetup()

	gmailIdx := findNavIndex(p, "gmail")
	if gmailIdx < 0 {
		t.Fatal("gmail not found in nav")
	}

	p.navCursor = gmailIdx
	p.focusZone = FocusNav

	item := p.selectedNavItem()
	if item == nil || item.Enabled == nil || !*item.Enabled {
		t.Error("expected gmail to be enabled initially")
	}

	// Toggle off
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})

	item = p.selectedNavItem()
	if item == nil || item.Enabled == nil || *item.Enabled {
		t.Error("expected gmail to be disabled after toggle")
	}
}

// --- View rendering tests ---

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

	palIdx := findNavIndex(p, "palette")
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

// --- Data source validation status ---

func TestDataSourcesHaveValidField(t *testing.T) {
	p, _ := testSetup()
	for _, cat := range p.navCategories {
		if cat.Label != "DATA SOURCES" {
			continue
		}
		for _, item := range cat.Items {
			if item.Valid == nil {
				t.Errorf("expected data source %q to have non-nil Valid field", item.Slug)
			}
		}
	}
}

func TestDataSourcesAreToggleable(t *testing.T) {
	p, _ := testSetup()
	for _, cat := range p.navCategories {
		if cat.Label != "DATA SOURCES" {
			continue
		}
		for _, item := range cat.Items {
			if !item.Toggleable {
				t.Errorf("expected data source %q to be toggleable", item.Slug)
			}
			if item.Enabled == nil {
				t.Errorf("expected data source %q to have non-nil Enabled", item.Slug)
			}
		}
	}
}

// --- System items are not toggleable ---

func TestSystemItemsNotToggleable(t *testing.T) {
	p, _ := testSetup()
	for _, cat := range p.navCategories {
		if cat.Label != "SYSTEM" {
			continue
		}
		for _, item := range cat.Items {
			if item.Toggleable {
				t.Errorf("expected system item %q to not be toggleable", item.Slug)
			}
		}
	}
}

func TestAppearanceItemsNotToggleable(t *testing.T) {
	p, _ := testSetup()
	for _, cat := range p.navCategories {
		if cat.Label != "APPEARANCE" {
			continue
		}
		for _, item := range cat.Items {
			if item.Toggleable {
				t.Errorf("expected appearance item %q to not be toggleable", item.Slug)
			}
		}
	}
}
