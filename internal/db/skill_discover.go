package db

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SkillInfo holds frontmatter from a SKILL.md file.
type SkillInfo struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

// SkillCache holds discovered skills with a TTL.
type SkillCache struct {
	Skills    []SkillInfo `json:"skills"`
	ScannedAt time.Time   `json:"scanned_at"`
}

const skillCacheTTL = 1 * time.Hour

// DiscoverSkills scans <dir>/.claude/skills/*/SKILL.md, parses YAML
// frontmatter, and returns SkillInfo for each valid file.
func DiscoverSkills(dir string) []SkillInfo {
	pattern := filepath.Join(dir, ".claude", "skills", "*", "SKILL.md")
	return discoverSkillsGlob(pattern)
}

// DiscoverGlobalSkills scans ~/.claude/skills/*/SKILL.md using the same logic.
func DiscoverGlobalSkills() []SkillInfo {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	pattern := filepath.Join(home, ".claude", "skills", "*", "SKILL.md")
	return discoverSkillsGlob(pattern)
}

func discoverSkillsGlob(pattern string) []SkillInfo {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var skills []SkillInfo
	for _, path := range matches {
		info, ok := parseSkillFrontmatter(path)
		if ok {
			skills = append(skills, info)
		}
	}
	return skills
}

// parseSkillFrontmatter reads a SKILL.md file and extracts name/description
// from YAML frontmatter delimited by --- markers.
func parseSkillFrontmatter(path string) (SkillInfo, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillInfo{}, false
	}

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return SkillInfo{}, false
	}

	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return SkillInfo{}, false
	}

	var info SkillInfo
	if err := yaml.Unmarshal([]byte(content[4:4+end]), &info); err != nil {
		return SkillInfo{}, false
	}
	if info.Name == "" {
		return SkillInfo{}, false
	}
	return info, true
}

// skillCacheDir returns ~/.config/ccc/cache/skills/.
func skillCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ccc", "cache", "skills"), nil
}

// projectCacheKey returns a hex SHA-256 hash of the directory path.
func projectCacheKey(dir string) string {
	h := sha256.Sum256([]byte(dir))
	return fmt.Sprintf("%x", h[:16])
}

// LoadCachedSkills returns cached skills for the given cache file if the
// cache exists and is within TTL. Returns nil, false if cache is stale or missing.
func LoadCachedSkills(cacheFile string) ([]SkillInfo, bool) {
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}
	var cache SkillCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if time.Since(cache.ScannedAt) > skillCacheTTL {
		return nil, false
	}
	return cache.Skills, true
}

// WriteCachedSkills writes skills to the given cache file.
func WriteCachedSkills(cacheFile string, skills []SkillInfo) error {
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		return err
	}
	cache := SkillCache{
		Skills:    skills,
		ScannedAt: time.Now(),
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cacheFile, data, 0o600)
}

// GetProjectSkills returns skills for a project directory, using disk cache
// when available and fresh. If forceRefresh is true, the cache is bypassed.
func GetProjectSkills(dir string, forceRefresh bool) ([]SkillInfo, error) {
	cacheDir, err := skillCacheDir()
	if err != nil {
		return nil, err
	}
	cacheFile := filepath.Join(cacheDir, projectCacheKey(dir)+".json")

	if !forceRefresh {
		if skills, ok := LoadCachedSkills(cacheFile); ok {
			return skills, nil
		}
	}

	skills := DiscoverSkills(dir)
	if err := WriteCachedSkills(cacheFile, skills); err != nil {
		return skills, err // return skills even if cache write fails
	}
	return skills, nil
}

// GetGlobalSkills returns global skills, using disk cache when available and
// fresh. If forceRefresh is true, the cache is bypassed.
func GetGlobalSkills(forceRefresh bool) ([]SkillInfo, error) {
	cacheDir, err := skillCacheDir()
	if err != nil {
		return nil, err
	}
	cacheFile := filepath.Join(cacheDir, "global.json")

	if !forceRefresh {
		if skills, ok := LoadCachedSkills(cacheFile); ok {
			return skills, nil
		}
	}

	skills := DiscoverGlobalSkills()
	if err := WriteCachedSkills(cacheFile, skills); err != nil {
		return skills, err
	}
	return skills, nil
}
