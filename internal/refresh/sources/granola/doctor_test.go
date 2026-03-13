package granola

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
)

func TestGranolaDoctorChecks_Missing(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	s := &Settings{cfg: &config.Config{}}
	checks := s.DoctorChecks(plugin.DoctorOpts{})
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Name != "Granola" {
		t.Errorf("expected check name 'Granola', got %q", checks[0].Name)
	}
	if checks[0].Result.Status != "missing" {
		t.Errorf("expected status 'missing', got %q", checks[0].Result.Status)
	}
}

func TestGranolaDoctorChecks_OK(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", tmpHome)

	dir := filepath.Join(tmpHome, "Library", "Application Support", "Granola")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stored-accounts.json"), []byte(`[{"email":"test@example.com"}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Settings{cfg: &config.Config{}}
	checks := s.DoctorChecks(plugin.DoctorOpts{})
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Result.Status != "ok" {
		t.Errorf("expected status 'ok', got %q: %s", checks[0].Result.Status, checks[0].Result.Message)
	}
}
