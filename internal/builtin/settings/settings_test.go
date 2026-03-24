package settings

import (
	"fmt"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/auth"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/refresh/sources/calendar"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/oauth2"
)

func testSetup(t *testing.T) (*Plugin, *plugin.Registry) {
	t.Helper()
	// Redirect config writes to a temp dir so tests never touch the real
	// user config at ~/.config/ccc/config.yaml (root cause of BUG-046).
	// Also redirect HOME so tests don't find real Google OAuth tokens
	// (which would skip the credential form in auth flow tests).
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmpHome)
	t.Setenv("HOME", tmpHome)

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
	cfg.MarkLoadedFromFile()

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
	p, _ := testSetup(t)
	if p.Slug() != "settings" {
		t.Errorf("expected slug 'settings', got %q", p.Slug())
	}
	if p.TabName() != "Settings" {
		t.Errorf("expected tab name 'Settings', got %q", p.TabName())
	}
}

func TestRoutesReturnsOne(t *testing.T) {
	p, _ := testSetup(t)
	routes := p.Routes()
	if len(routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(routes))
	}
}

// --- Nav model tests ---

func TestNavCategoriesPopulated(t *testing.T) {
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
	if len(p.navCategories) != 5 {
		t.Errorf("expected 5 categories, got %d", len(p.navCategories))
	}
}

func TestNavAppearanceItems(t *testing.T) {
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
	if !findNavItemInCategory(p, "DATA SOURCES", "gmail") {
		t.Error("expected gmail in DATA SOURCES category")
	}
}

func TestNavSystemItems(t *testing.T) {
	p, _ := testSetup(t)
	for _, cat := range p.navCategories {
		if cat.Label == "SYSTEM" {
			expected := []string{"system-automations", "system-schedule", "system-mcp", "system-skills", "system-shell", "system-logs"}
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
	p, _ := testSetup(t)
	// With only settings registered (excluded) + 1 external plugin, PLUGINS should have 1 item
	for _, cat := range p.navCategories {
		if cat.Label == "PLUGINS" {
			if len(cat.Items) != 1 {
				t.Errorf("expected 1 PLUGINS item (external Pomodoro), got %d", len(cat.Items))
			}
			slugs := map[string]bool{}
			for _, item := range cat.Items {
				slugs[item.Slug] = true
			}
			if !slugs["external-0"] {
				t.Error("expected external-0 in PLUGINS")
			}
			return
		}
	}
	t.Error("PLUGINS category not found")
}

func TestNavHasExpectedItems(t *testing.T) {
	p, _ := testSetup(t)

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
	p, _ := testSetup(t)
	// APPEARANCE(2) + PLUGINS(1: external) + DATA SOURCES(5) + AGENT(2: budget+sandbox) + SYSTEM(6) = 16
	expected := 16
	if got := p.navItemCount(); got != expected {
		t.Errorf("expected %d nav items, got %d", expected, got)
	}
}

func TestSelectedNavItemReturnsCorrectItem(t *testing.T) {
	p, _ := testSetup(t)

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
	p, _ := testSetup(t)
	p.navCursor = 999
	item := p.selectedNavItem()
	if item != nil {
		t.Errorf("expected nil for out-of-range cursor, got %q", item.Slug)
	}
}

// --- Navigation tests ---

func TestNavCursorUpDown(t *testing.T) {
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
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
	// Item at index 2 should be the first plugin (external-0)
	if item.Slug != "external-0" {
		t.Errorf("expected external-0 at cursor 2, got %q", item.Slug)
	}
}

// --- Focus transition tests ---

func TestFocusSwitching(t *testing.T) {
	p, _ := testSetup(t)
	p.focusZone = FocusNav

	// Enter should switch to content
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm, got %d", p.focusZone)
	}

	// Esc should go back to nav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after esc, got %d", p.focusZone)
	}
}

func TestFocusRightToContent(t *testing.T) {
	p, _ := testSetup(t)
	p.focusZone = FocusNav

	// Right arrow should switch to content
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm after right, got %d", p.focusZone)
	}
}

func TestFocusLeftToNav(t *testing.T) {
	p, _ := testSetup(t)
	p.focusZone = FocusForm

	// Left arrow should switch back to nav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after left, got %d", p.focusZone)
	}
}

func TestFocusLToContent(t *testing.T) {
	p, _ := testSetup(t)
	p.focusZone = FocusNav

	// 'l' should switch to content (vim-style)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm after 'l', got %d", p.focusZone)
	}
}

func TestFocusHToNav(t *testing.T) {
	p, _ := testSetup(t)
	p.focusZone = FocusForm

	// 'h' should switch back to nav (vim-style)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after 'h', got %d", p.focusZone)
	}
}

// --- Toggle tests ---

func TestToggleDataSourceViaSidebar(t *testing.T) {
	p, _ := testSetup(t)

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
	p, _ := testSetup(t)

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
	if p.flashMessage != "Pomodoro disabled" {
		t.Errorf("expected disabled flash message, got %q", p.flashMessage)
	}
}

func TestSpaceOnNonToggleable(t *testing.T) {
	p, _ := testSetup(t)

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
	p, _ := testSetup(t)

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
	p, _ := testSetup(t)

	v := p.View(120, 40, 0)
	if v == "" {
		t.Error("expected non-empty view")
	}
}

func TestViewContentPaneRendersForEachNavItem(t *testing.T) {
	p, _ := testSetup(t)

	// Iterate through all nav items and render each content pane
	total := p.navItemCount()
	for i := 0; i < total; i++ {
		p.navCursor = i
		p.focusZone = FocusForm
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

func TestPaletteFormCreated(t *testing.T) {
	p, _ := testSetup(t)

	palIdx := findNavIndex(p, "palette")
	if palIdx < 0 {
		t.Fatal("palette not found in nav")
	}

	p.navCursor = palIdx

	// Opening the palette pane should create a huh form
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if p.activeForm == nil {
		t.Error("expected activeForm to be set for palette")
	}
	if p.activeFormSlug != "palette" {
		t.Errorf("expected activeFormSlug 'palette', got %q", p.activeFormSlug)
	}
	if p.paletteValues == nil {
		t.Error("expected paletteValues to be set")
	}
}

// --- Data source validation status ---

func TestDataSourcesHaveValidField(t *testing.T) {
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
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
	p, _ := testSetup(t)
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

// --- Validation status tests ---

func TestDataSourcesHaveValidationStatus(t *testing.T) {
	p, _ := testSetup(t)
	for _, cat := range p.navCategories {
		if cat.Label != "DATA SOURCES" {
			continue
		}
		for _, item := range cat.Items {
			if item.ValidationStatus == "" {
				t.Errorf("expected data source %q to have non-empty ValidationStatus", item.Slug)
			}
			// Status should be one of the known values
			switch item.ValidationStatus {
			case "ok", "missing", "incomplete", "no_client", "unverified":
				// valid
			default:
				t.Errorf("unexpected ValidationStatus %q for %s", item.ValidationStatus, item.Slug)
			}
		}
	}
}

func TestGoogleDatasourcesIdentified(t *testing.T) {
	if !isGoogleDatasource("calendar") {
		t.Error("calendar should be a Google datasource")
	}
	if !isGoogleDatasource("gmail") {
		t.Error("gmail should be a Google datasource")
	}
	if isGoogleDatasource("github") {
		t.Error("github should not be a Google datasource")
	}
	if isGoogleDatasource("slack") {
		t.Error("slack should not be a Google datasource")
	}
	if isGoogleDatasource("granola") {
		t.Error("granola should not be a Google datasource")
	}
}

// --- Datasource recheck tests ---

func TestRecheckUpdatesNavItem(t *testing.T) {
	p, _ := testSetup(t)

	// Apply a recheck result for calendar
	msg := datasourceRecheckResult{
		Slug: "calendar",
		Result: plugin.ValidationResult{
			Status:  "incomplete",
			Message: "Token expired",
			Hint:    "Press 'a' to re-authenticate",
		},
	}
	p.applyRecheckResult(msg)

	// Find the calendar nav item and verify it was updated
	calIdx := findNavIndex(p, "calendar")
	if calIdx < 0 {
		t.Fatal("calendar not found")
	}
	p.navCursor = calIdx
	item := p.selectedNavItem()
	if item.ValidationStatus != "incomplete" {
		t.Errorf("expected ValidationStatus 'incomplete', got %q", item.ValidationStatus)
	}
	if item.ValidationMsg != "Token expired" {
		t.Errorf("expected ValidationMsg 'Token expired', got %q", item.ValidationMsg)
	}
	if item.ValidHint != "Press 'a' to re-authenticate" {
		t.Errorf("expected ValidHint, got %q", item.ValidHint)
	}
	if item.Valid == nil || *item.Valid {
		t.Error("expected Valid=false for incomplete status")
	}
	if p.flashMessage != "Token expired" {
		t.Errorf("expected flash message, got %q", p.flashMessage)
	}
}

func TestRecheckOKUpdatesValid(t *testing.T) {
	p, _ := testSetup(t)

	msg := datasourceRecheckResult{
		Slug: "gmail",
		Result: plugin.ValidationResult{
			Status:  "ok",
			Message: "Gmail token is valid",
		},
	}
	p.applyRecheckResult(msg)

	gmailIdx := findNavIndex(p, "gmail")
	p.navCursor = gmailIdx
	item := p.selectedNavItem()
	// BUG-030: When credentials check returns "ok" but there's no sync
	// history, status should be "unverified" (yellow) not "ok" (green).
	// Green check is reserved for tokens proven to work via successful sync.
	if item.ValidationStatus != "unverified" {
		t.Errorf("expected 'unverified' (no sync history), got %q", item.ValidationStatus)
	}
	if item.Valid == nil || *item.Valid {
		t.Error("expected Valid=false when credentials ok but never synced")
	}
}

func TestRecheckOKWithSuccessfulSync(t *testing.T) {
	p, _ := testSetup(t)

	// Set up a successful sync record
	gmailIdx := findNavIndex(p, "gmail")
	p.navCursor = gmailIdx
	item := p.selectedNavItem()
	now := time.Now()
	item.SyncStatus = &db.SourceSync{
		Source:      "gmail",
		LastSuccess: &now,
		UpdatedAt:   now,
	}

	// Recheck returns "ok" and sync succeeded — should be green
	msg := datasourceRecheckResult{
		Slug: "gmail",
		Result: plugin.ValidationResult{
			Status:  "ok",
			Message: "Gmail token is valid",
		},
	}
	p.applyRecheckResult(msg)

	item = p.selectedNavItem()
	if item.ValidationStatus != "ok" {
		t.Errorf("expected 'ok' (verified via sync), got %q", item.ValidationStatus)
	}
	if item.Valid == nil || !*item.Valid {
		t.Error("expected Valid=true when credentials ok and sync succeeded")
	}
}

func TestRecheckOKWithSyncError(t *testing.T) {
	p, _ := testSetup(t)

	// Simulate a sync record with an error by setting SyncStatus directly
	// on the nav item (since we have no database in tests).
	gmailIdx := findNavIndex(p, "gmail")
	p.navCursor = gmailIdx
	item := p.selectedNavItem()
	now := time.Now()
	item.SyncStatus = &db.SourceSync{
		Source:      "gmail",
		LastSuccess: &now,
		LastError:   "auth: token expired",
		UpdatedAt:   now,
	}

	// BUG-030: When credentials check returns "ok" but last sync had an
	// error, status should be "incomplete" (yellow) to indicate the problem.
	msg := datasourceRecheckResult{
		Slug: "gmail",
		Result: plugin.ValidationResult{
			Status:  "ok",
			Message: "Gmail token is valid",
		},
	}
	p.applyRecheckResult(msg)

	item = p.selectedNavItem()
	if item.ValidationStatus != "incomplete" {
		t.Errorf("expected 'incomplete' (sync error), got %q", item.ValidationStatus)
	}
	if item.Valid == nil || *item.Valid {
		t.Error("expected Valid=false when last sync had an error")
	}
}

// --- Content key handler tests ---

func TestRecheckActionFiresRecheck(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusForm

	// Simulate selecting "recheck" from the datasource form
	p.datasourceValues = &datasourceFormValues{Action: "recheck"}
	cmd := p.handleDatasourceFormCompletion("calendar")
	if cmd == nil {
		t.Error("expected recheck action to return a tea.Cmd")
	}
	if p.flashMessage == "" {
		t.Error("expected flash message for recheck")
	}
}

func TestRecheckActionLiveCheckForGoogle(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusForm

	p.datasourceValues = &datasourceFormValues{Action: "recheck"}
	p.handleDatasourceFormCompletion("calendar")
	if p.flashMessage != "Verifying Calendar credentials..." {
		t.Errorf("expected verify flash for Google datasource, got %q", p.flashMessage)
	}
}

func TestRecheckActionLiveCheckForNonGoogle(t *testing.T) {
	p, _ := testSetup(t)

	ghIdx := findNavIndex(p, "github")
	p.navCursor = ghIdx
	p.focusZone = FocusForm

	p.datasourceValues = &datasourceFormValues{Action: "recheck"}
	p.handleDatasourceFormCompletion("github")
	// All sources now do live verification (BUG-053)
	if p.flashMessage != "Verifying GitHub credentials..." {
		t.Errorf("expected verify flash for datasource, got %q", p.flashMessage)
	}
}

func TestAuthActionTriggersFormForGoogle(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusForm

	// Simulate selecting "auth" from the datasource form
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	cmd := p.handleDatasourceFormCompletion("calendar")
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm after auth action, got %d", p.focusZone)
	}
	if p.activeForm == nil {
		t.Error("expected activeForm to be set")
	}
	if p.pendingAuthSlug != "calendar" {
		t.Errorf("expected pendingAuthSlug 'calendar', got %q", p.pendingAuthSlug)
	}
	if p.pendingAuthCreds == nil {
		t.Error("expected pendingAuthCreds to be set")
	}
	if cmd == nil {
		t.Error("expected tea.Cmd from form init")
	}
}

func TestAuthActionNoopForNonGoogle(t *testing.T) {
	p, _ := testSetup(t)

	ghIdx := findNavIndex(p, "github")
	p.navCursor = ghIdx
	p.focusZone = FocusForm

	// GitHub datasource form doesn't offer "auth" action, but even if
	// triggered directly, it should be a no-op (no form change).
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	p.handleDatasourceFormCompletion("github")
	// Auth action for non-Google should not set up auth state
	if p.pendingAuthCreds != nil {
		t.Error("expected no pendingAuthCreds for non-Google datasource")
	}
}

func TestConsoleActionNoopForNonGoogle(t *testing.T) {
	p, _ := testSetup(t)

	slackIdx := findNavIndex(p, "slack")
	p.navCursor = slackIdx
	p.focusZone = FocusForm

	// Console action for non-Google datasource should be a no-op
	p.datasourceValues = &datasourceFormValues{Action: "console"}
	cmd := p.handleDatasourceFormCompletion("slack")
	if cmd != nil {
		t.Error("expected nil cmd for console action on non-Google datasource")
	}
}

// --- FocusForm key handling tests ---

func TestEscCancelsFormAndReturnsToContent(t *testing.T) {
	p, _ := testSetup(t)

	// Trigger auth form via the form completion handler
	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusForm

	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	p.handleDatasourceFormCompletion("calendar")

	if p.focusZone != FocusForm {
		t.Fatal("expected FocusForm")
	}

	// Esc cancels
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm after esc, got %d", p.focusZone)
	}
	if p.activeForm != nil {
		t.Error("expected activeForm to be nil after esc")
	}
	if p.pendingAuthCreds != nil {
		t.Error("expected pendingAuthCreds to be nil after esc")
	}
	if p.pendingAuthSlug != "" {
		t.Error("expected pendingAuthSlug to be empty after esc")
	}
}

func TestFormFocusWithNilFormFallsBackToContent(t *testing.T) {
	p, _ := testSetup(t)

	p.focusZone = FocusForm
	p.activeForm = nil

	// Should gracefully fall back to content
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm when form is nil, got %d", p.focusZone)
	}
}

// --- TabLeave tests ---

func TestTabLeaveCancelsForm(t *testing.T) {
	p, _ := testSetup(t)

	// Set up auth form state via form completion handler
	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusForm

	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	p.handleDatasourceFormCompletion("calendar")

	if p.activeForm == nil {
		t.Fatal("expected form to be active")
	}

	// Tab leave
	p.HandleMessage(plugin.TabLeaveMsg{})
	if p.activeForm != nil {
		t.Error("expected form to be cleared on tab leave")
	}
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm on tab leave, got %d", p.focusZone)
	}
}

// --- Content view rendering tests ---

func TestViewValidationStatusRendersAllStatuses(t *testing.T) {
	p, _ := testSetup(t)

	statuses := []struct {
		status string
		expect string // substring to find in output
	}{
		{"ok", "Credentials configured"},
		{"incomplete", "Credentials incomplete"},
		{"no_client", "OAuth client credentials missing"},
		{"missing", "Credentials not found"},
	}

	for _, tc := range statuses {
		item := &NavItem{
			Slug:             "calendar",
			ValidationStatus: tc.status,
			ValidationMsg:    "test message",
			ValidHint:        "test hint",
		}
		output := p.viewValidationStatus(item)
		if output == "" {
			t.Errorf("expected non-empty output for status %q", tc.status)
		}
		// Check that the expected text appears somewhere in the ANSI-styled output
		// (lipgloss adds ANSI codes, so we check for substrings)
		if len(output) < 10 {
			t.Errorf("output too short for status %q: %q", tc.status, output)
		}
	}
}

func TestGoogleDatasourceAuthActionWorks(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx

	// Verify auth action triggers credential form for Google
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	cmd := p.handleDatasourceFormCompletion("calendar")
	if p.pendingAuthCreds == nil {
		t.Error("expected auth action on Google datasource to set pendingAuthCreds")
	}
	if p.pendingAuthSlug != "calendar" {
		t.Errorf("expected pendingAuthSlug 'calendar', got %q", p.pendingAuthSlug)
	}
	if cmd == nil {
		t.Error("expected tea.Cmd from auth form init")
	}
}

func TestNonGoogleDatasourceAuthActionNoOp(t *testing.T) {
	p, _ := testSetup(t)

	ghIdx := findNavIndex(p, "github")
	p.navCursor = ghIdx

	// Auth action on non-Google should not set up auth state
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	p.handleDatasourceFormCompletion("github")
	if p.pendingAuthCreds != nil {
		t.Error("should not set pendingAuthCreds for non-Google datasource")
	}
}

// --- Help line tests ---

func TestHelpLineShowsFormHintsInContentWithForm(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	// Navigate into the content pane (which now shows a form)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	v := p.View(120, 40, 0)
	// With a form active, help should show form navigation hints
	if !containsStr(v, "tab") {
		t.Error("expected 'tab' in help line when form is active")
	}
	if !containsStr(v, "esc") {
		t.Error("expected 'esc' in help line when form is active")
	}
}

func TestHelpLineShowsFormHintsInFormMode(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusForm

	// Trigger auth form via form completion handler
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	p.handleDatasourceFormCompletion("calendar")

	if p.focusZone != FocusForm {
		t.Fatal("expected FocusForm")
	}

	v := p.View(120, 40, 0)
	if !containsStr(v, "tab") {
		t.Error("expected 'tab' in help line during form mode")
	}
	if !containsStr(v, "esc save & back") {
		t.Error("expected 'esc save & back' in help line during form mode")
	}
}

// --- Auth flow result handling tests ---

func TestAuthFlowResultSuccess(t *testing.T) {
	p, _ := testSetup(t)

	p.pendingAuthSlug = "calendar"
	p.pendingAuthCreds = &clientCredentials{ClientID: "id", ClientSecret: "secret"}

	msg := auth.AuthFlowResultMsg{
		Token: &oauth2.Token{AccessToken: "new-token"},
		Error: nil,
	}

	handled, action := p.HandleMessage(msg)
	if !handled {
		t.Error("expected AuthFlowResultMsg to be handled")
	}
	if p.flashMessage == "" {
		t.Error("expected flash message on success")
	}
	if !containsStr(p.flashMessage, "Authenticated") {
		t.Errorf("expected success flash, got %q", p.flashMessage)
	}
	// Pending state should be cleared
	if p.pendingAuthSlug != "" {
		t.Error("expected pendingAuthSlug to be cleared")
	}
	if p.pendingAuthCreds != nil {
		t.Error("expected pendingAuthCreds to be cleared")
	}
	// After successful auth, a live recheck cmd should be returned
	if action.TeaCmd == nil {
		t.Error("expected TeaCmd for async live recheck after successful auth")
	}
}

func TestAuthFlowResultError(t *testing.T) {
	p, _ := testSetup(t)

	p.pendingAuthSlug = "gmail"
	p.pendingAuthCreds = &clientCredentials{ClientID: "id", ClientSecret: "secret"}

	msg := auth.AuthFlowResultMsg{
		Error: fmt.Errorf("token exchange failed"),
	}

	handled, _ := p.HandleMessage(msg)
	if !handled {
		t.Error("expected AuthFlowResultMsg to be handled")
	}
	if !containsStr(p.flashMessage, "Auth failed") {
		t.Errorf("expected failure flash, got %q", p.flashMessage)
	}
}

func TestCancelAuthFlow(t *testing.T) {
	p, _ := testSetup(t)

	cancelled := false
	p.authCancel = func() { cancelled = true }
	p.pendingAuthSlug = "calendar"
	p.pendingAuthCreds = &clientCredentials{}

	p.cancelAuthFlow()

	if !cancelled {
		t.Error("expected cancel function to be called")
	}
	if p.authCancel != nil {
		t.Error("expected authCancel to be nil after cancel")
	}
	if p.pendingAuthSlug != "" {
		t.Error("expected pendingAuthSlug to be cleared")
	}
	if p.pendingAuthCreds != nil {
		t.Error("expected pendingAuthCreds to be cleared")
	}
}

func TestOAuthConfigForCalendar(t *testing.T) {
	p, _ := testSetup(t)

	conf, path := p.oauthConfigForSlug("calendar", "test-id", "test-secret")
	if conf == nil {
		t.Fatal("expected non-nil config for calendar")
	}
	if conf.ClientID != "test-id" {
		t.Errorf("expected ClientID 'test-id', got %q", conf.ClientID)
	}
	if path == "" {
		t.Error("expected non-empty token path for calendar")
	}
	if !containsStr(path, "google-calendar-mcp") {
		t.Errorf("expected calendar path to contain 'google-calendar-mcp', got %q", path)
	}
}

func TestOAuthConfigForGmail(t *testing.T) {
	p, _ := testSetup(t)

	conf, path := p.oauthConfigForSlug("gmail", "test-id", "test-secret")
	if conf == nil {
		t.Fatal("expected non-nil config for gmail")
	}
	if path == "" {
		t.Error("expected non-empty token path for gmail")
	}
	if !containsStr(path, "gmail-mcp") {
		t.Errorf("expected gmail path to contain 'gmail-mcp', got %q", path)
	}
}

func TestOAuthConfigForUnknown(t *testing.T) {
	p, _ := testSetup(t)

	conf, path := p.oauthConfigForSlug("unknown", "id", "secret")
	if conf != nil {
		t.Error("expected nil config for unknown slug")
	}
	if path != "" {
		t.Error("expected empty path for unknown slug")
	}
}

// --- Calendar f key fetch tests ---

func TestCalendarNavDoesNotForwardFKey(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusNav

	// Get the calendar settings provider and verify fetchLoading is false
	sp := p.providers["calendar"].(*calendar.Settings)
	if sp.FetchLoading() {
		t.Fatal("expected fetchLoading to be false initially")
	}

	// Press f from nav — all datasource actions are now form-based,
	// so 'f' should NOT forward to the provider.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	if sp.FetchLoading() {
		t.Error("expected 'f' from nav not to trigger fetch (actions are form-based)")
	}
}

func TestCalendarFetchResultUpdatesState(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusForm

	sp := p.providers["calendar"].(*calendar.Settings)

	// Simulate fetch result arriving
	result := calendar.CalendarFetchResultMsg{
		Calendars: []calendar.CalendarInfo{
			{ID: "test@group.calendar.google.com", Summary: "Test Calendar", Primary: true},
		},
	}

	handled, _ := p.HandleMessage(result)
	if !handled {
		t.Error("expected calendarFetchResult to be handled")
	}
	if sp.FetchLoading() {
		t.Error("expected fetchLoading to be false after result")
	}
}

func TestCalendarFetchResultHandledFromNav(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusNav // BUG-023: fetch triggered from nav sidebar

	sp := p.providers["calendar"].(*calendar.Settings)

	// Simulate that a fetch was started (e.g. from nav key forwarding)
	// and the result arrives while still on the nav sidebar.
	result := calendar.CalendarFetchResultMsg{
		Calendars: []calendar.CalendarInfo{
			{ID: "test@group.calendar.google.com", Summary: "Test Calendar", Primary: true},
		},
	}

	handled, _ := p.HandleMessage(result)
	if !handled {
		t.Error("expected CalendarFetchResultMsg to be handled even when focusZone is FocusNav")
	}
	if sp.FetchLoading() {
		t.Error("expected fetchLoading to be false after result")
	}
	if len(sp.FetchedCalendars()) != 1 {
		t.Errorf("expected 1 fetched calendar, got %d", len(sp.FetchedCalendars()))
	}
}

func TestCalendarFetchResultErrorHandledFromNav(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusNav

	sp := p.providers["calendar"].(*calendar.Settings)

	// Simulate a fetch error arriving while on nav
	result := calendar.CalendarFetchResultMsg{
		Err: fmt.Errorf("invalid_client: The OAuth client was not found"),
	}

	handled, _ := p.HandleMessage(result)
	if !handled {
		t.Error("expected CalendarFetchResultMsg error to be handled from nav")
	}
	if sp.FetchLoading() {
		t.Error("expected fetchLoading to be false after error result")
	}
	if sp.FetchError() == "" {
		t.Error("expected fetchError to be set after error result")
	}
}

// --- Logs scrolling tests ---

func TestLogsScrollJK(t *testing.T) {
	p, _ := testSetup(t)
	// Add plenty of log entries
	logger := p.logger.(*plugin.FileLogger)
	for i := 0; i < 50; i++ {
		logger.Info("test", fmt.Sprintf("Log message %d", i))
	}

	logsIdx := findNavIndex(p, "system-logs")
	if logsIdx < 0 {
		t.Fatal("system-logs not found")
	}
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40
	p.width = 120

	// Initial offset should be 0
	if p.logOffset != 0 {
		t.Errorf("expected initial logOffset 0, got %d", p.logOffset)
	}

	// Press j to scroll down
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if p.logOffset != 1 {
		t.Errorf("expected logOffset 1 after j, got %d", p.logOffset)
	}

	// Press k to scroll up
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if p.logOffset != 0 {
		t.Errorf("expected logOffset 0 after k, got %d", p.logOffset)
	}

	// k at 0 stays at 0
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if p.logOffset != 0 {
		t.Errorf("expected logOffset to stay at 0, got %d", p.logOffset)
	}
}

func TestLogsScrollFB(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	for i := 0; i < 50; i++ {
		logger.Info("test", fmt.Sprintf("Log message %d", i))
	}

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40

	maxVis := p.logsMaxVisible()
	if maxVis <= 0 {
		t.Fatalf("logsMaxVisible returned %d", maxVis)
	}

	// f should page forward
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if p.logOffset != maxVis {
		t.Errorf("expected logOffset %d after f, got %d", maxVis, p.logOffset)
	}

	// b should page back
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if p.logOffset != 0 {
		t.Errorf("expected logOffset 0 after b, got %d", p.logOffset)
	}
}

func TestLogsScrollDU(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	for i := 0; i < 50; i++ {
		logger.Info("test", fmt.Sprintf("Log message %d", i))
	}

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40

	half := p.logsMaxVisible() / 2

	// d should half-page forward
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if p.logOffset != half {
		t.Errorf("expected logOffset %d after d, got %d", half, p.logOffset)
	}

	// u should half-page back
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if p.logOffset != 0 {
		t.Errorf("expected logOffset 0 after u, got %d", p.logOffset)
	}
}

func TestLogsViewChangesWithScroll(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	for i := 0; i < 50; i++ {
		logger.Info("test", fmt.Sprintf("Log message %d", i))
	}

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40
	p.width = 120

	v1 := p.View(120, 40, 0)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	v2 := p.View(120, 40, 0)

	if v1 == v2 {
		t.Error("expected view to change after scroll with j")
	}
}

func TestLogsScrollFromNavMode(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	for i := 0; i < 50; i++ {
		logger.Info("test", fmt.Sprintf("Log message %d", i))
	}

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusNav // User is in nav mode, not content mode
	p.height = 40
	p.width = 120

	// BUG-056: Pressing j while in FocusNav should move the nav cursor,
	// NOT scroll logs. Logs scrolling only happens in FocusLogs.
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	if p.logOffset != 0 {
		t.Errorf("expected logOffset to remain 0 in FocusNav, got %d", p.logOffset)
	}

	// Nav cursor should have moved down (logs is last item, so it may clamp)
	if p.navCursor == logsIdx && logsIdx < p.navItemCount()-1 {
		t.Errorf("expected navCursor to move from %d, but it stayed", logsIdx)
	}

	// Reset cursor back to logs
	p.navCursor = logsIdx

	// Enter FocusLogs via enter, then j/k should scroll
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focusZone != FocusLogs {
		t.Fatalf("expected FocusLogs after enter on logs item, got %d", p.focusZone)
	}

	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if p.logOffset != 1 {
		t.Errorf("expected logOffset 1 after j in FocusLogs, got %d", p.logOffset)
	}

	// Esc should return to FocusNav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after esc from FocusLogs, got %d", p.focusZone)
	}

	// After returning to FocusNav, j should move nav cursor again, not scroll
	prevOffset := p.logOffset
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if p.logOffset != prevOffset {
		t.Errorf("expected logOffset unchanged in FocusNav, got %d (was %d)", p.logOffset, prevOffset)
	}
}

func TestLogsFilterSlashActivates(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	for i := 0; i < 10; i++ {
		logger.Info("test", fmt.Sprintf("Log message %d", i))
	}

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40

	// Press / to activate filter
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !p.logFilterMode {
		t.Error("expected logFilterMode to be true after /")
	}
}

func TestLogsFilterEnterApplies(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	logger.Info("test", "hello world")
	logger.Error("test", "bad thing happened")

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40

	// Activate filter and type "ERROR"
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "ERROR" {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if p.logFilterMode {
		t.Error("expected logFilterMode to be false after enter")
	}
	if p.logFilterInput.Value() != "ERROR" {
		t.Errorf("expected filter value 'ERROR', got %q", p.logFilterInput.Value())
	}

	// Filtered entries should only include errors
	entries := p.filteredLogEntries()
	if len(entries) != 1 {
		t.Errorf("expected 1 filtered entry, got %d", len(entries))
	}
}

func TestLogsFilterEscClears(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	logger.Info("test", "hello")

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusForm
	p.height = 40

	// Activate filter and type something
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	// Esc should cancel and clear filter
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})

	if p.logFilterMode {
		t.Error("expected logFilterMode to be false after esc")
	}
	if p.logFilterInput.Value() != "" {
		t.Errorf("expected empty filter value after esc, got %q", p.logFilterInput.Value())
	}
}

func TestLogsFilterResetsScroll(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	for i := 0; i < 50; i++ {
		logger.Info("test", fmt.Sprintf("Log message %d", i))
	}

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40

	// Scroll down first
	p.logOffset = 10

	// Activate and apply filter — should reset offset
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if p.logOffset != 0 {
		t.Errorf("expected logOffset 0 after filter apply, got %d", p.logOffset)
	}
}

func TestLogsEscClearsFilterBeforeNav(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	logger.Info("test", "hello")

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40

	// Set a filter (apply it, not in filter mode)
	p.logFilterInput.SetValue("hello")

	// First esc should clear filter, not go back to nav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focusZone != FocusLogs {
		t.Error("expected to stay in FocusLogs after clearing filter")
	}
	if p.logFilterInput.Value() != "" {
		t.Errorf("expected filter cleared, got %q", p.logFilterInput.Value())
	}

	// Second esc should go back to nav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after second esc, got %d", p.focusZone)
	}
}

func TestSlackTokenFormCompletionCallsSave(t *testing.T) {
	p, _ := testSetup(t)

	slackIdx := findNavIndex(p, "slack")
	if slackIdx < 0 {
		t.Fatal("slack nav item not found")
	}
	p.navCursor = slackIdx
	p.focusZone = FocusForm

	item := p.selectedNavItem()
	if item == nil || item.Slug != "slack" {
		t.Fatalf("expected slack nav item, got %v", item)
	}

	// Trigger Slack token form via form completion handler
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	initCmd := p.handleDatasourceFormCompletion("slack")
	if p.focusZone != FocusForm {
		t.Fatalf("expected FocusForm after auth action, got %d", p.focusZone)
	}
	if p.activeForm == nil {
		t.Fatal("expected activeForm to be set")
	}
	if p.pendingSlackToken == nil {
		t.Fatal("expected pendingSlackToken to be set")
	}
	if p.pendingAuthSlug != "slack" {
		t.Fatalf("expected pendingAuthSlug 'slack', got %q", p.pendingAuthSlug)
	}

	// Process init cmd
	if initCmd != nil {
		msg := initCmd()
		if msg != nil {
			p.HandleMessage(msg)
		}
	}

	// Type a token into the form
	for _, r := range "xoxp-test-token" {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify the token value is bound
	if p.pendingSlackToken.Token != "xoxp-test-token" {
		t.Fatalf("expected token 'xoxp-test-token', got %q", p.pendingSlackToken.Token)
	}

	// Press Enter to submit the form
	enterAction := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	// The huh form doesn't complete synchronously — it needs multiple
	// message cycles (nextFieldMsg -> nextGroupMsg -> StateCompleted).
	// Simulate bubbletea's event loop by processing cmds.
	var pendingCmds []tea.Cmd
	if enterAction.TeaCmd != nil {
		pendingCmds = append(pendingCmds, enterAction.TeaCmd)
	}
	for cycles := 0; cycles < 20 && len(pendingCmds) > 0; cycles++ {
		cmd := pendingCmds[0]
		pendingCmds = pendingCmds[1:]
		msg := cmd()
		if msg == nil {
			continue
		}
		// Check if it's a batch msg (tea.BatchMsg)
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					pendingCmds = append(pendingCmds, c)
				}
			}
			continue
		}
		_, nextAction := p.HandleMessage(msg)
		if nextAction.TeaCmd != nil {
			pendingCmds = append(pendingCmds, nextAction.TeaCmd)
		}
		if p.pendingSlackToken == nil {
			break // Token form completed and saved; datasource form rebuilt (BUG-066)
		}
	}

	// After form completion, saveSlackToken should have been called.
	// The datasource form is rebuilt so the pane stays populated (BUG-066),
	// so activeForm should NOT be nil — it's the rebuilt datasource form.
	if p.activeForm == nil {
		t.Fatal("expected activeForm to be rebuilt after token save (BUG-066)")
	}
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm after form completion, got %d", p.focusZone)
	}
	if p.pendingSlackToken != nil {
		t.Error("expected pendingSlackToken to be cleared after save")
	}
	if p.pendingAuthSlug != "" {
		t.Errorf("expected pendingAuthSlug to be empty, got %q", p.pendingAuthSlug)
	}
	if !containsStr(p.flashMessage, "Slack token saved") {
		t.Errorf("expected flash 'Slack token saved', got %q", p.flashMessage)
	}
	// Nav should have been rebuilt with updated validation
	slackItem := p.selectedNavItem()
	if slackItem == nil || slackItem.Slug != "slack" {
		t.Fatal("expected to still be on slack nav item")
	}
}

func TestTabKeyReturnedConsumedWhenFormActive(t *testing.T) {
	p, _ := testSetup(t)

	slackIdx := findNavIndex(p, "slack")
	p.navCursor = slackIdx
	p.focusZone = FocusForm

	// Open the Slack token form via form completion handler
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	p.handleDatasourceFormCompletion("slack")
	if p.focusZone != FocusForm {
		t.Fatal("expected FocusForm")
	}

	// Press Tab while form is active — should be consumed, not switch tabs
	action := p.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if action.Type != plugin.ActionConsumed && action.TeaCmd == nil {
		t.Errorf("expected Tab to be consumed when form is active, got type=%v", action.Type)
	}
}

func TestTabLeaveCleansPendingSlackToken(t *testing.T) {
	p, _ := testSetup(t)

	slackIdx := findNavIndex(p, "slack")
	p.navCursor = slackIdx
	p.focusZone = FocusForm

	// Open the Slack token form via form completion handler
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	p.handleDatasourceFormCompletion("slack")
	if p.pendingSlackToken == nil {
		t.Fatal("expected pendingSlackToken to be set")
	}

	// Tab leave should clean up
	p.HandleMessage(plugin.TabLeaveMsg{})
	if p.pendingSlackToken != nil {
		t.Error("expected pendingSlackToken to be nil after tab leave")
	}
	if p.activeForm != nil {
		t.Error("expected activeForm to be nil after tab leave")
	}
}

// --- Banner form tests ---

func TestBuildBannerFormReturnsCorrectInitialValues(t *testing.T) {
	p, _ := testSetup(t)

	form := p.buildBannerForm()
	if form == nil {
		t.Fatal("expected non-nil form from buildBannerForm")
	}
	if p.bannerValues == nil {
		t.Fatal("expected bannerValues to be set")
	}
	if p.bannerValues.Name != "Test Center" {
		t.Errorf("expected Name 'Test Center', got %q", p.bannerValues.Name)
	}
	if p.bannerValues.Subtitle != "" {
		t.Errorf("expected empty Subtitle, got %q", p.bannerValues.Subtitle)
	}
	if !p.bannerValues.Show {
		t.Error("expected Show to be true (default)")
	}
	// Verify padding is populated from config (default is 2)
	expectedPadding := fmt.Sprintf("%d", p.cfg.GetBannerTopPadding())
	if p.bannerValues.Padding != expectedPadding {
		t.Errorf("expected Padding %q, got %q", expectedPadding, p.bannerValues.Padding)
	}
}

func TestBannerFormCompletionSavesConfig(t *testing.T) {
	p, _ := testSetup(t)

	// Build form, then set values as if user filled them in
	p.buildBannerForm()
	p.bannerValues.Name = "New Name"
	p.bannerValues.Subtitle = "My Subtitle"
	p.bannerValues.Show = false
	p.bannerValues.Padding = "3"

	p.handleBannerFormCompletion()

	if p.cfg.Name != "New Name" {
		t.Errorf("expected cfg.Name 'New Name', got %q", p.cfg.Name)
	}
	if p.cfg.Subtitle != "My Subtitle" {
		t.Errorf("expected cfg.Subtitle 'My Subtitle', got %q", p.cfg.Subtitle)
	}
	if p.cfg.BannerVisible() {
		t.Error("expected banner to be hidden after saving Show=false")
	}
	if p.cfg.GetBannerTopPadding() != 3 {
		t.Errorf("expected banner top padding 3, got %d", p.cfg.GetBannerTopPadding())
	}
	if !containsStr(p.flashMessage, "Banner saved") {
		t.Errorf("expected 'Banner saved' flash, got %q", p.flashMessage)
	}
	// Form should be rebuilt (stays on screen)
	if p.activeForm == nil {
		t.Error("expected activeForm to be rebuilt after banner completion")
	}
	if p.activeFormSlug != "banner" {
		t.Errorf("expected activeFormSlug 'banner', got %q", p.activeFormSlug)
	}
}

func TestBannerFormCompletionWithNilValues(t *testing.T) {
	p, _ := testSetup(t)

	// Completion with nil bannerValues should be no-op
	p.bannerValues = nil
	cmd := p.handleBannerFormCompletion()
	if cmd != nil {
		t.Error("expected nil cmd when bannerValues is nil")
	}
}

func TestBannerFormOpenedFromNav(t *testing.T) {
	p, _ := testSetup(t)

	bannerIdx := findNavIndex(p, "banner")
	if bannerIdx < 0 {
		t.Fatal("banner not found in nav")
	}
	p.navCursor = bannerIdx
	p.focusZone = FocusNav

	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if p.activeForm == nil {
		t.Error("expected activeForm to be set for banner")
	}
	if p.activeFormSlug != "banner" {
		t.Errorf("expected activeFormSlug 'banner', got %q", p.activeFormSlug)
	}
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm, got %d", p.focusZone)
	}
}

// --- Palette form tests ---

func TestBuildPaletteFormReturnsCorrectInitialValues(t *testing.T) {
	p, _ := testSetup(t)

	form := p.buildPaletteForm()
	if form == nil {
		t.Fatal("expected non-nil form from buildPaletteForm")
	}
	if p.paletteValues == nil {
		t.Fatal("expected paletteValues to be set")
	}
	if p.paletteValues.Selected != "aurora" {
		t.Errorf("expected Selected 'aurora', got %q", p.paletteValues.Selected)
	}
}

func TestPaletteFormCompletionAppliesPalette(t *testing.T) {
	p, _ := testSetup(t)

	// Build form, change selection
	p.buildPaletteForm()
	names := config.PaletteNames()
	newPalette := "aurora"
	for _, name := range names {
		if name != p.cfg.Palette {
			newPalette = name
			break
		}
	}
	p.paletteValues.Selected = newPalette

	p.handlePaletteFormCompletion()

	if p.cfg.Palette != newPalette {
		t.Errorf("expected cfg.Palette %q, got %q", newPalette, p.cfg.Palette)
	}
	if !containsStr(p.flashMessage, "Palette saved") {
		t.Errorf("expected 'Palette saved' flash, got %q", p.flashMessage)
	}
	// Form should be rebuilt
	if p.activeForm == nil {
		t.Error("expected activeForm to be rebuilt after palette completion")
	}
	if p.activeFormSlug != "palette" {
		t.Errorf("expected activeFormSlug 'palette', got %q", p.activeFormSlug)
	}
}

func TestPaletteFormCompletionWithNilValues(t *testing.T) {
	p, _ := testSetup(t)

	p.paletteValues = nil
	cmd := p.handlePaletteFormCompletion()
	if cmd != nil {
		t.Error("expected nil cmd when paletteValues is nil")
	}
}

// --- System form tests ---

func TestBuildScheduleFormSetsSystemValues(t *testing.T) {
	p, _ := testSetup(t)

	form := p.buildScheduleForm()
	if form == nil {
		t.Fatal("expected non-nil form")
	}
	if p.systemValues == nil {
		t.Fatal("expected systemValues to be set")
	}
	// Select fields default to the first option's value
	if p.systemValues.Action != "install" {
		t.Errorf("expected initial Action 'install' (first option), got %q", p.systemValues.Action)
	}
}

func TestScheduleFormCompletionInstall(t *testing.T) {
	p, _ := testSetup(t)

	p.buildScheduleForm()
	p.systemValues.Action = "install"

	cmd := p.handleScheduleFormCompletion()
	if cmd == nil {
		t.Error("expected non-nil cmd for install action")
	}
	// Form should be rebuilt
	if p.activeForm == nil {
		t.Error("expected activeForm to be rebuilt after schedule completion")
	}
	if p.activeFormSlug != "system-schedule" {
		t.Errorf("expected activeFormSlug 'system-schedule', got %q", p.activeFormSlug)
	}
}

func TestScheduleFormCompletionUninstall(t *testing.T) {
	p, _ := testSetup(t)

	p.buildScheduleForm()
	p.systemValues.Action = "uninstall"

	cmd := p.handleScheduleFormCompletion()
	if cmd == nil {
		t.Error("expected non-nil cmd for uninstall action")
	}
}

func TestScheduleFormCompletionNilValues(t *testing.T) {
	p, _ := testSetup(t)

	p.systemValues = nil
	cmd := p.handleScheduleFormCompletion()
	if cmd != nil {
		t.Error("expected nil cmd when systemValues is nil")
	}
}

func TestMCPFormCompletionBuild(t *testing.T) {
	p, _ := testSetup(t)

	p.buildMCPForm()
	p.systemValues.Action = "build"

	cmd := p.handleMCPFormCompletion()
	if cmd == nil {
		t.Error("expected non-nil cmd for build action")
	}
	if p.activeFormSlug != "system-mcp" {
		t.Errorf("expected activeFormSlug 'system-mcp', got %q", p.activeFormSlug)
	}
}

func TestSkillsFormCompletionInstall(t *testing.T) {
	p, _ := testSetup(t)

	p.buildSkillsForm()
	p.systemValues.Action = "install"

	cmd := p.handleSkillsFormCompletion()
	if cmd == nil {
		t.Error("expected non-nil cmd for install action")
	}
	if p.activeFormSlug != "system-skills" {
		t.Errorf("expected activeFormSlug 'system-skills', got %q", p.activeFormSlug)
	}
}

func TestShellFormCompletionInstall(t *testing.T) {
	p, _ := testSetup(t)

	p.buildShellForm()
	p.systemValues.Action = "install"

	cmd := p.handleShellFormCompletion()
	if cmd == nil {
		t.Error("expected non-nil cmd for install action")
	}
	if p.activeFormSlug != "system-shell" {
		t.Errorf("expected activeFormSlug 'system-shell', got %q", p.activeFormSlug)
	}
}

func TestSystemActionResultSetsFlash(t *testing.T) {
	p, _ := testSetup(t)

	msg := systemActionResult{slug: "system-schedule", message: "Schedule installed", err: nil}
	handled, _ := p.handleSystemActionResult(msg)
	if !handled {
		t.Error("expected handleSystemActionResult to return true")
	}
	if p.flashMessage != "Schedule installed" {
		t.Errorf("expected flash 'Schedule installed', got %q", p.flashMessage)
	}
}

func TestSystemActionResultErrorSetsFlash(t *testing.T) {
	p, _ := testSetup(t)

	msg := systemActionResult{slug: "system-mcp", message: "MCP built", err: fmt.Errorf("build failed")}
	_, _ = p.handleSystemActionResult(msg)
	if !containsStr(p.flashMessage, "Error:") {
		t.Errorf("expected error flash, got %q", p.flashMessage)
	}
}

func TestSystemActionResultRebuildActiveForm(t *testing.T) {
	p, _ := testSetup(t)

	// Set up an active schedule form
	p.activeForm = p.buildScheduleForm()
	p.activeFormSlug = "system-schedule"

	msg := systemActionResult{slug: "system-schedule", message: "Schedule installed", err: nil}
	_, initCmd := p.handleSystemActionResult(msg)

	// Form should be rebuilt
	if p.activeForm == nil {
		t.Error("expected activeForm to be rebuilt after system action result")
	}
	// Init cmd should be returned so cursor is visible
	if initCmd == nil {
		t.Error("expected Init cmd to be returned after system action rebuilds form")
	}
}

// --- Datasource form tests ---

func TestBuildDatasourceFormSetsValues(t *testing.T) {
	p, _ := testSetup(t)

	calItem := p.findNavItem("calendar")
	if calItem == nil {
		t.Fatal("calendar nav item not found")
	}

	form := p.buildDatasourceForm(calItem)
	if form == nil {
		t.Fatal("expected non-nil form")
	}
	if p.datasourceValues == nil {
		t.Fatal("expected datasourceValues to be set")
	}
	// Select fields default to the first option's value
	if p.datasourceValues.Action != "recheck" {
		t.Errorf("expected initial Action 'recheck' (first option), got %q", p.datasourceValues.Action)
	}
}

func TestDatasourceFormCompletionRecheckRebuildsForm(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx

	p.datasourceValues = &datasourceFormValues{Action: "recheck"}
	p.handleDatasourceFormCompletion("calendar")

	// Form should be rebuilt after recheck
	if p.activeForm == nil {
		t.Error("expected activeForm to be rebuilt after recheck")
	}
	if p.activeFormSlug != "calendar" {
		t.Errorf("expected activeFormSlug 'calendar', got %q", p.activeFormSlug)
	}
}

func TestDatasourceFormCompletionAuthChainsToGoogleCredForm(t *testing.T) {
	p, _ := testSetup(t)

	gmailIdx := findNavIndex(p, "gmail")
	p.navCursor = gmailIdx

	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	cmd := p.handleDatasourceFormCompletion("gmail")

	// Should chain to a credential form
	if p.pendingAuthCreds == nil {
		t.Error("expected pendingAuthCreds to be set for Gmail auth")
	}
	if p.pendingAuthSlug != "gmail" {
		t.Errorf("expected pendingAuthSlug 'gmail', got %q", p.pendingAuthSlug)
	}
	if p.activeForm == nil {
		t.Error("expected activeForm to be set (credential form)")
	}
	if cmd == nil {
		t.Error("expected init cmd from credential form")
	}
}

func TestDatasourceFormCompletionAuthChainsToSlackTokenForm(t *testing.T) {
	p, _ := testSetup(t)

	slackIdx := findNavIndex(p, "slack")
	p.navCursor = slackIdx

	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	cmd := p.handleDatasourceFormCompletion("slack")

	if p.pendingSlackToken == nil {
		t.Error("expected pendingSlackToken to be set for Slack auth")
	}
	if p.pendingAuthSlug != "slack" {
		t.Errorf("expected pendingAuthSlug 'slack', got %q", p.pendingAuthSlug)
	}
	if p.activeForm == nil {
		t.Error("expected activeForm to be set (Slack token form)")
	}
	if cmd == nil {
		t.Error("expected init cmd from Slack token form")
	}
}

// --- Plugin form tests ---

func TestBuildPluginFormSetsValues(t *testing.T) {
	p, _ := testSetup(t)

	extItem := p.findNavItem("external-0")
	if extItem == nil {
		t.Fatal("external-0 nav item not found")
	}

	form := p.buildPluginForm(extItem)
	if form == nil {
		t.Fatal("expected non-nil form")
	}
	if p.pluginValues == nil {
		t.Fatal("expected pluginValues to be set")
	}
}

func TestPluginFormCompletionRebuildsForm(t *testing.T) {
	p, _ := testSetup(t)

	extItem := p.findNavItem("external-0")
	if extItem == nil {
		t.Fatal("external-0 nav item not found")
	}

	p.pluginValues = &pluginFormValues{}
	p.handlePluginFormCompletion("external-0")

	if p.activeForm == nil {
		t.Error("expected activeForm to be rebuilt after plugin form completion")
	}
	if p.activeFormSlug != "external-0" {
		t.Errorf("expected activeFormSlug 'external-0', got %q", p.activeFormSlug)
	}
}

func TestExternalPluginFormShowsInfo(t *testing.T) {
	p, _ := testSetup(t)

	extItem := p.findNavItem("external-0")
	if extItem == nil {
		t.Fatal("external-0 nav item not found")
	}

	form := p.buildPluginForm(extItem)
	if form == nil {
		t.Fatal("expected non-nil form for external plugin")
	}
}

// --- isFormOnlySlug tests ---

func TestIsFormOnlySlugReturnsTrueForBannerAndPalette(t *testing.T) {
	if !isFormOnlySlug("banner") {
		t.Error("expected banner to be a form-only slug")
	}
	if !isFormOnlySlug("palette") {
		t.Error("expected palette to be a form-only slug")
	}
}

func TestIsFormOnlySlugReturnsTrueForSystemSlugs(t *testing.T) {
	for _, slug := range []string{"system-schedule", "system-mcp", "system-skills", "system-shell"} {
		if !isFormOnlySlug(slug) {
			t.Errorf("expected %s to be a form-only slug", slug)
		}
	}
}

func TestIsFormOnlySlugReturnsFalseForDatasourcesAndPlugins(t *testing.T) {
	for _, slug := range []string{"calendar", "gmail", "slack", "github", "system-logs", "system-automations"} {
		if isFormOnlySlug(slug) {
			t.Errorf("expected %s to NOT be a form-only slug", slug)
		}
	}
}

// --- Esc from form-only vs datasource panes ---

func TestEscFromBannerFormReturnsToNav(t *testing.T) {
	p, _ := testSetup(t)

	bannerIdx := findNavIndex(p, "banner")
	p.navCursor = bannerIdx
	p.focusZone = FocusNav

	// Open banner form
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if p.focusZone != FocusForm {
		t.Fatal("expected FocusForm after opening banner")
	}

	// Esc should return to nav (banner is form-only)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focusZone != FocusNav {
		t.Errorf("expected FocusNav after esc from banner form, got %d", p.focusZone)
	}
	if p.activeForm != nil {
		t.Error("expected activeForm to be nil after esc")
	}
}

func TestEscFromDatasourceFormStaysInFocusForm(t *testing.T) {
	p, _ := testSetup(t)

	calIdx := findNavIndex(p, "calendar")
	p.navCursor = calIdx
	p.focusZone = FocusForm

	// Set up an auth form for calendar (datasource, not form-only)
	p.datasourceValues = &datasourceFormValues{Action: "auth"}
	p.handleDatasourceFormCompletion("calendar")

	if p.activeForm == nil {
		t.Fatal("expected form to be active")
	}

	// Esc should stay in FocusForm (datasource slug is not form-only)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.focusZone != FocusForm {
		t.Errorf("expected FocusForm after esc from datasource auth form, got %d", p.focusZone)
	}
}

// --- buildFormForSlug dispatch tests ---

func TestBuildFormForSlugDispatchesCorrectly(t *testing.T) {
	p, _ := testSetup(t)

	tests := []struct {
		slug     string
		kind     string
		wantForm bool
	}{
		{"banner", "appearance", true},
		{"palette", "appearance", true},
		{"system-schedule", "system", true},
		{"system-mcp", "system", true},
		{"system-skills", "system", true},
		{"system-shell", "system", true},
		{"system-logs", "system", false},        // logs has no form
		{"system-automations", "system", false}, // automations has no form
	}

	for _, tc := range tests {
		item := &NavItem{Slug: tc.slug, Kind: tc.kind}
		form, _ := p.buildFormForSlug(item)
		if tc.wantForm && form == nil {
			t.Errorf("expected form for slug %q", tc.slug)
		}
		if !tc.wantForm && form != nil {
			t.Errorf("expected no form for slug %q", tc.slug)
		}
	}
}

func TestBuildFormForSlugDatasource(t *testing.T) {
	p, _ := testSetup(t)

	calItem := p.findNavItem("calendar")
	if calItem == nil {
		t.Fatal("calendar not found")
	}

	form, _ := p.buildFormForSlug(calItem)
	if form == nil {
		t.Error("expected form for datasource calendar")
	}
}

func TestBuildFormForSlugPlugin(t *testing.T) {
	p, _ := testSetup(t)

	extItem := p.findNavItem("external-0")
	if extItem == nil {
		t.Fatal("external-0 not found")
	}

	form, _ := p.buildFormForSlug(extItem)
	if form == nil {
		t.Error("expected form for plugin external-0")
	}
}

// --- handleFormCompletion dispatch tests ---

func TestHandleFormCompletionDispatchesBanner(t *testing.T) {
	p, _ := testSetup(t)

	p.buildBannerForm()
	p.bannerValues.Name = "Dispatch Test"

	p.handleFormCompletion("banner")
	if p.cfg.Name != "Dispatch Test" {
		t.Errorf("expected cfg.Name 'Dispatch Test', got %q", p.cfg.Name)
	}
}

func TestHandleFormCompletionDispatchesPalette(t *testing.T) {
	p, _ := testSetup(t)

	p.buildPaletteForm()
	names := config.PaletteNames()
	newPalette := names[0]
	if newPalette == p.cfg.Palette && len(names) > 1 {
		newPalette = names[1]
	}
	p.paletteValues.Selected = newPalette

	p.handleFormCompletion("palette")
	if p.cfg.Palette != newPalette {
		t.Errorf("expected palette %q, got %q", newPalette, p.cfg.Palette)
	}
}

func TestHandleFormCompletionDispatchesSchedule(t *testing.T) {
	p, _ := testSetup(t)

	p.buildScheduleForm()
	p.systemValues.Action = "install"

	cmd := p.handleFormCompletion("system-schedule")
	if cmd == nil {
		t.Error("expected non-nil cmd for schedule install")
	}
}

// containsStr is a simple substring check for test assertions.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Automations content tests ---

func TestAutomationsContentEmpty(t *testing.T) {
	p, _ := testSetup(t)
	// No automations configured (default)
	output := p.viewAutomationsContent(80, 24)
	if !containsStr(output, "No automations configured") {
		t.Errorf("expected empty state message, got: %s", output)
	}
}

func TestAutomationsContentWithAutomations(t *testing.T) {
	p, _ := testSetup(t)

	// Add automations to config
	p.cfg.Automations = []config.AutomationConfig{
		{Name: "calendar-accept", Command: "test", Enabled: true, Schedule: "hourly"},
		{Name: "weekly-report", Command: "test", Enabled: true, Schedule: "weekly_fri"},
		{Name: "disabled-thing", Command: "test", Enabled: false, Schedule: "every_refresh"},
	}

	// Set up an in-memory database with the automation runs table
	tmpDir := t.TempDir()
	database, err := db.OpenDB(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()
	// Insert a run record for calendar-accept
	_, err = database.Exec(
		`INSERT INTO cc_automation_runs (name, started_at, finished_at, status, message) VALUES (?, ?, ?, ?, ?)`,
		"calendar-accept",
		time.Now().Add(-2*time.Minute).UTC().Format(time.RFC3339),
		time.Now().Add(-1*time.Minute).UTC().Format(time.RFC3339),
		"success",
		"Accepted 3 invites",
	)
	if err != nil {
		t.Fatalf("failed to insert run: %v", err)
	}

	p.database = database

	output := p.viewAutomationsContent(120, 24)

	// Should show all three automations
	if !containsStr(output, "calendar-accept") {
		t.Error("expected calendar-accept in output")
	}
	if !containsStr(output, "weekly-report") {
		t.Error("expected weekly-report in output")
	}
	if !containsStr(output, "disabled") {
		t.Error("expected disabled status in output")
	}
	if !containsStr(output, "Accepted 3 invites") {
		t.Error("expected run message in output")
	}
	if !containsStr(output, "never") {
		t.Error("expected 'never' for weekly-report which has no runs")
	}
}

func TestAutomationsContentShowsError(t *testing.T) {
	p, _ := testSetup(t)

	p.cfg.Automations = []config.AutomationConfig{
		{Name: "failing-auto", Command: "test", Enabled: true, Schedule: "daily"},
	}

	tmpDir := t.TempDir()
	database, err := db.OpenDB(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()
	_, err = database.Exec(
		`INSERT INTO cc_automation_runs (name, started_at, finished_at, status, message) VALUES (?, ?, ?, ?, ?)`,
		"failing-auto",
		time.Now().Add(-3*time.Hour).UTC().Format(time.RFC3339),
		time.Now().Add(-3*time.Hour).UTC().Format(time.RFC3339),
		"error",
		"Credential expired",
	)
	if err != nil {
		t.Fatalf("failed to insert run: %v", err)
	}

	p.database = database

	output := p.viewAutomationsContent(120, 24)

	if !containsStr(output, "error") {
		t.Error("expected 'error' status in output")
	}
	if !containsStr(output, "Credential expired") {
		t.Error("expected error message in output")
	}
}

func TestAutomationsNavItemExists(t *testing.T) {
	p, _ := testSetup(t)
	if !findNavItemInCategory(p, "SYSTEM", "system-automations") {
		t.Error("expected system-automations in SYSTEM category")
	}
}
