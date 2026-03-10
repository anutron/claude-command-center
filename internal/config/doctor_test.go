package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestCheckDataFreshness_WithFreshData(t *testing.T) {
	// Set up isolated config/state dirs
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, "data")
	t.Setenv("CCC_CONFIG_DIR", tmp)
	t.Setenv("CCC_STATE_DIR", stateDir)

	// Open DB (creates schema including cc_meta table)
	database, err := db.OpenDB(DBPath())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	// Insert a fresh generated_at value into cc_meta (the correct table)
	now := time.Now().Format(time.RFC3339)
	_, err = database.Exec(`INSERT OR REPLACE INTO cc_meta (key, value, updated_at) VALUES ('generated_at', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert generated_at: %v", err)
	}

	// checkDataFreshness should report OK since data is fresh
	// BUG: It queries non-existent "command_center" table instead of "cc_meta"
	check := checkDataFreshness()
	if !check.OK {
		t.Errorf("expected OK=true for fresh data, got OK=false with message: %s", check.Message)
	}
}
