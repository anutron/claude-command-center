package daemon_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/daemon"
)

func TestSubscribeReceivesEvents(t *testing.T) {
	dir := t.TempDir()
	d := testDB(t)
	sockPath := filepath.Join(dir, "daemon.sock")
	srv := daemon.NewServer(daemon.ServerConfig{
		SocketPath: sockPath,
		DB:         d,
	})
	go srv.Serve()
	defer srv.Shutdown()
	time.Sleep(50 * time.Millisecond)

	// Create a subscriber client (dedicated connection)
	subClient, err := daemon.NewClient(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer subClient.Close()

	events := make(chan daemon.Event, 10)
	go subClient.Subscribe(func(e daemon.Event) {
		events <- e
	})
	time.Sleep(50 * time.Millisecond) // let subscribe register

	// Broadcast a test event from the server
	srv.Broadcast(daemon.Event{Type: "test.event"})

	select {
	case evt := <-events:
		if evt.Type != "test.event" {
			t.Fatalf("expected test.event, got %s", evt.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}
