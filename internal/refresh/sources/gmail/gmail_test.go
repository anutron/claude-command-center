package gmail

import (
	"os"
	"testing"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestName(t *testing.T) {
	s := New()
	if got := s.Name(); got != "gmail" {
		t.Errorf("Name() = %q, want %q", got, "gmail")
	}
}

func TestEnabled(t *testing.T) {
	s := New()
	if !s.Enabled() {
		t.Error("Enabled() = false, want true")
	}
}

func TestLoadGmailAuthMissingFile(t *testing.T) {
	// Point HOME to a temp dir so the token file won't be found.
	t.Setenv("HOME", t.TempDir())
	_, err := loadGmailAuth()
	if err == nil {
		t.Fatal("loadGmailAuth() expected error with missing token file, got nil")
	}
	if got := err.Error(); !contains(got, "no gmail token") {
		t.Errorf("loadGmailAuth() error = %q, want it to contain %q", got, "no gmail token")
	}
}

func TestLoadGmailAuthBadJSON(t *testing.T) {
	// Create a token file with invalid JSON.
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := home + "/.gmail-mcp"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/work.json", []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadGmailAuth()
	if err == nil {
		t.Fatal("loadGmailAuth() expected error with bad JSON, got nil")
	}
	if got := err.Error(); !contains(got, "parsing gmail token") {
		t.Errorf("loadGmailAuth() error = %q, want it to contain %q", got, "parsing gmail token")
	}
}

func TestLoadGmailAuthMissingClientID(t *testing.T) {
	// Create a valid JSON file but with no clientId and no env var.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GMAIL_CLIENT_ID", "")
	dir := home + "/.gmail-mcp"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Valid JSON but missing clientId field.
	if err := os.WriteFile(dir+"/work.json", []byte(`{"access_token":"x","refresh_token":"y"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadGmailAuth()
	if err == nil {
		t.Fatal("loadGmailAuth() expected error with missing clientId, got nil")
	}
	if got := err.Error(); !contains(got, "missing clientId") {
		t.Errorf("loadGmailAuth() error = %q, want it to contain %q", got, "missing clientId")
	}
}

// contains checks if s contains substr (avoids importing strings for one use).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
