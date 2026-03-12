package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// resolveDir resolves symlinks in a path for consistent comparisons
// (macOS /var -> /private/var).
func resolveDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("failed to resolve %s: %v", dir, err)
	}
	return resolved
}

// initGitRepo creates a bare-minimum git repo in dir with an initial commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	// Create initial commit so HEAD exists.
	f := filepath.Join(dir, "README")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %s: %v", name, args, out, err)
	}
	return strings.TrimSpace(string(out))
}

func TestLoadProjectConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config for missing file")
	}
}

func TestLoadProjectConfig_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ccc"), 0o755)
	yaml := `
worktree:
  base_branch: main
  branch_prefix: myapp
setup:
  symlinks:
    - .env
    - node_modules
  scripts:
    - npm install
`
	os.WriteFile(filepath.Join(dir, ".ccc", "config.yaml"), []byte(yaml), 0o644)

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Worktree.BaseBranch != "main" {
		t.Errorf("expected base_branch=main, got %s", cfg.Worktree.BaseBranch)
	}
	if cfg.Worktree.BranchPrefix != "myapp" {
		t.Errorf("expected branch_prefix=myapp, got %s", cfg.Worktree.BranchPrefix)
	}
	if len(cfg.Setup.Symlinks) != 2 {
		t.Errorf("expected 2 symlinks, got %d", len(cfg.Setup.Symlinks))
	}
	if len(cfg.Setup.Scripts) != 1 {
		t.Errorf("expected 1 script, got %d", len(cfg.Setup.Scripts))
	}
}

func TestLoadProjectConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ccc"), 0o755)
	os.WriteFile(filepath.Join(dir, ".ccc", "config.yaml"), []byte("{{invalid"), 0o644)

	cfg, err := LoadProjectConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if cfg != nil {
		t.Fatal("expected nil config on error")
	}
}

func TestLoadProjectConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ccc"), 0o755)
	// Empty config — defaults should apply.
	os.WriteFile(filepath.Join(dir, ".ccc", "config.yaml"), []byte("{}"), 0o644)

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Worktree.BranchPrefix != "ccc" {
		t.Errorf("expected default branch_prefix=ccc, got %s", cfg.Worktree.BranchPrefix)
	}
}

func TestPrepareWorktree(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	wtPath, branch, err := PrepareWorktree(dir)
	if err != nil {
		t.Fatalf("PrepareWorktree failed: %v", err)
	}

	// Worktree path should exist.
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree dir does not exist: %v", err)
	}

	// Should be under .claude/worktrees/
	if !strings.Contains(wtPath, filepath.Join(".claude", "worktrees")) {
		t.Errorf("worktree path %s not under .claude/worktrees/", wtPath)
	}

	// Branch should start with ccc/
	if !strings.HasPrefix(branch, "ccc/") {
		t.Errorf("branch %s does not start with ccc/", branch)
	}

	// Worktree should be a functional git checkout.
	out := run(t, wtPath, "git", "rev-parse", "--is-inside-work-tree")
	if out != "true" {
		t.Errorf("worktree is not a valid git work tree")
	}

	// Branch should exist.
	run(t, dir, "git", "rev-parse", "--verify", branch)
}

func TestPrepareWorktreeWithSymlinks(t *testing.T) {
	dir := resolveDir(t, t.TempDir())
	initGitRepo(t, dir)

	// Create a source file to symlink.
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("SECRET=foo"), 0o644)

	// Create nested source.
	os.MkdirAll(filepath.Join(dir, "config"), 0o755)
	os.WriteFile(filepath.Join(dir, "config", "local.yaml"), []byte("key: val"), 0o644)

	// Write project config with symlinks (including a missing source).
	os.MkdirAll(filepath.Join(dir, ".ccc"), 0o755)
	cfg := `
setup:
  symlinks:
    - .env
    - config/local.yaml
    - nonexistent_file
`
	os.WriteFile(filepath.Join(dir, ".ccc", "config.yaml"), []byte(cfg), 0o644)

	wtPath, _, err := PrepareWorktree(dir)
	if err != nil {
		t.Fatalf("PrepareWorktree failed: %v", err)
	}

	// .env symlink should exist and point to repo root.
	link, err := os.Readlink(filepath.Join(wtPath, ".env"))
	if err != nil {
		t.Fatalf("expected .env symlink: %v", err)
	}
	if link != envFile {
		t.Errorf("symlink target = %s, want %s", link, envFile)
	}

	// Nested symlink should work too.
	link, err = os.Readlink(filepath.Join(wtPath, "config", "local.yaml"))
	if err != nil {
		t.Fatalf("expected config/local.yaml symlink: %v", err)
	}
	if link != filepath.Join(dir, "config", "local.yaml") {
		t.Errorf("nested symlink target wrong: %s", link)
	}

	// Missing source should NOT cause error (just warned).
	if _, err := os.Lstat(filepath.Join(wtPath, "nonexistent_file")); !os.IsNotExist(err) {
		t.Error("expected nonexistent_file symlink to not be created")
	}
}

func TestPrepareWorktreeWithScripts(t *testing.T) {
	dir := resolveDir(t, t.TempDir())
	initGitRepo(t, dir)

	os.MkdirAll(filepath.Join(dir, ".ccc"), 0o755)
	cfg := `
setup:
  scripts:
    - echo "$CCC_REPO_PATH" > marker.txt
    - echo "$CCC_WORKTREE_PATH" >> marker.txt
    - echo "$CCC_BRANCH" >> marker.txt
`
	os.WriteFile(filepath.Join(dir, ".ccc", "config.yaml"), []byte(cfg), 0o644)

	wtPath, branch, err := PrepareWorktree(dir)
	if err != nil {
		t.Fatalf("PrepareWorktree failed: %v", err)
	}

	// Read the marker file created by the script.
	data, err := os.ReadFile(filepath.Join(wtPath, "marker.txt"))
	if err != nil {
		t.Fatalf("marker.txt not created: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines in marker.txt, got %d: %q", len(lines), string(data))
	}
	if lines[0] != dir {
		t.Errorf("CCC_REPO_PATH = %s, want %s", lines[0], dir)
	}
	if lines[1] != wtPath {
		t.Errorf("CCC_WORKTREE_PATH = %s, want %s", lines[1], wtPath)
	}
	if lines[2] != branch {
		t.Errorf("CCC_BRANCH = %s, want %s", lines[2], branch)
	}
}

func TestListWorktrees(t *testing.T) {
	dir := resolveDir(t, t.TempDir())
	initGitRepo(t, dir)

	// Create two worktrees.
	wt1, _, err := PrepareWorktree(dir)
	if err != nil {
		t.Fatalf("PrepareWorktree 1 failed: %v", err)
	}
	wt2, _, err := PrepareWorktree(dir)
	if err != nil {
		t.Fatalf("PrepareWorktree 2 failed: %v", err)
	}

	// Create a non-CCC worktree to ensure it's filtered out.
	nonCCCPath := filepath.Join(dir, ".claude", "worktrees", "other-wt")
	run(t, dir, "git", "worktree", "add", "-b", "feature/other", nonCCCPath, "HEAD")

	worktrees, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	// Should only include CCC worktrees.
	if len(worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(worktrees))
	}

	// Should be sorted newest first.
	paths := []string{worktrees[0].Path, worktrees[1].Path}
	if paths[0] != wt2 || paths[1] != wt1 {
		t.Errorf("expected newest first: got %v, want [%s, %s]", paths, wt2, wt1)
	}
}

func TestRemoveWorktree(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	wtPath, branch, err := PrepareWorktree(dir)
	if err != nil {
		t.Fatalf("PrepareWorktree failed: %v", err)
	}

	if err := RemoveWorktree(dir, wtPath); err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}

	// Dir should be gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory still exists")
	}

	// Branch should be gone.
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Error("branch still exists after removal")
	}
}

func TestPruneWorktrees(t *testing.T) {
	dir := resolveDir(t, t.TempDir())
	initGitRepo(t, dir)

	wt1, _, _ := PrepareWorktree(dir)
	wt2, _, _ := PrepareWorktree(dir)

	removed, err := PruneWorktrees(dir)
	if err != nil {
		t.Fatalf("PruneWorktrees failed: %v", err)
	}

	if len(removed) != 2 {
		t.Fatalf("expected 2 removed, got %d", len(removed))
	}

	// Both should be gone.
	for _, p := range []string{wt1, wt2} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("worktree %s still exists", p)
		}
	}

	// Listing should return empty.
	remaining, _ := ListWorktrees(dir)
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining worktrees, got %d", len(remaining))
	}
}

func TestPrepareWorktreeNotGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, _, err := PrepareWorktree(dir)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected error message: %v", err)
	}
}
