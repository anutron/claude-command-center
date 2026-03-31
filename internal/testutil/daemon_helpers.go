package testutil

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
)

// StartTestDaemon creates a daemon server with an agent runner and returns
// a connected client. The daemon is automatically shut down when the test ends.
func StartTestDaemon(t *testing.T) *daemon.Client {
	t.Helper()
	dir := t.TempDir()
	d, err := db.OpenDB(filepath.Join(dir, "daemon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	runner := agent.NewRunner(10)
	sockPath := filepath.Join("/tmp", fmt.Sprintf("ccc-test-%d.sock", time.Now().UnixNano()))
	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath:  sockPath,
		DB:          d,
		AgentRunner: runner,
	})
	go srv.Serve()
	t.Cleanup(func() { srv.Shutdown() })

	time.Sleep(50 * time.Millisecond)

	client, err := daemon.NewClient(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// InsertTestTodo inserts a todo into the database for test setup.
func InsertTestTodo(t *testing.T, database *sql.DB, todo db.Todo) {
	t.Helper()
	if err := db.DBInsertTodo(database, todo); err != nil {
		t.Fatalf("insert test todo: %v", err)
	}
}

// InsertTestPR inserts a pull request into the database for test setup.
func InsertTestPR(t *testing.T, database *sql.DB, pr db.PullRequest) {
	t.Helper()
	tx, err := database.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := db.DBSavePullRequests(tx, []db.PullRequest{pr}); err != nil {
		tx.Rollback()
		t.Fatalf("insert test PR: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
}
