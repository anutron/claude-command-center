package daemon_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
)

func TestRefreshRPC(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	sockPath := filepath.Join(dir, "d.sock")

	refreshCalled := atomic.Bool{}
	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:      sockPath,
		DB:              d,
		RefreshFunc:     func() error { refreshCalled.Store(true); return nil },
		RefreshInterval: 0, // disable timer for this test
	})
	go srv.Serve()
	t.Cleanup(srv.Shutdown)
	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	err = client.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // give async refresh time
	if !refreshCalled.Load() {
		t.Fatal("refresh was not called")
	}
}
