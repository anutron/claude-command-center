package external

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// ExternalPlugin adapts an external subprocess into a plugin.Plugin.
type ExternalPlugin struct {
	proc            *Process
	command         string
	slug            string
	tabName         string
	routes          []plugin.Route
	keys            []plugin.KeyBinding
	migrations      []plugin.Migration
	refreshInterval time.Duration
	cachedView      string
	errState        string // non-empty = crashed
	ctx             plugin.Context
	width, height   int
}

// Slug returns the plugin identifier.
func (ep *ExternalPlugin) Slug() string { return ep.slug }

// TabName returns the display name for the tab.
func (ep *ExternalPlugin) TabName() string { return ep.tabName }

// Routes returns the plugin's declared routes.
func (ep *ExternalPlugin) Routes() []plugin.Route { return ep.routes }

// KeyBindings returns the plugin's declared key bindings.
func (ep *ExternalPlugin) KeyBindings() []plugin.KeyBinding { return ep.keys }

// Migrations returns the plugin's declared database migrations.
func (ep *ExternalPlugin) Migrations() []plugin.Migration { return ep.migrations }

// RefreshInterval returns the plugin's preferred refresh interval.
func (ep *ExternalPlugin) RefreshInterval() time.Duration { return ep.refreshInterval }

// Init saves the context and starts the subprocess.
func (ep *ExternalPlugin) Init(ctx plugin.Context) error {
	ep.ctx = ctx
	return ep.startProcess()
}

func (ep *ExternalPlugin) startProcess() error {
	// Check if the command exists before attempting to launch.
	// Extract the first word as the binary name (ignore arguments).
	cmdName := ep.command
	if parts := strings.Fields(ep.command); len(parts) > 0 {
		cmdName = parts[0]
	}
	if _, err := exec.LookPath(cmdName); err != nil {
		ep.errState = fmt.Sprintf("command %q not found on PATH", cmdName)
		return fmt.Errorf("command not found: %w", err)
	}

	proc := &Process{}
	if err := proc.Start(ep.command, ep.ctx.Logger); err != nil {
		ep.errState = fmt.Sprintf("failed to start: %v", err)
		return fmt.Errorf("start process: %w", err)
	}

	if err := proc.Send(HostMsg{
		Type:   "init",
		DBPath: ep.ctx.DBPath,
		Width:  ep.width,
		Height: ep.height,
	}); err != nil {
		proc.Kill()
		ep.errState = fmt.Sprintf("failed to send init: %v", err)
		return fmt.Errorf("send init: %w", err)
	}

	resp, err := proc.Receive(5 * time.Second)
	if err != nil {
		proc.Kill()
		ep.errState = fmt.Sprintf("init handshake failed: %v", err)
		return fmt.Errorf("receive ready: %w", err)
	}

	if resp.Type != "ready" {
		proc.Kill()
		ep.errState = fmt.Sprintf("expected ready, got %s", resp.Type)
		return fmt.Errorf("expected ready message, got %q", resp.Type)
	}

	// Send scoped config: only the top-level config sections the plugin declared.
	// If no config_scopes declared, send no config (secure by default).
	scopedCfg := plugin.ScopeConfig(ep.ctx.Config, resp.ConfigScopes)
	cfgJSON, err := json.Marshal(scopedCfg)
	if err != nil {
		proc.Kill()
		ep.errState = fmt.Sprintf("failed to marshal config: %v", err)
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := proc.Send(HostMsg{Type: "config", Config: cfgJSON}); err != nil {
		proc.Kill()
		ep.errState = fmt.Sprintf("failed to send config: %v", err)
		return fmt.Errorf("send config: %w", err)
	}

	ep.proc = proc
	ep.slug = resp.Slug
	ep.tabName = resp.TabName
	ep.refreshInterval = time.Duration(resp.RefreshMS) * time.Millisecond
	proc.slug = resp.Slug

	// Convert routes
	ep.routes = make([]plugin.Route, len(resp.Routes))
	for i, r := range resp.Routes {
		ep.routes[i] = plugin.Route{
			Slug:        r.Slug,
			Description: r.Description,
			ArgKeys:     r.ArgKeys,
		}
	}

	// Convert key bindings
	ep.keys = make([]plugin.KeyBinding, len(resp.KeyBindings))
	for i, kb := range resp.KeyBindings {
		ep.keys[i] = plugin.KeyBinding{
			Key:         kb.Key,
			Description: kb.Description,
			Mode:        kb.Mode,
			Promoted:    kb.Promoted,
		}
	}

	// Convert migrations
	ep.migrations = make([]plugin.Migration, len(resp.Migrations))
	for i, m := range resp.Migrations {
		ep.migrations[i] = plugin.Migration{
			Version: m.Version,
			SQL:     m.SQL,
		}
	}

	// Validate external plugin migrations (only DDL namespaced to slug allowed)
	for _, m := range ep.migrations {
		if err := plugin.ValidateExternalMigrationSQL(ep.slug, m.SQL); err != nil {
			if ep.ctx.Logger != nil {
				ep.ctx.Logger.Warn(ep.slug, fmt.Sprintf("migration v%d rejected: %s", m.Version, err.Error()))
			}
			return fmt.Errorf("migration v%d for %s: %w", m.Version, ep.slug, err)
		}
	}

	// Run migrations
	if err := plugin.RunMigrations(ep.ctx.DB, ep.slug, ep.migrations); err != nil {
		if ep.ctx.Logger != nil {
			ep.ctx.Logger.Warn(ep.slug, "migration error: "+err.Error())
		}
	}

	ep.errState = ""
	return nil
}

// Shutdown sends a shutdown message and kills the process if it doesn't exit.
func (ep *ExternalPlugin) Shutdown() {
	if ep.proc == nil {
		return
	}
	_ = ep.proc.Send(HostMsg{Type: "shutdown"})

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ep.proc.done:
	case <-timer.C:
		ep.proc.Kill()
	}
}

// View requests a rendered frame from the plugin.
func (ep *ExternalPlugin) View(width, height, frame int) string {
	ep.width = width
	ep.height = height

	if ep.errState != "" {
		return ep.errorView()
	}

	err := ep.proc.Send(HostMsg{
		Type:   "render",
		Width:  width,
		Height: height,
		Frame:  frame,
	})
	if err != nil {
		ep.errState = fmt.Sprintf("send error: %v", err)
		return ep.errorView()
	}

	resp, err := ep.proc.Receive(50 * time.Millisecond)
	if err != nil {
		// On timeout just return cached; on process death set error
		if ep.proc != nil && !ep.proc.Alive() {
			ep.errState = fmt.Sprintf("process exited unexpectedly")
		}
		return ep.cachedView
	}

	if resp.Type == "view" {
		ep.cachedView = resp.Content
	}
	return ep.cachedView
}

// HandleKey sends a key event to the plugin and returns the resulting action.
func (ep *ExternalPlugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	if ep.errState != "" {
		if msg.String() == "r" {
			if ep.proc != nil {
				ep.proc.Kill()
			}
			if err := ep.startProcess(); err != nil {
				if ep.ctx.Logger != nil {
					ep.ctx.Logger.Error(ep.slug, "restart failed: "+err.Error())
				}
			}
			return plugin.NoopAction()
		}
		return plugin.NoopAction()
	}

	err := ep.proc.Send(HostMsg{
		Type: "key",
		Key:  msg.String(),
		Alt:  msg.Alt,
	})
	if err != nil {
		return plugin.NoopAction()
	}

	resp, err := ep.proc.Receive(50 * time.Millisecond)
	if err != nil {
		return plugin.NoopAction()
	}

	if resp.Type == "action" {
		return plugin.Action{
			Type:    resp.Action,
			Payload: resp.APayload,
			Args:    resp.AArgs,
		}
	}
	return plugin.NoopAction()
}

// HandleMessage processes bubbletea messages and drains async plugin output.
func (ep *ExternalPlugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		ep.width = m.Width
		ep.height = m.Height
		return false, plugin.NoopAction()
	default:
		if ep.proc == nil || ep.errState != "" {
			return false, plugin.NoopAction()
		}
		msgs := ep.proc.DrainAsync()
		for _, pm := range msgs {
			switch pm.Type {
			case "event":
				if ep.ctx.Bus != nil {
					// Auto-prefix event topics with the plugin's slug to prevent
					// external plugins from impersonating built-in event topics.
					topic := ep.slug + ":" + pm.Topic2
					ep.ctx.Bus.Publish(plugin.Event{
						Source:  ep.slug,
						Topic:   topic,
						Payload: pm.EPayload,
					})
				}
			case "log":
				if ep.ctx.Logger != nil {
					switch pm.Level {
					case "error":
						ep.ctx.Logger.Error(ep.slug, pm.Message)
					case "warn":
						ep.ctx.Logger.Warn(ep.slug, pm.Message)
					default:
						ep.ctx.Logger.Info(ep.slug, pm.Message)
					}
				}
			}
		}
		return false, plugin.NoopAction()
	}
}

// Refresh returns a tea.Cmd that sends a refresh message to the plugin.
func (ep *ExternalPlugin) Refresh() tea.Cmd {
	return func() tea.Msg {
		if ep.proc != nil && ep.errState == "" {
			_ = ep.proc.Send(HostMsg{Type: "refresh"})
		}
		return nil
	}
}

// NavigateTo sends a navigate message to the plugin.
func (ep *ExternalPlugin) NavigateTo(route string, args map[string]string) {
	if ep.proc != nil && ep.errState == "" {
		_ = ep.proc.Send(HostMsg{
			Type:  "navigate",
			Route: route,
			Args:  args,
		})
	}
}

func (ep *ExternalPlugin) errorView() string {
	name := ep.slug
	if name == "" {
		name = ep.tabName
	}
	if name == "" {
		name = ep.command
	}

	notFound := strings.Contains(ep.errState, "not found on PATH") ||
		strings.Contains(ep.errState, "exit status 127")

	if notFound {
		return fmt.Sprintf(
			"\n  Plugin %q — not installed\n\n  Command: %s\n  Error: %s\n\n  Press 'r' to retry\n",
			name, ep.command, ep.errState,
		)
	}

	return fmt.Sprintf(
		"\n  Plugin %q crashed\n\n  Command: %s\n  Error: %s\n\n  Press 'r' to restart\n",
		name, ep.command, ep.errState,
	)
}
