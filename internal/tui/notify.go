package tui

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// SocketPath returns the path for the notification unix socket.
// Each process gets its own socket file based on PID.
func SocketPath() string {
	dir := os.Getenv("CCC_STATE_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("WARNING: cannot determine home directory: %v", err)
			return ""
		}
		dir = filepath.Join(home, ".config", "ccc", "data")
	}
	return filepath.Join(dir, fmt.Sprintf("ccc-%d.sock", os.Getpid()))
}

// sockDir returns the directory where socket files live.
func sockDir() string {
	dir := os.Getenv("CCC_STATE_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("WARNING: cannot determine home directory: %v", err)
			return ""
		}
		dir = filepath.Join(home, ".config", "ccc", "data")
	}
	return dir
}

// StartNotifyListener starts a unix socket listener that sends plugin.NotifyMsg
// into the bubbletea program when a client connects and writes an event.
// Returns a cleanup function that removes the socket file.
func StartNotifyListener(p *tea.Program) func() {
	sockPath := SocketPath()
	_ = os.MkdirAll(filepath.Dir(sockPath), 0o700)

	// Clean up stale socket if it exists
	_ = os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return func() {}
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go handleNotifyConn(conn, p)
		}
	}()

	return func() {
		ln.Close()
		os.Remove(sockPath)
	}
}

func handleNotifyConn(conn net.Conn, p *tea.Program) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		event := strings.TrimSpace(scanner.Text())
		if event != "" {
			p.Send(plugin.NotifyMsg{Event: event})
		}
	}
}

// SendNotify connects to all running CCC instances and sends an event.
func SendNotify(event string) error {
	dir := sockDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read state dir: %w", err)
	}

	sent := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "ccc-") || !strings.HasSuffix(name, ".sock") {
			continue
		}
		sockPath := filepath.Join(dir, name)
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			// Stale socket — clean up
			os.Remove(sockPath)
			continue
		}
		fmt.Fprintf(conn, "%s\n", event)
		conn.Close()
		sent++
	}

	if sent == 0 {
		return fmt.Errorf("no running CCC instances found")
	}
	fmt.Printf("Notified %d instance(s): %s\n", sent, event)
	return nil
}
