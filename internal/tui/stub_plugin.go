package tui

import (
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stubPlugin is a lightweight placeholder for external plugins that are
// enabled in config but were not running at startup. It shows a "restart
// required" message instead of real plugin content.
type stubPlugin struct {
	slug    string
	tabName string
}

func newStubPlugin(slug, tabName string) *stubPlugin {
	return &stubPlugin{slug: slug, tabName: tabName}
}

func (s *stubPlugin) Slug() string    { return s.slug }
func (s *stubPlugin) TabName() string { return s.tabName }

func (s *stubPlugin) Init(_ plugin.Context) error { return nil }
func (s *stubPlugin) Shutdown()                    {}

func (s *stubPlugin) Migrations() []plugin.Migration { return nil }

func (s *stubPlugin) View(width, height, frame int) string {
	msg := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#e0af68")).
		Render("Restart CCC to activate this plugin")
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, msg)
}

func (s *stubPlugin) KeyBindings() []plugin.KeyBinding { return nil }

func (s *stubPlugin) HandleKey(_ tea.KeyMsg) plugin.Action {
	return plugin.NoopAction()
}

func (s *stubPlugin) HandleMessage(_ tea.Msg) (bool, plugin.Action) {
	return false, plugin.NoopAction()
}

func (s *stubPlugin) Routes() []plugin.Route { return nil }

func (s *stubPlugin) NavigateTo(_ string, _ map[string]string) {}

func (s *stubPlugin) RefreshInterval() time.Duration { return 0 }

func (s *stubPlugin) Refresh() tea.Cmd { return nil }
