package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSkillMD(t *testing.T, dir, skillName, content string) {
	t.Helper()
	skillDir := filepath.Join(dir, ".claude", "skills", skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverSkills_FindsSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, dir, "wind-down", `---
name: wind-down
description: Save session context to disk
---

# Wind Down
Body content here.
`)
	writeSkillMD(t, dir, "review", `---
name: review
description: Review pull requests
---
`)

	skills := DiscoverSkills(dir)
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	names := map[string]string{}
	for _, s := range skills {
		names[s.Name] = s.Description
	}
	if names["wind-down"] != "Save session context to disk" {
		t.Errorf("unexpected wind-down description: %q", names["wind-down"])
	}
	if names["review"] != "Review pull requests" {
		t.Errorf("unexpected review description: %q", names["review"])
	}
}

func TestDiscoverSkills_EmptyForNoSkillsDir(t *testing.T) {
	dir := t.TempDir()
	skills := DiscoverSkills(dir)
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}
}

func TestDiscoverSkills_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()

	// Valid skill
	writeSkillMD(t, dir, "good", `---
name: good
description: A good skill
---
`)
	// Missing frontmatter entirely
	writeSkillMD(t, dir, "no-frontmatter", `# Just a heading
No frontmatter here.
`)
	// Frontmatter without name
	writeSkillMD(t, dir, "no-name", `---
description: Has description but no name
---
`)
	// Malformed YAML
	writeSkillMD(t, dir, "bad-yaml", `---
name: [unclosed
description: bad
---
`)

	skills := DiscoverSkills(dir)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d: %+v", len(skills), skills)
	}
	if skills[0].Name != "good" {
		t.Errorf("expected 'good', got %q", skills[0].Name)
	}
}

func TestDiskCache_ReturnsCachedWithinTTL(t *testing.T) {
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "test.json")

	original := []SkillInfo{
		{Name: "cached-skill", Description: "From cache"},
	}
	cache := SkillCache{
		Skills:    original,
		ScannedAt: time.Now(), // fresh
	}
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	skills, ok := LoadCachedSkills(cacheFile)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(skills) != 1 || skills[0].Name != "cached-skill" {
		t.Errorf("unexpected cached skills: %+v", skills)
	}
}

func TestDiskCache_ExpiredTTL(t *testing.T) {
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "test.json")

	cache := SkillCache{
		Skills:    []SkillInfo{{Name: "old", Description: "stale"}},
		ScannedAt: time.Now().Add(-2 * time.Hour), // expired
	}
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok := LoadCachedSkills(cacheFile)
	if ok {
		t.Fatal("expected cache miss for expired TTL")
	}
}

func TestDiskCache_MissingFile(t *testing.T) {
	_, ok := LoadCachedSkills("/nonexistent/path/cache.json")
	if ok {
		t.Fatal("expected cache miss for missing file")
	}
}

func TestWriteAndLoadCachedSkills(t *testing.T) {
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "roundtrip.json")

	skills := []SkillInfo{
		{Name: "alpha", Description: "First"},
		{Name: "beta", Description: "Second"},
	}
	if err := WriteCachedSkills(cacheFile, skills); err != nil {
		t.Fatal(err)
	}

	loaded, ok := LoadCachedSkills(cacheFile)
	if !ok {
		t.Fatal("expected cache hit after write")
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(loaded))
	}
	if loaded[0].Name != "alpha" || loaded[1].Name != "beta" {
		t.Errorf("unexpected skills: %+v", loaded)
	}
}
