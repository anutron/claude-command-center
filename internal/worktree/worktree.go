package worktree

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ProjectConfig holds per-repo worktree and setup configuration,
// loaded from .ccc/config.yaml in the repository root.
type ProjectConfig struct {
	Worktree WorktreeConfig `yaml:"worktree"`
	Setup    SetupConfig    `yaml:"setup"`
}

// WorktreeConfig controls branch naming and base branch for new worktrees.
type WorktreeConfig struct {
	BaseBranch   string `yaml:"base_branch"`
	BranchPrefix string `yaml:"branch_prefix"`
}

// SetupConfig defines symlinks and scripts to run when creating a worktree.
type SetupConfig struct {
	Symlinks []string `yaml:"symlinks"`
	Scripts  []string `yaml:"scripts"`
}

// WorktreeInfo describes an existing CCC-managed worktree.
type WorktreeInfo struct {
	Path      string    // absolute path to worktree
	Branch    string    // branch name
	RepoRoot  string    // source repo
	CreatedAt time.Time // from filesystem
}

// LoadProjectConfig reads .ccc/config.yaml from repoRoot.
// Returns nil, nil if the file doesn't exist.
// Returns nil, error if YAML is invalid.
func LoadProjectConfig(repoRoot string) (*ProjectConfig, error) {
	path := filepath.Join(repoRoot, ".ccc", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid .ccc/config.yaml: %w", err)
	}

	// Apply defaults
	if cfg.Worktree.BranchPrefix == "" {
		cfg.Worktree.BranchPrefix = "ccc"
	}

	return &cfg, nil
}

// PrepareWorktree creates a new git worktree for an isolated session.
// dir is any directory inside a git repo. Returns the worktree path and branch name.
func PrepareWorktree(dir string) (worktreePath string, branch string, err error) {
	repoRoot, err := gitRepoRoot(dir)
	if err != nil {
		return "", "", fmt.Errorf("not a git repository: %w", err)
	}

	cfg, err := LoadProjectConfig(repoRoot)
	if err != nil {
		return "", "", err
	}

	prefix := "ccc"
	baseBranch := "HEAD"
	if cfg != nil {
		if cfg.Worktree.BranchPrefix != "" {
			prefix = cfg.Worktree.BranchPrefix
		}
		if cfg.Worktree.BaseBranch != "" {
			baseBranch = cfg.Worktree.BaseBranch
		}
	}

	datePart := time.Now().Format("2006-01-02")

	// Try to create worktree, retry once on branch name collision.
	for attempt := 0; attempt < 2; attempt++ {
		hex := randomHex(4)
		branch = fmt.Sprintf("%s/%s-%s", prefix, datePart, hex)
		dirName := fmt.Sprintf("%s-%s-%s", prefix, datePart, hex)
		worktreePath = filepath.Join(repoRoot, ".claude", "worktrees", dirName)

		cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreePath, baseBranch)
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			if attempt == 0 && strings.Contains(string(out), "already exists") {
				continue
			}
			return "", "", fmt.Errorf("git worktree add failed: %s: %w", string(out), err)
		}
		break
	}

	// Run setup if config exists.
	if cfg != nil {
		runSetup(cfg, repoRoot, worktreePath, branch)
	}

	return worktreePath, branch, nil
}

// ListWorktrees returns CCC-managed worktrees for the given repo, sorted newest first.
func ListWorktrees(repoRoot string) ([]WorktreeInfo, error) {
	wtDir := filepath.Join(repoRoot, ".claude", "worktrees")
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Parse git worktree list to map paths to branches.
	branchMap, err := parseWorktreeList(repoRoot)
	if err != nil {
		return nil, err
	}

	// Determine prefix from config.
	prefix := "ccc"
	cfg, _ := LoadProjectConfig(repoRoot)
	if cfg != nil && cfg.Worktree.BranchPrefix != "" {
		prefix = cfg.Worktree.BranchPrefix
	}

	var infos []WorktreeInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		absPath := filepath.Join(wtDir, e.Name())
		// Resolve symlinks for consistent matching with git's output.
		resolvedPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			resolvedPath = absPath
		}
		branch, ok := branchMap[resolvedPath]
		if !ok {
			continue
		}
		// Only include worktrees whose branch matches our prefix pattern.
		if !strings.HasPrefix(branch, prefix+"/") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		infos = append(infos, WorktreeInfo{
			Path:      absPath,
			Branch:    branch,
			RepoRoot:  repoRoot,
			CreatedAt: info.ModTime(),
		})
	}

	// Sort newest first.
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].CreatedAt.After(infos[j].CreatedAt)
	})

	return infos, nil
}

// RemoveWorktree removes a worktree and deletes its branch.
func RemoveWorktree(repoRoot string, worktreePath string) error {
	// Determine branch before removal.
	branchMap, err := parseWorktreeList(repoRoot)
	if err != nil {
		return err
	}
	// Resolve symlinks for consistent matching with git's output.
	resolvedPath, resolveErr := filepath.EvalSymlinks(worktreePath)
	if resolveErr != nil {
		resolvedPath = worktreePath
	}
	branch := branchMap[resolvedPath]

	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove failed: %s: %w", string(out), err)
	}

	if branch != "" {
		cmd = exec.Command("git", "branch", "-D", branch)
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git branch -D failed: %s: %w", string(out), err)
		}
	}

	return nil
}

// PruneWorktrees removes all CCC-managed worktrees and returns the removed paths.
func PruneWorktrees(repoRoot string) ([]string, error) {
	worktrees, err := ListWorktrees(repoRoot)
	if err != nil {
		return nil, err
	}

	var removed []string
	for _, wt := range worktrees {
		if err := RemoveWorktree(repoRoot, wt.Path); err != nil {
			log.Printf("warning: failed to remove worktree %s: %v", wt.Path, err)
			continue
		}
		removed = append(removed, wt.Path)
	}

	return removed, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func gitRepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(out))
	// Evaluate symlinks for consistent paths (e.g., macOS /var -> /private/var).
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return root, nil
	}
	return resolved, nil
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// parseWorktreeList runs `git worktree list --porcelain` and returns a map
// of absolute worktree path -> branch name.
func parseWorktreeList(repoRoot string) (map[string]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list failed: %w", err)
	}

	result := make(map[string]string)
	var currentPath string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			// ref is like "refs/heads/ccc/2026-03-11-a1b2"
			branch := strings.TrimPrefix(ref, "refs/heads/")
			result[currentPath] = branch
		} else if line == "" {
			currentPath = ""
		}
	}

	return result, nil
}

func runSetup(cfg *ProjectConfig, repoRoot, worktreePath, branch string) {
	// Phase 1: Symlinks
	for _, entry := range cfg.Setup.Symlinks {
		src := filepath.Join(repoRoot, entry)
		dst := filepath.Join(worktreePath, entry)

		// Check source exists.
		if _, err := os.Lstat(src); err != nil {
			log.Printf("warning: symlink source %s does not exist, skipping", src)
			continue
		}

		// Create parent directory if needed.
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			log.Printf("warning: failed to create parent dir for symlink %s: %v", dst, err)
			continue
		}

		if err := os.Symlink(src, dst); err != nil {
			log.Printf("warning: failed to create symlink %s -> %s: %v", dst, src, err)
		}
	}

	// Phase 2: Scripts
	env := append(os.Environ(),
		"CCC_REPO_PATH="+repoRoot,
		"CCC_WORKTREE_PATH="+worktreePath,
		"CCC_BRANCH="+branch,
	)
	for _, script := range cfg.Setup.Scripts {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		cmd := exec.CommandContext(ctx, "sh", "-c", script)
		cmd.Dir = worktreePath
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("warning: setup script %q failed: %s: %v", script, string(out), err)
		}
		cancel()
	}
}
