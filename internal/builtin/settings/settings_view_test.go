package settings

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// assertViewContains fails the test with a view dump if substr is not found.
func assertViewContains(t *testing.T, view, substr string) {
	t.Helper()
	if !strings.Contains(view, substr) {
		t.Errorf("expected view to contain %q, but it did not.\nView output:\n%s", substr, view)
	}
}

// assertViewNotContains fails the test with a view dump if substr IS found.
func assertViewNotContains(t *testing.T, view, substr string) {
	t.Helper()
	if strings.Contains(view, substr) {
		t.Errorf("expected view NOT to contain %q, but it did.\nView output:\n%s", substr, view)
	}
}

// --- Appearance view tests ---

func TestView_BannerToggleKeepsTabBar(t *testing.T) {
	p, _ := testSetup(t)

	// Render with banner visible (default)
	v1 := p.View(120, 40, 0)
	assertViewContains(t, v1, "APPEARANCE")
	assertViewContains(t, v1, "Banner")

	// Toggle banner visibility off in config
	p.cfg.SetShowBanner(false)

	// The settings view itself should still render correctly — sidebar and content intact
	v2 := p.View(120, 40, 0)
	assertViewContains(t, v2, "APPEARANCE")
	assertViewContains(t, v2, "Banner")
	assertViewContains(t, v2, "Palette")
}

func TestView_PaletteChangeUpdatesView(t *testing.T) {
	p, _ := testSetup(t)

	// Navigate to palette pane and open it
	palIdx := findNavIndex(p, "palette")
	if palIdx < 0 {
		t.Fatal("palette not found in nav")
	}
	p.navCursor = palIdx
	p.focusZone = FocusNav

	// Open palette form
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if p.activeForm == nil {
		t.Fatal("expected palette form to be active")
	}

	v1 := p.View(120, 40, 0)

	// Change palette via form values and trigger completion
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

	v2 := p.View(120, 40, 0)

	// The flash message should indicate the palette was saved
	assertViewContains(t, v2, "Palette saved")

	// After a palette change, the view should differ (new palette name in flash)
	if v1 == v2 {
		t.Error("expected view to change after palette switch")
	}
}

func TestView_BannerNameChangeVisible(t *testing.T) {
	p, _ := testSetup(t)

	// Set custom banner name in config
	p.cfg.Name = "My Custom Dashboard"

	// Navigate to banner and open the form
	bannerIdx := findNavIndex(p, "banner")
	if bannerIdx < 0 {
		t.Fatal("banner not found in nav")
	}
	p.navCursor = bannerIdx
	p.focusZone = FocusNav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if p.activeForm == nil {
		t.Fatal("expected banner form to be active")
	}

	// The form's bound value should reflect the config name
	if p.bannerValues.Name != "My Custom Dashboard" {
		t.Errorf("expected bannerValues.Name 'My Custom Dashboard', got %q", p.bannerValues.Name)
	}

	// Render and check the form shows the custom name
	v := p.View(120, 40, 0)
	assertViewContains(t, v, "My Custom Dashboard")
}

func TestView_TopPaddingAffectsView(t *testing.T) {
	p, _ := testSetup(t)

	// Set a specific top padding
	p.cfg.SetBannerTopPadding(5)

	// Navigate to banner and open form
	bannerIdx := findNavIndex(p, "banner")
	p.navCursor = bannerIdx
	p.focusZone = FocusNav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if p.bannerValues == nil {
		t.Fatal("expected bannerValues to be set")
	}

	// The padding value should be reflected in the form
	if p.bannerValues.Padding != "5" {
		t.Errorf("expected Padding '5', got %q", p.bannerValues.Padding)
	}

	// Render the view and verify the padding value appears
	v := p.View(120, 40, 0)
	assertViewContains(t, v, "5")
}

// --- Plugin management view tests ---

func TestView_PluginDisableRemovesFromNav(t *testing.T) {
	p, _ := testSetup(t)

	// Verify Pomodoro is visible initially
	v1 := p.View(120, 40, 0)
	assertViewContains(t, v1, "Pomodoro")

	// Toggle the external plugin off
	extIdx := findNavIndex(p, "external-0")
	if extIdx < 0 {
		t.Fatal("external plugin not found in nav")
	}
	p.navCursor = extIdx
	p.focusZone = FocusNav
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})

	// Verify the plugin is marked disabled in the sidebar
	v2 := p.View(120, 40, 0)
	// When disabled, the sidebar shows "[off]" next to the plugin
	assertViewContains(t, v2, "[off]")
}

func TestView_PluginEnableAddsToNav(t *testing.T) {
	p, _ := testSetup(t)

	// Disable the plugin first
	extIdx := findNavIndex(p, "external-0")
	if extIdx < 0 {
		t.Fatal("external plugin not found in nav")
	}
	p.navCursor = extIdx
	p.focusZone = FocusNav
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace}) // disable

	v1 := p.View(120, 40, 0)
	assertViewContains(t, v1, "[off]")

	// Re-enable the plugin
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace}) // enable

	v2 := p.View(120, 40, 0)
	// When enabled, should show "[on]"
	assertViewContains(t, v2, "[on]")
	assertViewContains(t, v2, "Pomodoro")
}

func TestView_DataSourceToggleUpdatesStatus(t *testing.T) {
	p, _ := testSetup(t)

	// Calendar is enabled initially — toggle it off
	calIdx := findNavIndex(p, "calendar")
	if calIdx < 0 {
		t.Fatal("calendar not found in nav")
	}
	p.navCursor = calIdx
	p.focusZone = FocusNav

	v1 := p.View(120, 40, 0)
	assertViewContains(t, v1, "[on]")

	// Toggle off
	p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})

	v2 := p.View(120, 40, 0)
	// After toggling, the sidebar should reflect the change
	if v1 == v2 {
		t.Error("expected view to change after toggling data source")
	}
}

// --- Form interaction view tests ---

func TestView_FormFieldEditShowsValue(t *testing.T) {
	p, _ := testSetup(t)

	// Navigate to banner and open form
	bannerIdx := findNavIndex(p, "banner")
	p.navCursor = bannerIdx
	p.focusZone = FocusNav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if p.activeForm == nil {
		t.Fatal("expected banner form to be active")
	}

	// The view should show the current config value in the form
	v := p.View(120, 40, 0)
	assertViewContains(t, v, "Test Center") // the default name from testSetup
	assertViewContains(t, v, "Name")        // field title
}

func TestView_FormCompletionUpdatesPreview(t *testing.T) {
	p, _ := testSetup(t)

	// Open banner form
	bannerIdx := findNavIndex(p, "banner")
	p.navCursor = bannerIdx
	p.focusZone = FocusNav
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	v1 := p.View(120, 40, 0)

	// Simulate form completion with new values
	p.bannerValues.Name = "Updated Name"
	p.handleBannerFormCompletion()

	v2 := p.View(120, 40, 0)

	// The view should now show the updated name and flash message
	assertViewContains(t, v2, "Updated Name")
	assertViewContains(t, v2, "Banner saved")
	if v1 == v2 {
		t.Error("expected view to change after form completion")
	}
}

// --- Logs viewer view tests ---

func TestView_LogsScrollChangesContent(t *testing.T) {
	// NOTE: This overlaps partially with TestLogsViewChangesWithScroll in
	// settings_test.go but verifies it from the view output perspective with
	// specific log message content assertions.
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	for i := 0; i < 50; i++ {
		logger.Info("test", fmt.Sprintf("UniqueLogMsg-%03d", i))
	}

	logsIdx := findNavIndex(p, "system-logs")
	if logsIdx < 0 {
		t.Fatal("system-logs not found")
	}
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40
	p.width = 120

	v1 := p.View(120, 40, 0)
	// Should show some log messages
	assertViewContains(t, v1, "UniqueLogMsg-")

	// Scroll down
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	v2 := p.View(120, 40, 0)

	if v1 == v2 {
		t.Error("expected view to change after scrolling with j")
	}
}

func TestView_LogsFilterNarrowsContent(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	logger.Info("test", "apple-fruit-message")
	logger.Error("test", "banana-error-message")
	logger.Info("test", "cherry-fruit-message")

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40
	p.width = 120

	// Activate filter, type "banana", apply
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "banana" {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	v := p.View(120, 40, 0)
	assertViewContains(t, v, "banana-error-message")
	// Other messages should not appear — filtered out
	assertViewNotContains(t, v, "apple-fruit-message")
	assertViewNotContains(t, v, "cherry-fruit-message")
}

func TestView_LogsFilterClearRestoresAll(t *testing.T) {
	p, _ := testSetup(t)
	logger := p.logger.(*plugin.FileLogger)
	logger.Info("test", "delta-msg-one")
	logger.Error("test", "echo-msg-two")

	logsIdx := findNavIndex(p, "system-logs")
	p.navCursor = logsIdx
	p.focusZone = FocusLogs
	p.height = 40
	p.width = 120

	// Apply a filter for "delta"
	p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "delta" {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	vFiltered := p.View(120, 40, 0)
	assertViewContains(t, vFiltered, "delta-msg-one")
	assertViewNotContains(t, vFiltered, "echo-msg-two")

	// Clear filter with esc (first esc clears filter when filter text is set)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})

	vRestored := p.View(120, 40, 0)
	assertViewContains(t, vRestored, "delta-msg-one")
	assertViewContains(t, vRestored, "echo-msg-two")
}
