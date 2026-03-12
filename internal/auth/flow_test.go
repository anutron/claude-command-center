package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestRunAuthFlow_Success(t *testing.T) {
	// Mock token endpoint that returns a valid token.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "credentials.json")

	conf := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "http://localhost/auth", // not used directly in test
			TokenURL: tokenServer.URL,
		},
		Scopes: []string{"test-scope"},
	}

	opts := AuthFlowOpts{
		Config:       conf,
		TokenPath:    tokenPath,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Timeout:      10 * time.Second,
	}

	ctx := context.Background()

	// Run the flow in a goroutine so we can simulate the callback.
	type result struct {
		tok *oauth2.Token
		err error
	}
	resultCh := make(chan result, 1)

	// We need to start the flow, then discover the port and hit the callback.
	// Since runAuthFlow blocks, we run it in a goroutine.
	go func() {
		tok, err := runAuthFlow(ctx, opts)
		resultCh <- result{tok, err}
	}()

	// Give the server a moment to start, then find it and hit the callback.
	// We'll try a few times to hit the callback URL.
	time.Sleep(100 * time.Millisecond)

	// We don't know the exact port, so we read the saved redirect URL concept.
	// Actually, runAuthFlow opens a browser which we can't control in tests.
	// Instead, let's test the pieces: saveTokenFile and cancellation.
	// The full integration requires a browser, so we test cancel and save separately.

	// Cancel the flow.
	// Actually the goroutine is already running. Let's just wait and it will
	// time out or we cancel. For unit tests, let's test the cancellation path.
	t.Skip("Full flow test requires browser interaction; see TestRunAuthFlow_Cancel and TestSaveTokenFile")
}

func TestRunAuthFlow_Cancel(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be reached
		t.Error("token endpoint should not be called on cancel")
	}))
	defer tokenServer.Close()

	conf := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "http://localhost/auth",
			TokenURL: tokenServer.URL,
		},
		Scopes: []string{"test-scope"},
	}

	ctx, cancel := context.WithCancel(context.Background())

	opts := AuthFlowOpts{
		Config:  conf,
		Timeout: 30 * time.Second,
	}

	type result struct {
		tok *oauth2.Token
		err error
	}
	resultCh := make(chan result, 1)

	go func() {
		tok, err := runAuthFlow(ctx, opts)
		resultCh <- result{tok, err}
	}()

	// Give server time to start, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case r := <-resultCh:
		if r.err == nil {
			t.Fatal("expected error on cancel, got nil")
		}
		if !strings.Contains(r.err.Error(), "cancelled") {
			t.Fatalf("expected cancel error, got: %v", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for cancelled flow")
	}
}

func TestRunAuthFlow_Timeout(t *testing.T) {
	conf := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "http://localhost/auth",
			TokenURL: "http://localhost/token",
		},
		Scopes: []string{"test-scope"},
	}

	opts := AuthFlowOpts{
		Config:  conf,
		Timeout: 200 * time.Millisecond,
	}

	tok, err := runAuthFlow(context.Background(), opts)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	if tok != nil {
		t.Fatal("expected nil token on timeout")
	}
}

func TestSaveTokenFile(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "subdir", "credentials.json")

	expiry := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	tok := &oauth2.Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		Expiry:       expiry,
	}

	opts := AuthFlowOpts{
		TokenPath:    tokenPath,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}

	if err := saveTokenFile(opts, tok); err != nil {
		t.Fatalf("saveTokenFile: %v", err)
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read saved token: %v", err)
	}

	var tf GoogleTokenFile
	if err := json.Unmarshal(data, &tf); err != nil {
		t.Fatalf("unmarshal saved token: %v", err)
	}

	if tf.ClientID != "client-id" {
		t.Errorf("ClientID = %q, want %q", tf.ClientID, "client-id")
	}
	if tf.ClientSecret != "client-secret" {
		t.Errorf("ClientSecret = %q, want %q", tf.ClientSecret, "client-secret")
	}
	if tf.AccessToken != "access-123" {
		t.Errorf("AccessToken = %q, want %q", tf.AccessToken, "access-123")
	}
	if tf.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken = %q, want %q", tf.RefreshToken, "refresh-456")
	}
	if tf.ExpiryDate != expiry.UnixMilli() {
		t.Errorf("ExpiryDate = %d, want %d", tf.ExpiryDate, expiry.UnixMilli())
	}
}

func TestSaveTokenFile_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "a", "b", "c", "token.json")

	tok := &oauth2.Token{
		AccessToken: "x",
		TokenType:   "Bearer",
	}
	opts := AuthFlowOpts{
		TokenPath: tokenPath,
	}

	if err := saveTokenFile(opts, tok); err != nil {
		t.Fatalf("saveTokenFile: %v", err)
	}

	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("token file not created: %v", err)
	}
}

func TestAuthFlowCmd_ReturnsResultMsg(t *testing.T) {
	// Verify that AuthFlowCmd wraps runAuthFlow and returns AuthFlowResultMsg.
	// Use a very short timeout so it returns quickly.
	conf := &oauth2.Config{
		ClientID:     "test",
		ClientSecret: "test",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "http://localhost/auth",
			TokenURL: "http://localhost/token",
		},
	}

	cmd := AuthFlowCmd(context.Background(), AuthFlowOpts{
		Config:  conf,
		Timeout: 100 * time.Millisecond,
	})

	msg := cmd()
	result, ok := msg.(AuthFlowResultMsg)
	if !ok {
		t.Fatalf("expected AuthFlowResultMsg, got %T", msg)
	}
	if result.Error == nil {
		t.Fatal("expected error (timeout), got nil")
	}
	fmt.Println("Got expected error:", result.Error)
}
