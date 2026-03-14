package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// repoSkillsDir returns the path to the .claude/skills directory in the repo.
func repoSkillsDir() string {
	// Next to the current executable (resolve symlinks to find the real location)
	if exe, err := os.Executable(); err == nil {
		resolved, err := filepath.EvalSymlinks(exe)
		if err == nil {
			exe = resolved
		}
		candidate := filepath.Join(filepath.Dir(exe), ".claude", "skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	// Current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, ".claude", "skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

// userSkillsDir returns the path to ~/.claude/skills.
func userSkillsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "skills")
}

// SkillNames returns the list of skill names available in the repo.
func SkillNames() []string {
	dir := repoSkillsDir()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.Name() == "." || e.Name() == ".." {
			continue
		}
		names = append(names, e.Name())
	}
	return names
}

// IsSkillInstalled checks if a skill symlink exists in ~/.claude/skills.
func IsSkillInstalled(name string) bool {
	target := filepath.Join(userSkillsDir(), name)
	_, err := os.Lstat(target)
	return err == nil
}

// InstallSkills symlinks all repo skills to ~/.claude/skills.
func InstallSkills() error {
	repoDir := repoSkillsDir()
	if repoDir == "" {
		return fmt.Errorf("repo .claude/skills directory not found")
	}
	destDir := userSkillsDir()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating skills dir: %w", err)
	}

	names := SkillNames()
	for _, name := range names {
		src := filepath.Join(repoDir, name)
		dst := filepath.Join(destDir, name)
		// Remove existing symlink if present
		os.Remove(dst)
		if err := os.Symlink(src, dst); err != nil {
			return fmt.Errorf("symlinking skill %s: %w", name, err)
		}
	}
	return nil
}

// UninstallSkills removes all repo skill symlinks from ~/.claude/skills.
func UninstallSkills() error {
	repoDir := repoSkillsDir()
	if repoDir == "" {
		return fmt.Errorf("repo .claude/skills directory not found")
	}
	destDir := userSkillsDir()
	names := SkillNames()
	for _, name := range names {
		dst := filepath.Join(destDir, name)
		// Only remove if it's a symlink pointing to our repo
		if target, err := os.Readlink(dst); err == nil {
			expected := filepath.Join(repoDir, name)
			if target == expected {
				os.Remove(dst)
			}
		}
	}
	return nil
}
