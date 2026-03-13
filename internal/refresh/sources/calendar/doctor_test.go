package calendar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/anutron/claude-command-center/internal/plugin"
)

func TestValidateCalendarResult_Missing(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	result := ValidateCalendarResult()
	if result.Status != "missing" {
		t.Errorf("expected status 'missing', got %q", result.Status)
	}
}

func TestValidateCalendarResult_ValidCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", tmpHome)

	dir := filepath.Join(tmpHome, ".config", "google-calendar-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	creds := map[string]interface{}{
		"clientId":      "test-client-id",
		"clientSecret":  "test-secret",
		"access_token":  "test-token",
		"refresh_token": "test-refresh",
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateCalendarResult()
	if result.Status != "ok" {
		t.Errorf("expected status 'ok', got %q: %s", result.Status, result.Message)
	}
}

func TestValidateCalendarResult_MalformedCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", tmpHome)

	dir := filepath.Join(tmpHome, ".config", "google-calendar-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateCalendarResult()
	if result.Status != "incomplete" {
		t.Errorf("expected status 'incomplete', got %q", result.Status)
	}
}

func TestValidateCalendarResult_LegacyToken(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", tmpHome)

	dir := filepath.Join(tmpHome, ".config", "google-calendar-mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No credentials.json, but token.json exists
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte(`{"access_token":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateCalendarResult()
	if result.Status != "ok" {
		t.Errorf("expected status 'ok' for legacy token, got %q", result.Status)
	}
	if result.Hint == "" {
		t.Error("expected hint about migration for legacy token")
	}
}

func TestCalendarDoctorChecks_Structural(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	s := &Settings{}
	checks := s.DoctorChecks(plugin.DoctorOpts{Live: false})
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Name != "Calendar credentials" {
		t.Errorf("expected check name 'Calendar credentials', got %q", checks[0].Name)
	}
	if checks[0].Result.Status != "missing" {
		t.Errorf("expected status 'missing', got %q", checks[0].Result.Status)
	}
}
