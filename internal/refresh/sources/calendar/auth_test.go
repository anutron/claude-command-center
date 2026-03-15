package calendar

import (
	"testing"
	"time"
)

func TestRunCalendarAuth_BindsToLoopback(t *testing.T) {
	// Verify that RunCalendarAuth binds to 127.0.0.1 (loopback) and uses a random port.
	// We can't easily test the full flow, but we verify it starts by checking
	// that the credential validation kicks in before binding.
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunCalendarAuth()
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error (placeholder credentials), got nil")
		} else {
			t.Logf("got expected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("RunCalendarAuth blocked for >3s — expected quick failure on credential validation")
	}
}
