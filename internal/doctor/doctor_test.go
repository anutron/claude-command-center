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
	checks   []plugin.DoctorCheck
	lastOpts plugin.DoctorOpts // captures the opts passed to DoctorChecks
}

func (m *mockProvider) DoctorChecks(opts plugin.DoctorOpts) []plugin.DoctorCheck {
	m.lastOpts = opts
	return m.checks
}

func TestDoctorChecks_Inconclusive(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmp)
	t.Setenv("CCC_STATE_DIR", filepath.Join(tmp, "data"))

	mock := &mockProvider{
		checks: []plugin.DoctorCheck{
			{
				Name:         "Network check",
				Result:       plugin.ValidationResult{Status: "incomplete", Message: "Cannot reach endpoint"},
				Inconclusive: true,
			},
		},
	}

	checks := runDoctorChecks([]plugin.DoctorProvider{mock}, plugin.DoctorOpts{})

	// Find the provider check
	var found *DoctorCheck
	for i, c := range checks {
		if c.Name == "Network check" {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("inconclusive check not found in results")
	}
	if found.OK {
		t.Error("expected inconclusive check to not be OK")
	}
	if !found.Inconclusive {
		t.Error("expected Inconclusive=true")
	}
}

func TestDoctorChecks_LiveFlagPropagated(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmp)
	t.Setenv("CCC_STATE_DIR", filepath.Join(tmp, "data"))

	mock := &mockProvider{
		checks: []plugin.DoctorCheck{
			{Name: "Test", Result: plugin.ValidationResult{Status: "ok", Message: "ok"}},
		},
	}

	// Run without live
	runDoctorChecks([]plugin.DoctorProvider{mock}, plugin.DoctorOpts{Live: false})
	if mock.lastOpts.Live {
		t.Error("expected Live=false to be propagated")
	}

	// Run with live
	runDoctorChecks([]plugin.DoctorProvider{mock}, plugin.DoctorOpts{Live: true})
	if !mock.lastOpts.Live {
		t.Error("expected Live=true to be propagated")
	}
}

func TestDoctorChecks_MultipleProviders(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmp)
	t.Setenv("CCC_STATE_DIR", filepath.Join(tmp, "data"))

	p1 := &mockProvider{
		checks: []plugin.DoctorCheck{
			{Name: "Calendar creds", Result: plugin.ValidationResult{Status: "ok", Message: "found"}},
			{Name: "Calendar token (live)", Result: plugin.ValidationResult{Status: "ok", Message: "valid"}},
		},
	}
	p2 := &mockProvider{
		checks: []plugin.DoctorCheck{
			{Name: "Gmail creds", Result: plugin.ValidationResult{Status: "missing", Message: "not found", Hint: "press a"}},
		},
	}

	checks := runDoctorChecks([]plugin.DoctorProvider{p1, p2}, plugin.DoctorOpts{Live: true})

	// Find provider checks
	var calCred, calLive, gmailCred *DoctorCheck
	for i, c := range checks {
		switch c.Name {
		case "Calendar creds":
			calCred = &checks[i]
		case "Calendar token (live)":
			calLive = &checks[i]
		case "Gmail creds":
			gmailCred = &checks[i]
		}
	}

	if calCred == nil || calLive == nil {
		t.Fatal("expected both calendar checks")
	}
	if !calCred.OK || !calLive.OK {
		t.Error("expected both calendar checks to pass")
	}

	if gmailCred == nil {
		t.Fatal("expected gmail check")
	}
	if gmailCred.OK {
		t.Error("expected gmail check to fail")
	}
	if gmailCred.Message == "" {
		t.Error("expected gmail check to have a message with hint")
	}
}

func TestDoctorChecks_FailedProviderShowsHint(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmp)
	t.Setenv("CCC_STATE_DIR", filepath.Join(tmp, "data"))

	mock := &mockProvider{
		checks: []plugin.DoctorCheck{
			{
				Name: "Test creds",
				Result: plugin.ValidationResult{
					Status:  "no_client",
					Message: "OAuth client missing",
					Hint:    "Press 'a' to configure",
				},
			},
		},
	}

	checks := runDoctorChecks([]plugin.DoctorProvider{mock}, plugin.DoctorOpts{})
	var found *DoctorCheck
	for i, c := range checks {
		if c.Name == "Test creds" {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("check not found")
	}
	if found.OK {
		t.Error("expected check to fail")
	}
	// Message should include the hint
	if found.Message != "OAuth client missing — Press 'a' to configure" {
		t.Errorf("expected message with hint, got %q", found.Message)
	}
}
