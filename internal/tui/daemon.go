package tui

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
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
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	if err := os.MkdirAll(config.DataDir(), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	logPath := filepath.Join(config.DataDir(), "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}

	cmd := exec.Command(exe, "--daemon-internal")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session — survives parent exit.
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}
	logFile.Close()

	// Write PID file so daemon stop/status work.
	pidPath := filepath.Join(config.ConfigDir(), "daemon.pid")
	if err := os.MkdirAll(config.ConfigDir(), 0o755); err != nil {
		log.Printf("WARNING: could not create config dir for PID file: %v", err)
	}
	_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o644)

	// Release the process so it's fully detached.
	_ = cmd.Process.Release()

	logger.Info("daemon", fmt.Sprintf("auto-started daemon (PID: %d)", cmd.Process.Pid))
	return nil
}

// StartDaemonConnection connects to the daemon and starts an event subscription.
// It follows the same pattern as StartNotifyListener: called from main.go after
// tea.NewProgram is created, returns a cleanup function.
// Also sets the Model's daemon fields via the returned DaemonConn.
func StartDaemonConnection(p *tea.Program, logger plugin.Logger, bus plugin.EventBus) *DaemonConn {
	dc := &DaemonConn{
		logger:  logger,
		bus:     bus,
		program: p,
	}

	// Connect the RPC client.
	dc.rpcClient = connectDaemon(logger)
	if dc.rpcClient == nil {
		logger.Info("daemon", "running without daemon connection (not fatal)")
		return dc
	}
	dc.connected.Store(true)

	// Open a second connection for event subscription.
	sockPath := daemonSocketPath()
	subClient, err := daemon.NewClient(sockPath)
	if err != nil {
		logger.Info("daemon", fmt.Sprintf("could not open subscription connection: %v", err))
		return dc
	}
	dc.subClient = subClient

	go func() {
		err := subClient.Subscribe(func(e daemon.Event) {
			p.Send(DaemonEventMsg{Event: e})
		})
		if err != nil {
			logger.Info("daemon", fmt.Sprintf("subscription ended: %v", err))
		}
		p.Send(DaemonDisconnectedMsg{})
	}()

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
