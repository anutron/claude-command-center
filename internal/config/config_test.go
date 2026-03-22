package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Name != "Claude Command" {
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
	if cfg.Name != "Claude Command" {
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

func TestSaveRefusesDefaultsOverCustomConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", dir)

	// Write a user config with custom settings
	userConfig := `name: My Custom Dashboard
palette: dracula
calendar:
    enabled: true
    calendars:
        - id: work@example.com
          label: Work
github:
    enabled: true
    repos:
        - owner/repo1
    username: myuser
todos:
    enabled: true
granola:
    enabled: false
slack:
    enabled: false
gmail:
    enabled: false
external_plugins:
    - name: Pomodoro
      command: pomodoro
      enabled: true
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(userConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Attempt to save a default config over it (simulating the bug scenario)
	defaults := DefaultConfig()
	defaults.loadedFromFile = true // pretend it was loaded
	err := Save(defaults)
	if err == nil {
		t.Fatal("Save should have refused to overwrite custom config with defaults")
	}
	t.Logf("Save correctly refused: %v", err)

	// Verify the original file is intact
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "My Custom Dashboard" {
		t.Errorf("Name was changed: got %q, want %q", loaded.Name, "My Custom Dashboard")
	}
	if len(loaded.ExternalPlugins) != 1 {
		t.Errorf("ExternalPlugins lost: got %d, want 1", len(loaded.ExternalPlugins))
	}
}

func TestSaveAllowsLegitimateChanges(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", dir)

	// Write a user config
	userConfig := `name: My Dashboard
palette: aurora
calendar:
    enabled: false
    calendars: []
github:
    enabled: false
    repos: []
    username: ""
todos:
    enabled: true
threads:
    enabled: true
granola:
    enabled: false
slack:
    enabled: false
gmail:
    enabled: false
external_plugins:
    - name: Pomodoro
      command: pomodoro
      enabled: true
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(userConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Load, make a legitimate change, and save
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Palette = "dracula"
	if err := Save(cfg); err != nil {
		t.Fatalf("Save should allow legitimate changes: %v", err)
	}

	// Verify the change was saved
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Palette != "dracula" {
		t.Errorf("Palette: got %q, want %q", loaded.Palette, "dracula")
	}
	if loaded.Name != "My Dashboard" {
		t.Errorf("Name should be preserved: got %q, want %q", loaded.Name, "My Dashboard")
	}
	if len(loaded.ExternalPlugins) != 1 {
		t.Errorf("ExternalPlugins should be preserved: got %d, want 1", len(loaded.ExternalPlugins))
	}
}

func TestSaveRoundTripPreservesAllFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", dir)

	// Write a config with all fields populated
	fullConfig := `name: Test Center
palette: aurora
calendar:
    enabled: true
    calendars: []
github:
    enabled: false
    repos: []
    username: ""
todos:
    enabled: true
granola:
    enabled: false
slack:
    enabled: false
gmail:
    enabled: false
external_plugins:
    - name: Pomodoro
      command: pomodoro
      enabled: true
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(fullConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Load and immediately save (no changes)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load again and verify everything survived
	cfg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Name != "Test Center" {
		t.Errorf("Name: got %q, want %q", cfg2.Name, "Test Center")
	}
	if len(cfg2.ExternalPlugins) != 1 {
		t.Errorf("ExternalPlugins: got %d, want 1", len(cfg2.ExternalPlugins))
	}
	if cfg2.ExternalPlugins[0].Name != "Pomodoro" {
		t.Errorf("ExternalPlugins[0].Name: got %q, want %q", cfg2.ExternalPlugins[0].Name, "Pomodoro")
	}
}

func TestAutomationConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", dir)

	original := &Config{
		Name:    "Test Dashboard",
		Palette: "aurora",
		Todos:   TodosConfig{Enabled: true},
		Automations: []AutomationConfig{
			{
				Name:         "daily-review",
				Command:      "claude -p 'review todos'",
				Enabled:      true,
				Schedule:     "0 9 * * *",
				ConfigScopes: []string{"todos", "calendar"},
				Settings:     map[string]interface{}{"verbose": true},
			},
			{
				Name:         "weekly-report",
				Command:      "claude -p 'generate report'",
				Enabled:      false,
				Schedule:     "0 17 * * 5",
				ConfigScopes: []string{"github"},
			},
		},
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded.Automations) != 2 {
		t.Fatalf("expected 2 automations, got %d", len(loaded.Automations))
	}

	a := loaded.Automations[0]
	if a.Name != "daily-review" {
		t.Errorf("Name: got %q, want %q", a.Name, "daily-review")
	}
	if a.Command != "claude -p 'review todos'" {
		t.Errorf("Command: got %q", a.Command)
	}
	if !a.Enabled {
		t.Error("expected Enabled=true")
	}
	if a.Schedule != "0 9 * * *" {
		t.Errorf("Schedule: got %q", a.Schedule)
	}
	if len(a.ConfigScopes) != 2 || a.ConfigScopes[0] != "todos" {
		t.Errorf("ConfigScopes: got %v", a.ConfigScopes)
	}
	if a.Settings["verbose"] != true {
		t.Errorf("Settings[verbose]: got %v", a.Settings["verbose"])
	}

	b := loaded.Automations[1]
	if b.Enabled {
		t.Error("expected second automation Enabled=false")
	}
	if b.Settings != nil && len(b.Settings) > 0 {
		t.Errorf("expected nil/empty Settings for second automation, got %v", b.Settings)
	}
}

func TestRegressionDetectsDroppedAutomations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", dir)

	// Write a config with automations
	configWithAutomations := `name: My Dashboard
palette: aurora
todos:
    enabled: true
automations:
    - name: daily-review
      command: "claude -p 'review'"
      enabled: true
      schedule: "0 9 * * *"
      config_scopes:
          - todos
`
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(configWithAutomations), 0o644); err != nil {
		t.Fatal(err)
	}

	// Try to save a config without automations — should be rejected
	noAuto := &Config{
		Name:           "My Dashboard",
		Palette:        "aurora",
		Todos:          TodosConfig{Enabled: true},
		loadedFromFile: true,
	}
	err := Save(noAuto)
	if err == nil {
		t.Fatal("Save should have refused to drop automations")
	}
	if !contains(err.Error(), "automation") {
		t.Errorf("error should mention automations: %v", err)
	}

	// Verify original file is intact
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Automations) != 1 {
		t.Errorf("automations should be preserved: got %d, want 1", len(loaded.Automations))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestAgentConfig_SandboxDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Agent.TodoWriteLearnedPathsEnabled() {
		t.Error("TodoWriteLearnedPaths should default to true")
	}
	if len(cfg.Agent.AutonomousAllowedDomains) == 0 {
		t.Error("AutonomousAllowedDomains should have defaults")
	}
	// Verify specific default domains
	domains := cfg.Agent.AutonomousAllowedDomains
	found := map[string]bool{"github.com": false, "api.github.com": false}
	for _, d := range domains {
		if _, ok := found[d]; ok {
			found[d] = true
		}
	}
	for domain, ok := range found {
		if !ok {
			t.Errorf("expected default domain %q in AutonomousAllowedDomains", domain)
		}
	}

	// Verify explicit false overrides the default
	f := false
	cfg.Agent.TodoWriteLearnedPaths = &f
	if cfg.Agent.TodoWriteLearnedPathsEnabled() {
		t.Error("TodoWriteLearnedPaths should be false when explicitly set")
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


