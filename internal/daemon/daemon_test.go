package daemon_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestDaemonStartStop(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath: filepath.Join(dir, "daemon.sock"),
		DB:         d,
	})
	go srv.Serve()
	defer srv.Shutdown()

	// Wait for socket to appear
	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(filepath.Join(dir, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	err = client.Ping()
	if err != nil {
		t.Fatal(err)
	}
}
