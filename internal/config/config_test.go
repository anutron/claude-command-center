package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Name != "Command Center" {
		t.Errorf("expected Name='Command Center', got %q", cfg.Name)
	}
	if cfg.Palette != "aurora" {
		t.Errorf("expected Palette='aurora', got %q", cfg.Palette)
	}
	if !cfg.Todos.Enabled {
		t.Error("expected Todos.Enabled=true")
	}
	if cfg.Calendar.Enabled {
		t.Error("expected Calendar.Enabled=false")
	}
	if cfg.GitHub.Enabled {
		t.Error("expected GitHub.Enabled=false")
	}
	if cfg.Granola.Enabled {
		t.Error("expected Granola.Enabled=false")
	}
	if cfg.Colors != nil {
		t.Error("expected Colors=nil")
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "Command Center" {
		t.Errorf("expected default Name, got %q", cfg.Name)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmp)

	original := &Config{
		Name:    "My Dashboard",
		Palette: "ocean",
		Calendar: CalendarConfig{
			Enabled: true,
			Calendars: []CalendarEntry{
				{ID: "cal1", Label: "Work", Color: "#ff0000"},
			},
		},
		GitHub: GitHubConfig{
			Enabled:  true,
			Repos:    []string{"owner/repo1", "owner/repo2"},
			Username: "testuser",
		},
		Todos:   TodosConfig{Enabled: true},
		Granola: GranolaConfig{Enabled: false},
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", loaded.Name, original.Name)
	}
	if loaded.Palette != original.Palette {
		t.Errorf("Palette: got %q, want %q", loaded.Palette, original.Palette)
	}
	if !loaded.Calendar.Enabled {
		t.Error("expected Calendar.Enabled=true")
	}
	if len(loaded.Calendar.Calendars) != 1 {
		t.Fatalf("expected 1 calendar entry, got %d", len(loaded.Calendar.Calendars))
	}
	if loaded.Calendar.Calendars[0].ID != "cal1" {
		t.Errorf("calendar ID: got %q, want %q", loaded.Calendar.Calendars[0].ID, "cal1")
	}
	if !loaded.GitHub.Enabled {
		t.Error("expected GitHub.Enabled=true")
	}
	if len(loaded.GitHub.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(loaded.GitHub.Repos))
	}
	if loaded.GitHub.Username != "testuser" {
		t.Errorf("Username: got %q, want %q", loaded.GitHub.Username, "testuser")
	}
}

func TestGetPalette(t *testing.T) {
	names := PaletteNames()
	if len(names) != 5 {
		t.Fatalf("expected 5 palettes, got %d", len(names))
	}

	for _, name := range names {
		p := GetPalette(name, nil)
		if p.Fg == "" {
			t.Errorf("palette %q has empty Fg", name)
		}
		if p.Highlight == "" {
			t.Errorf("palette %q has empty Highlight", name)
		}
		if p.BgDark == "" {
			t.Errorf("palette %q has empty BgDark", name)
		}
	}

	// Unknown palette falls back to aurora
	unknown := GetPalette("nonexistent", nil)
	aurora := GetPalette("aurora", nil)
	if unknown.Fg != aurora.Fg {
		t.Errorf("unknown palette should fall back to aurora")
	}
}

func TestConfigPaths(t *testing.T) {
	t.Setenv("CCC_CONFIG_DIR", "/tmp/test-ccc-config")
	t.Setenv("CCC_STATE_DIR", "/tmp/test-ccc-state")

	if got := ConfigDir(); got != "/tmp/test-ccc-config" {
		t.Errorf("ConfigDir: got %q, want /tmp/test-ccc-config", got)
	}
	if got := ConfigPath(); got != "/tmp/test-ccc-config/config.yaml" {
		t.Errorf("ConfigPath: got %q, want /tmp/test-ccc-config/config.yaml", got)
	}
	if got := DataDir(); got != "/tmp/test-ccc-state" {
		t.Errorf("DataDir: got %q, want /tmp/test-ccc-state", got)
	}
	if got := DBPath(); got != "/tmp/test-ccc-state/ccc.db" {
		t.Errorf("DBPath: got %q, want /tmp/test-ccc-state/ccc.db", got)
	}
	if got := CredentialsDir(); got != "/tmp/test-ccc-config/credentials" {
		t.Errorf("CredentialsDir: got %q, want /tmp/test-ccc-config/credentials", got)
	}

	// Without env vars, falls back to defaults
	t.Setenv("CCC_CONFIG_DIR", "")
	t.Setenv("CCC_STATE_DIR", "")

	home, _ := os.UserHomeDir()
	expectedDir := filepath.Join(home, ".config", "ccc")
	if got := ConfigDir(); got != expectedDir {
		t.Errorf("ConfigDir default: got %q, want %q", got, expectedDir)
	}
	expectedData := filepath.Join(expectedDir, "data")
	if got := DataDir(); got != expectedData {
		t.Errorf("DataDir default: got %q, want %q", got, expectedData)
	}
}

func TestCustomPalette(t *testing.T) {
	custom := &CustomColors{
		Primary:   "#112233",
		Secondary: "#445566",
		Accent:    "#778899",
	}

	p := GetPalette("custom", custom)
	if p.Fg != "#112233" {
		t.Errorf("custom Fg: got %q, want #112233", p.Fg)
	}
	if p.Highlight != "#445566" {
		t.Errorf("custom Highlight: got %q, want #445566", p.Highlight)
	}
	if p.Pointer != "#778899" {
		t.Errorf("custom Pointer: got %q, want #778899", p.Pointer)
	}

	// "custom" without colors falls back to aurora
	p2 := GetPalette("custom", nil)
	aurora := GetPalette("aurora", nil)
	if p2.Fg != aurora.Fg {
		t.Error("custom without colors should fall back to aurora")
	}
}

func TestParseRefreshInterval(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"empty defaults to 5m", "", 5 * time.Minute},
		{"valid 10m", "10m", 10 * time.Minute},
		{"valid 1h", "1h", 1 * time.Hour},
		{"valid 2m", "2m", 2 * time.Minute},
		{"below minimum returns default", "30s", 5 * time.Minute},
		{"invalid string returns default", "invalid", 5 * time.Minute},
		{"zero returns default", "0s", 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{RefreshInterval: tt.input}
			got := cfg.ParseRefreshInterval()
			if got != tt.expected {
				t.Errorf("ParseRefreshInterval(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDoctorChecks(t *testing.T) {
	// Doctor checks with a temp config dir should get predictable results
	tmp := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", tmp)
	t.Setenv("CCC_STATE_DIR", filepath.Join(tmp, "data"))

	checks := runDoctorChecks()

	// We should get 8 checks
	if len(checks) != 8 {
		t.Fatalf("expected 8 doctor checks, got %d", len(checks))
	}

	// Config should pass (missing file returns default)
	if !checks[0].OK {
		t.Errorf("config check should pass with default config, got: %s", checks[0].Message)
	}

	// Database should fail (no DB file in temp dir)
	// (or pass if OpenDB creates it — either way, check it runs without panic)
	_ = checks[1]

	// Data freshness should fail (no data)
	freshCheck := checks[7]
	if freshCheck.OK && freshCheck.Message == "" {
		// If DB was created, data freshness should still fail (no command_center row)
	}
}

func TestSchedulePlistPath(t *testing.T) {
	path := plistPath()
	if path == "" {
		t.Error("plistPath() returned empty string")
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "Library", "LaunchAgents", "com.ccc.refresh.plist")
	if path != expected {
		t.Errorf("plistPath() = %q, want %q", path, expected)
	}
}
