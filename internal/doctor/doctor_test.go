package doctor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
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

	checks := runDoctorChecks()

	if len(checks) != 8 {
		t.Fatalf("expected 8 doctor checks, got %d", len(checks))
	}

	if !checks[0].OK {
		t.Errorf("config check should pass with default config, got: %s", checks[0].Message)
	}
}
