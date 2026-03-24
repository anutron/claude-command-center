package tui

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// DaemonEventMsg wraps a daemon event pushed via subscription.
type DaemonEventMsg struct {
	Event daemon.Event
}

// DaemonDisconnectedMsg signals that the daemon subscription connection was lost.
type DaemonDisconnectedMsg struct{}

// daemonReconnectMsg is an internal tick that triggers a reconnect attempt.
type daemonReconnectMsg struct{}

const daemonReconnectInterval = 10 * time.Second

// daemonSocketPath returns the path to the daemon socket.
func daemonSocketPath() string {
	return filepath.Join(config.ConfigDir(), "daemon.sock")
}

// connectDaemon attempts to connect to the daemon, auto-starting it if needed.
// Returns a connected client or nil if the daemon is unreachable.
func connectDaemon(logger plugin.Logger) *daemon.Client {
	sockPath := daemonSocketPath()

	// First attempt: try to connect directly.
	client, err := daemon.NewClient(sockPath)
	if err == nil {
		if pingErr := client.Ping(); pingErr == nil {
			return client
		}
		client.Close()
	}

	// Daemon not running — try to auto-start it.
	if startErr := autoStartDaemon(logger); startErr != nil {
		logger.Info("daemon", fmt.Sprintf("auto-start failed: %v", startErr))
		return nil
	}

	// Wait briefly for daemon to initialize, then retry.
	time.Sleep(500 * time.Millisecond)

	client, err = daemon.NewClient(sockPath)
	if err != nil {
		logger.Info("daemon", fmt.Sprintf("connect after auto-start failed: %v", err))
		return nil
	}
	if pingErr := client.Ping(); pingErr != nil {
		client.Close()
		logger.Info("daemon", fmt.Sprintf("ping after auto-start failed: %v", pingErr))
		return nil
	}
	return client
}

// autoStartDaemon spawns the daemon as a detached background process.
func autoStartDaemon(logger plugin.Logger) error {
	if err := daemon.StartProcess(); err != nil {
		return err
	}
	logger.Info("daemon", "auto-started daemon")
	return nil
}

// NewDaemonConn creates an unconnected DaemonConn. Call Connect(p) after
// tea.NewProgram to establish the connection and start event subscription.
// This two-phase init ensures the pointer is shared with the bubbletea model
// copy (since Model is a value type copied by NewProgram).
func NewDaemonConn(logger plugin.Logger, bus plugin.EventBus) *DaemonConn {
	return &DaemonConn{
		logger: logger,
		bus:    bus,
	}
}

// Connect establishes the daemon RPC connection and starts event subscription.
func (dc *DaemonConn) Connect(p *tea.Program) {
	dc.program = p

	// Connect the RPC client.
	dc.rpcClient = connectDaemon(dc.logger)
	if dc.rpcClient == nil {
		dc.logger.Info("daemon", "running without daemon connection (not fatal)")
		return
	}
	dc.connected.Store(true)

	// Open a second connection for event subscription.
	sockPath := daemonSocketPath()
	subClient, err := daemon.NewClient(sockPath)
	if err != nil {
		dc.logger.Info("daemon", fmt.Sprintf("could not open subscription connection: %v", err))
		return
	}
	dc.subClient = subClient

	go func() {
		err := subClient.Subscribe(func(e daemon.Event) {
			p.Send(DaemonEventMsg{Event: e})
		})
		if err != nil {
			dc.logger.Info("daemon", fmt.Sprintf("subscription ended: %v", err))
		}
		p.Send(DaemonDisconnectedMsg{})
	}()
}

// StartDaemonConnection is a convenience wrapper for backward compatibility.
// Prefer NewDaemonConn + Connect for new code.
func StartDaemonConnection(p *tea.Program, logger plugin.Logger, bus plugin.EventBus) *DaemonConn {
	dc := NewDaemonConn(logger, bus)
	dc.Connect(p)
	return dc
}

// DaemonConn holds the daemon connection state, accessible from main.go and the Model.
type DaemonConn struct {
	rpcClient *daemon.Client
	subClient *daemon.Client
	connected atomic.Bool
	logger    plugin.Logger
	bus       plugin.EventBus
	program   *tea.Program // stored for reconnection subscription goroutine
}

// Client returns the RPC client, or nil if not connected.
func (dc *DaemonConn) Client() *daemon.Client {
	if dc == nil {
		return nil
	}
	return dc.rpcClient
}

// Connected returns whether the daemon connection is active.
func (dc *DaemonConn) Connected() bool {
	if dc == nil {
		return false
	}
	return dc.connected.Load()
}

// Close shuts down both connections.
func (dc *DaemonConn) Close() {
	if dc == nil {
		return
	}
	if dc.subClient != nil {
		dc.subClient.Close()
	}
	if dc.rpcClient != nil {
		dc.rpcClient.Close()
	}
}

// Reconnect attempts to re-establish the daemon connection. Called from the
// reconnect tick handler. Returns true on success.
func (dc *DaemonConn) Reconnect() bool {
	if dc == nil {
		return false
	}

	// Close stale connections.
	if dc.subClient != nil {
		dc.subClient.Close()
		dc.subClient = nil
	}
	if dc.rpcClient != nil {
		dc.rpcClient.Close()
		dc.rpcClient = nil
	}

	dc.rpcClient = connectDaemon(dc.logger)
	if dc.rpcClient == nil {
		dc.connected.Store(false)
		return false
	}

	// Re-open subscription.
	sockPath := daemonSocketPath()
	subClient, err := daemon.NewClient(sockPath)
	if err != nil {
		dc.logger.Info("daemon", fmt.Sprintf("reconnect: subscription connection failed: %v", err))
		dc.connected.Store(false)
		return false
	}
	dc.subClient = subClient
	dc.connected.Store(true)

	go func() {
		err := subClient.Subscribe(func(e daemon.Event) {
			dc.program.Send(DaemonEventMsg{Event: e})
		})
		if err != nil {
			dc.logger.Info("daemon", fmt.Sprintf("subscription ended after reconnect: %v", err))
		}
		dc.program.Send(DaemonDisconnectedMsg{})
	}()

	dc.logger.Info("daemon", "reconnected to daemon")
	return true
}

// daemonReconnectCmd returns a tea.Cmd that ticks after the reconnect interval.
func daemonReconnectCmd() tea.Cmd {
	return tea.Tick(daemonReconnectInterval, func(time.Time) tea.Msg {
		return daemonReconnectMsg{}
	})
}

// routeDaemonEvent converts a daemon event to an event bus publication.
func routeDaemonEvent(bus plugin.EventBus, evt daemon.Event) {
	bus.Publish(plugin.Event{
		Source:  "daemon",
		Topic:   evt.Type,
		Payload: evt.Data,
	})
}
