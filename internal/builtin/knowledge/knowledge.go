// Package knowledge implements the knowledge plugin for CCC.
// It extracts structured knowledge artifacts from source material
// and surfaces proactive insights (silence alerts, drift detection).
package knowledge

import (
	"database/sql"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// Plugin implements plugin.Plugin for the knowledge layer.
type Plugin struct {
	database *sql.DB
	cfg      *config.Config
	bus      plugin.EventBus
	llm      llm.LLM
}

// New creates a new knowledge plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Slug() string    { return "knowledge" }
func (p *Plugin) TabName() string { return "" }

func (p *Plugin) Init(ctx plugin.Context) error {
	p.database = ctx.DB
	p.cfg = ctx.Config
	p.bus = ctx.Bus
	p.llm = ctx.LLM
	return nil
}

func (p *Plugin) Shutdown() {}

// Migrations returns the knowledge plugin's table migrations.
// Stub: returns nil – Stage 3 will implement the real migrations.
func (p *Plugin) Migrations() []plugin.Migration {
	return nil
}

func (p *Plugin) View(width, height, frame int) string { return "" }
func (p *Plugin) KeyBindings() []plugin.KeyBinding     { return nil }
func (p *Plugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	return plugin.NoopAction()
}
func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	return false, plugin.NoopAction()
}
func (p *Plugin) Routes() []plugin.Route         { return nil }
func (p *Plugin) NavigateTo(route string, args map[string]string) {}
func (p *Plugin) RefreshInterval() time.Duration { return 0 }
func (p *Plugin) Refresh() tea.Cmd               { return nil }
