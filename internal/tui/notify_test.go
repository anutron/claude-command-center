package tui

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSocketPath(t *testing.T) {
	path := SocketPath()
	if path == "" {
		t.Error("SocketPath() returned empty string")
	}
	if !strings.HasSuffix(path, ".sock") {
		t.Errorf("SocketPath() should end with .sock, got %q", path)
	}
	expected := fmt.Sprintf("ccc-%d.sock", os.Getpid())
	if !strings.HasSuffix(path, expected) {
		t.Errorf("SocketPath() should contain PID, got %q", path)
	}
}

func TestSocketPathRespectsEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_STATE_DIR", tmp)

	path := SocketPath()
	if !strings.HasPrefix(path, tmp) {
		t.Errorf("SocketPath() should use CCC_STATE_DIR, got %q", path)
	}
}

func TestSendNotifyNoInstances(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_STATE_DIR", tmp)

	err := SendNotify("reload")
	if err == nil {
		t.Error("expected error when no instances running")
	}
	if !strings.Contains(err.Error(), "no running CCC instances") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSendNotifyReachesListener(t *testing.T) {
	// Use /tmp directly to keep unix socket path short (macOS has ~104 char limit)
	tmp, err := os.MkdirTemp("/tmp", "ccc-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)
	t.Setenv("CCC_STATE_DIR", tmp)

	// Create a mock socket
	sockPath := filepath.Join(tmp, fmt.Sprintf("ccc-%d.sock", os.Getpid()))
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create test socket: %v", err)
	}
	defer ln.Close()
	defer os.Remove(sockPath)

	// Listen for a message in background
	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		received <- strings.TrimSpace(string(buf[:n]))
	}()

	// Send notification
	err = SendNotify("reload")
	if err != nil {
		t.Fatalf("SendNotify failed: %v", err)
	}

	msg := <-received
	if msg != "reload" {
		t.Errorf("expected 'reload', got %q", msg)
	}
}

func TestSendNotifyCleansStaleSockets(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_STATE_DIR", tmp)

	// Create a stale socket file (not listening)
	stalePath := filepath.Join(tmp, "ccc-99999.sock")
	os.WriteFile(stalePath, []byte{}, 0o644)

	// Should fail but clean up the stale socket
	_ = SendNotify("reload")

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Error("stale socket should have been cleaned up")
	}
}
