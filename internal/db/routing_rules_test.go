package db

import (
	"os"
	"path/filepath"
	"testing"
)

func setupRoutingRulesDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CCC_CONFIG_DIR", dir)
}

func TestLoadRoutingRules_ValidYAML(t *testing.T) {
	setupRoutingRulesDir(t)

	content := `/Users/aaron/Development/thanx/merchant-ui/:
  use_for:
    - Thanx loyalty program UI bugs
    - merchant dashboard features
  not_for:
    - backend API work
/Users/aaron/Development/other/:
  use_for:
    - general frontend
`
	if err := os.WriteFile(routingRulesPath(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRoutingRules()
	if err != nil {
		t.Fatal(err)
	}

	if len(rules) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(rules))
	}

	r := rules["/Users/aaron/Development/thanx/merchant-ui/"]
	if len(r.UseFor) != 2 {
		t.Errorf("expected 2 use_for entries, got %d", len(r.UseFor))
	}
	if len(r.NotFor) != 1 {
		t.Errorf("expected 1 not_for entry, got %d", len(r.NotFor))
	}
	if r.UseFor[0] != "Thanx loyalty program UI bugs" {
		t.Errorf("unexpected use_for[0]: %q", r.UseFor[0])
	}
	if r.NotFor[0] != "backend API work" {
		t.Errorf("unexpected not_for[0]: %q", r.NotFor[0])
	}
}

func TestLoadRoutingRules_MissingFile(t *testing.T) {
	setupRoutingRulesDir(t)

	rules, err := LoadRoutingRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(rules))
	}
}

func TestLoadRoutingRules_MalformedYAML(t *testing.T) {
	setupRoutingRulesDir(t)

	// Write invalid YAML
	if err := os.WriteFile(routingRulesPath(), []byte(":::not valid yaml[[["), 0o600); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRoutingRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected empty map for malformed YAML, got %d entries", len(rules))
	}
}

func TestAddRoutingRule_CreatesFileAndAppendsRule(t *testing.T) {
	setupRoutingRulesDir(t)

	path := "/Users/aaron/Development/myproject/"

	// File doesn't exist yet — AddRoutingRule should create it.
	if err := AddRoutingRule(path, "use_for", "frontend bugs"); err != nil {
		t.Fatal(err)
	}

	// Verify file was created.
	if _, err := os.Stat(routingRulesPath()); err != nil {
		t.Fatalf("routing rules file not created: %v", err)
	}

	// Add another rule of a different type.
	if err := AddRoutingRule(path, "not_for", "backend work"); err != nil {
		t.Fatal(err)
	}

	// Load and verify both entries.
	rules, err := LoadRoutingRules()
	if err != nil {
		t.Fatal(err)
	}

	r := rules[path]
	if len(r.UseFor) != 1 || r.UseFor[0] != "frontend bugs" {
		t.Errorf("unexpected use_for: %v", r.UseFor)
	}
	if len(r.NotFor) != 1 || r.NotFor[0] != "backend work" {
		t.Errorf("unexpected not_for: %v", r.NotFor)
	}
}

func TestAddRoutingRule_InvalidType(t *testing.T) {
	setupRoutingRulesDir(t)

	err := AddRoutingRule("/some/path/", "bad_type", "text")
	if err == nil {
		t.Fatal("expected error for invalid rule type")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	setupRoutingRulesDir(t)

	original := map[string]RoutingRule{
		"/path/one/": {
			UseFor: []string{"task A", "task B"},
			NotFor: []string{"task C"},
		},
		"/path/two/": {
			UseFor: []string{"task D"},
		},
	}

	if err := SaveRoutingRules(original); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadRoutingRules()
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != len(original) {
		t.Fatalf("expected %d paths, got %d", len(original), len(loaded))
	}

	for path, orig := range original {
		got, ok := loaded[path]
		if !ok {
			t.Fatalf("missing path %q after round-trip", path)
		}
		if len(got.UseFor) != len(orig.UseFor) {
			t.Errorf("path %q: use_for length mismatch: %d vs %d", path, len(got.UseFor), len(orig.UseFor))
		}
		for i, v := range orig.UseFor {
			if got.UseFor[i] != v {
				t.Errorf("path %q: use_for[%d] = %q, want %q", path, i, got.UseFor[i], v)
			}
		}
		if len(got.NotFor) != len(orig.NotFor) {
			t.Errorf("path %q: not_for length mismatch: %d vs %d", path, len(got.NotFor), len(orig.NotFor))
		}
		for i, v := range orig.NotFor {
			if got.NotFor[i] != v {
				t.Errorf("path %q: not_for[%d] = %q, want %q", path, i, got.NotFor[i], v)
			}
		}
	}

	// Verify the file exists on disk.
	info, err := os.Stat(filepath.Join(os.Getenv("CCC_CONFIG_DIR"), "routing-rules.yaml"))
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("file is empty after save")
	}
}
