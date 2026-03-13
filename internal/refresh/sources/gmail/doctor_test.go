package gmail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
)

func TestValidateGmailResult_Missing(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	result := ValidateGmailResult()
	if result.Status != "missing" {
		t.Errorf("expected status 'missing', got %q", result.Status)
	}
}

func TestValidateGmailResult_ValidCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", tmpHome)

	dir := filepath.Join(tmpHome, ".gmail-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	token := map[string]interface{}{
		"clientId":      "test-client-id",
		"clientSecret":  "test-secret",
		"access_token":  "test-token",
		"refresh_token": "test-refresh",
	}
	data, _ := json.Marshal(token)
	if err := os.WriteFile(filepath.Join(dir, "work.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateGmailResult()
	if result.Status != "ok" {
		t.Errorf("expected status 'ok', got %q: %s", result.Status, result.Message)
	}
}

func TestValidateGmailResult_MalformedCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", tmpHome)

	dir := filepath.Join(tmpHome, ".gmail-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "work.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateGmailResult()
	if result.Status != "incomplete" {
		t.Errorf("expected status 'incomplete', got %q", result.Status)
	}
}

func TestValidateGmailResult_NoClientCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", tmpHome)
	t.Setenv("GMAIL_CLIENT_ID", "")

	dir := filepath.Join(tmpHome, ".gmail-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Token without clientId
	token := map[string]interface{}{
		"access_token":  "test-token",
		"refresh_token": "test-refresh",
	}
	data, _ := json.Marshal(token)
	if err := os.WriteFile(filepath.Join(dir, "work.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateGmailResult()
	if result.Status != "no_client" {
		t.Errorf("expected status 'no_client', got %q", result.Status)
	}
}

func TestValidateGmailResult_ClientFromEnv(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", tmpHome)
	t.Setenv("GMAIL_CLIENT_ID", "env-client-id")

	dir := filepath.Join(tmpHome, ".gmail-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Token without clientId but env var set
	token := map[string]interface{}{
		"access_token":  "test-token",
		"refresh_token": "test-refresh",
	}
	data, _ := json.Marshal(token)
	if err := os.WriteFile(filepath.Join(dir, "work.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateGmailResult()
	if result.Status != "ok" {
		t.Errorf("expected status 'ok' with env fallback, got %q: %s", result.Status, result.Message)
	}
}

func TestGmailDoctorChecks_Structural(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	d := NewDoctor(config.GmailConfig{})
	checks := d.DoctorChecks(plugin.DoctorOpts{Live: false})
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Name != "Gmail credentials" {
		t.Errorf("expected check name 'Gmail credentials', got %q", checks[0].Name)
	}
	if checks[0].Result.Status != "missing" {
		t.Errorf("expected status 'missing', got %q", checks[0].Result.Status)
	}
}
