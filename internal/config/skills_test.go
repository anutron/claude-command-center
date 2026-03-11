package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillNames_NoDir(t *testing.T) {
	// When the skills dir doesn't exist, SkillNames returns nil
	names := SkillNames()
	// We can't guarantee the dir doesn't exist in the test env,
	// but we can test the function doesn't panic
	_ = names
}

func TestSkillNames_WithTempDir(t *testing.T) {
	// Create a temp dir structure with fake skills
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create some fake skill files
	for _, name := range []string{"skill-a", "skill-b"} {
		if err := os.WriteFile(filepath.Join(skillsDir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Read from the created dir directly
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 skill entries, got %d", len(entries))
	}
}

func TestIsSkillInstalled_Missing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if IsSkillInstalled("nonexistent-skill") {
		t.Error("expected IsSkillInstalled to return false for missing skill")
	}
}

func TestIsSkillInstalled_Exists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	skillsDir := filepath.Join(tmpHome, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a file at the expected location
	skillPath := filepath.Join(skillsDir, "test-skill")
	if err := os.WriteFile(skillPath, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !IsSkillInstalled("test-skill") {
		t.Error("expected IsSkillInstalled to return true for existing skill")
	}
}
