package doctor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
)

func TestCheckDataFreshness_WithFreshData(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, "data")
	t.Setenv("CCC_CONFIG_DIR", tmp)
	t.Setenv("CCC_STATE_DIR", stateDir)

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	now := time.Now().Format(time.RFC3339)
	_, err = database.Exec(`INSERT OR REPLACE INTO cc_meta (key, value, updated_at) VALUES ('generated_at', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert generated_at: %v", err)
	}

	check := checkDataFreshness()
	if !check.OK {
		t.Errorf("expected OK=true for fresh data, got OK=false with message: %s", check.Message)
	}
}

func TestDoctorChecks(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmp)
	t.Setenv("CCC_STATE_DIR", filepath.Join(tmp, "data"))

	// No providers — only built-in checks: config, database, refresh binary, claude CLI, data freshness
	checks := runDoctorChecks(nil, plugin.DoctorOpts{})

	if len(checks) < 5 {
		t.Fatalf("expected at least 5 built-in doctor checks, got %d", len(checks))
	}

	if !checks[0].OK {
		t.Errorf("config check should pass with default config, got: %s", checks[0].Message)
	}

	// With a mock provider
	mock := &mockProvider{
		checks: []plugin.DoctorCheck{
			{Name: "Mock check", Result: plugin.ValidationResult{Status: "ok", Message: "All good"}},
		},
	}
	checksWithProvider := runDoctorChecks([]plugin.DoctorProvider{mock}, plugin.DoctorOpts{})

	// Should have built-in checks + 1 provider check
	if len(checksWithProvider) != len(checks)+1 {
		t.Fatalf("expected %d checks with mock provider, got %d", len(checks)+1, len(checksWithProvider))
	}

	// Provider check should be after config and database but before refresh binary
	providerCheck := checksWithProvider[2] // config=0, database=1, provider=2
	if providerCheck.Name != "Mock check" {
		t.Errorf("expected provider check at index 2, got %q", providerCheck.Name)
	}
	if !providerCheck.OK {
		t.Errorf("expected provider check to be OK")
	}
}

// mockProvider implements plugin.DoctorProvider for testing.
type mockProvider struct {
	checks []plugin.DoctorCheck
}

func (m *mockProvider) DoctorChecks(opts plugin.DoctorOpts) []plugin.DoctorCheck {
	return m.checks
}
