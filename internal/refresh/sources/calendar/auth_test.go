package calendar

import (
	"net"
	"testing"
	"time"
)

func TestRunCalendarAuth_PortInUse(t *testing.T) {
	// Set dummy credentials to get past the credential check
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")

	// Pre-bind port 3000 to make ListenAndServe fail
	listener, err := net.Listen("tcp", ":3000")
	if err != nil {
		t.Skipf("cannot bind port 3000 (already in use): %v", err)
	}
	defer listener.Close()

	// RunCalendarAuth should fail quickly with "address already in use"
	// BUG: The error from go srv.ListenAndServe() is discarded on line 245,
	// so the function blocks for 5 minutes on the timeout instead of failing fast.
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunCalendarAuth()
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error when port is in use, got nil")
		} else {
			t.Logf("got expected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("RunCalendarAuth blocked for >3s instead of reporting port-in-use error — ListenAndServe error is silently discarded")
	}
}
