package gmail

import (
	"os"
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/llm"
)

func testCfg(enabled bool) config.GmailConfig {
	return config.GmailConfig{Enabled: enabled}
}

func TestNew(t *testing.T) {
	s := New(testCfg(true), llm.NoopLLM{})
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestName(t *testing.T) {
	s := New(testCfg(true), llm.NoopLLM{})
	if got := s.Name(); got != "gmail" {
		t.Errorf("Name() = %q, want %q", got, "gmail")
	}
}

func TestEnabled(t *testing.T) {
	s := New(testCfg(true), llm.NoopLLM{})
	if !s.Enabled() {
		t.Error("Enabled() = false, want true for enabled=true")
	}
	s2 := New(testCfg(false), llm.NoopLLM{})
	if s2.Enabled() {
		t.Error("Enabled() = true, want false for enabled=false")
	}
}

func TestLoadGmailAuthMissingFile(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	_, err := loadGmailAuth(false)
	if err == nil {
		t.Fatal("loadGmailAuth() expected error with missing token file, got nil")
	}
	if got := err.Error(); !contains(got, "no gmail token") {
		t.Errorf("loadGmailAuth() error = %q, want it to contain %q", got, "no gmail token")
	}
}

func TestLoadGmailAuthBadJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", home)
	dir := home + "/.gmail-mcp"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/work.json", []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadGmailAuth(false)
	if err == nil {
		t.Fatal("loadGmailAuth() expected error with bad JSON, got nil")
	}
	if got := err.Error(); !contains(got, "parsing gmail token") {
		t.Errorf("loadGmailAuth() error = %q, want it to contain %q", got, "parsing gmail token")
	}
}

func TestLoadGmailAuthMissingClientID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("GMAIL_CLIENT_ID", "")
	dir := home + "/.gmail-mcp"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/work.json", []byte(`{"access_token":"x","refresh_token":"y"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadGmailAuth(false)
	if err == nil {
		t.Fatal("loadGmailAuth() expected error with missing clientId, got nil")
	}
	if got := err.Error(); !contains(got, "missing clientId") {
		t.Errorf("loadGmailAuth() error = %q, want it to contain %q", got, "missing clientId")
	}
}

func TestLoadGmailAuthAdvancedScope(t *testing.T) {
	// Verify that advanced=true doesn't error differently than advanced=false
	// (both should fail the same way with missing file)
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	_, err := loadGmailAuth(true)
	if err == nil {
		t.Fatal("loadGmailAuth(true) expected error with missing token file, got nil")
	}
}

// contains checks if s contains substr.
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
