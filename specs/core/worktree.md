# SPEC: Worktree-Based Session Isolation

## Purpose

When launching Claude in a project, optionally create a git worktree so the session gets its own branch and working copy. This prevents concurrent sessions from conflicting on file changes and provides clean per-session git history. Per-project setup scripts (symlinks, dependency installs) run after worktree creation to make the worktree a functional working environment.

## Interface

### Inputs
- **Repo root**: Detected via `git rev-parse --show-toplevel`
- **Project config**: `.ccc/config.yaml` in the repo root (optional)
- **Git state**: Current branch and worktree list from git

### Outputs
- **Worktree directory**: `.claude/worktrees/<branch-suffix>/` relative to repo root
- **Branch**: `ccc/<YYYY-MM-DD>-<4hex>` (e.g., `ccc/2026-03-11-a1b2`)
- **Setup results**: Symlink and script execution logs/warnings

### Dependencies
- `os/exec` for git commands and setup scripts
- `os` for symlinks and directory operations
- `gopkg.in/yaml.v3` for config parsing
- `crypto/rand` for hex suffix generation

## Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `LoadProjectConfig` | `(repoRoot string) (*ProjectConfig, error)` | Reads `.ccc/config.yaml` from repo root. Returns `nil, nil` if file does not exist. Returns error for invalid YAML. |
| `PrepareWorktree` | `(repoRoot string) (worktreePath, branch string, err error)` | Creates worktree in `.claude/worktrees/`, checks out new branch, runs setup. |
| `ListWorktrees` | `(repoRoot string) ([]WorktreeInfo, error)` | Lists CCC-created worktrees (branches matching `ccc/` prefix). |
| `RemoveWorktree` | `(repoRoot, worktreePath string) error` | Removes a worktree directory and deletes its branch. |
| `PruneWorktrees` | `(repoRoot string) ([]string, error)` | Removes all CCC worktrees for the repo. Returns list of removed paths. |

## Config Format

Project config lives at `.ccc/config.yaml` in the repo root:

```yaml
worktree:
  base_branch: main          # Branch to create worktree from (default: current branch)
  branch_prefix: ccc         # Prefix for worktree branches (default: "ccc")

setup:
  symlinks:
    - .env                   # Symlink from main repo to worktree
    - node_modules
    - .venv
  scripts:
    - "npm install"          # Run after worktree creation
    - "make generate"
```

### ProjectConfig Type

```go
type ProjectConfig struct {
    Worktree WorktreeConfig `yaml:"worktree"`
    Setup    SetupConfig    `yaml:"setup"`
}

type WorktreeConfig struct {
    BaseBranch   string `yaml:"base_branch"`   // default: current HEAD branch
    BranchPrefix string `yaml:"branch_prefix"` // default: "ccc"
}

type SetupConfig struct {
    Symlinks []string `yaml:"symlinks"` // paths relative to repo root
    Scripts  []string `yaml:"scripts"`  // shell commands
}
```

## Behavior

### Worktree Location

Worktrees are created under `.claude/worktrees/` relative to the repo root. The directory name matches the branch suffix (e.g., `2026-03-11-a1b2`). The `.claude/` directory should be added to `.gitignore`.

### Branch Naming

Format: `<prefix>/<YYYY-MM-DD>-<4hex>`

- Prefix from config `worktree.branch_prefix`, default `ccc`
- Date is current date
- 4 hex chars from `crypto/rand` for uniqueness
- Example: `ccc/2026-03-11-a1b2`

### PrepareWorktree Flow

1. Verify directory is a git repo (`git rev-parse --show-toplevel`)
2. Load project config via `LoadProjectConfig`; if missing, use defaults (no symlinks, no scripts)
3. Determine base branch: config `base_branch` or current HEAD branch
4. Generate branch name: `<prefix>/<date>-<4hex>`
5. Compute worktree path: `<repoRoot>/.claude/worktrees/<date>-<4hex>/`
6. Run `git worktree add -b <branch> <worktreePath> <baseBranch>`
7. If branch name collision: regenerate suffix, retry once; fail on second collision
8. Run setup: symlinks phase, then scripts phase
9. Return worktree path and branch name

### Setup Execution

Setup runs in two phases after worktree creation:

**Phase 1 — Symlinks:**
- For each path in `setup.symlinks`:
  - Source: `<repoRoot>/<path>`
  - Target: `<worktreePath>/<path>`
  - Create parent directories for target if needed
  - Call `os.Symlink(source, target)`
  - If source does not exist: log warning, continue
  - If target already exists: log warning, skip

**Phase 2 — Scripts:**
- For each command in `setup.scripts`:
  - Execute via `sh -c "<script>"`
  - Working directory: worktree path
  - Environment variables set:
    - `CCC_REPO_PATH` — absolute path to main repo root
    - `CCC_WORKTREE_PATH` — absolute path to worktree
    - `CCC_BRANCH` — branch name created for this worktree
  - Timeout: 120 seconds
  - If script fails (non-zero exit): log warning with stderr, continue

### ListWorktrees

1. Run `git worktree list --porcelain`
2. Parse output for worktree paths and branch names
3. Filter to branches matching the `ccc/` prefix (or configured prefix)
4. Return `[]WorktreeInfo{Path, Branch, CreatedAt}` where `CreatedAt` is parsed from the branch name date

### RemoveWorktree

1. Run `git worktree remove <worktreePath> --force`
2. Delete the branch: `git branch -D <branch>`
3. If worktree path doesn't exist, return error

### PruneWorktrees

1. Call `ListWorktrees` to get all CCC worktrees
2. Call `RemoveWorktree` for each
3. Return list of removed worktree paths
4. Errors on individual removals are logged as warnings; removal continues for remaining worktrees

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Not a git repo | `PrepareWorktree` returns error |
| `git worktree add` fails | Return error |
| Branch name collision | Regenerate suffix, retry once; fail on second collision |
| `.ccc/config.yaml` missing | `LoadProjectConfig` returns `nil, nil`; caller uses defaults |
| `.ccc/config.yaml` invalid YAML | `LoadProjectConfig` returns error; caller decides to warn and continue without setup |
| Symlink source missing | Log warning, continue with remaining symlinks |
| Symlink target exists | Log warning, skip |
| Setup script fails | Log warning with stderr output, continue with remaining scripts |
| Script timeout (120s) | Kill process, log warning, continue |

## Test Cases

### LoadProjectConfig
- Missing file returns `nil, nil` (no error)
- Valid YAML parses all fields correctly
- Invalid YAML returns parse error
- Empty file returns zero-value config, no error

### PrepareWorktree
- Creates directory under `.claude/worktrees/`
- Branch name matches `<prefix>/<YYYY-MM-DD>-<4hex>` pattern
- Uses config `base_branch` when specified
- Uses current HEAD when `base_branch` not specified
- Returns error when not in a git repo
- Retries once on branch name collision

### Symlinks
- Creates symlinks from main repo to worktree
- Creates parent directories for nested symlink targets
- Logs warning and continues when source path missing
- Logs warning and skips when target already exists

### Scripts
- Scripts run with working directory set to worktree
- Environment variables `CCC_REPO_PATH`, `CCC_WORKTREE_PATH`, `CCC_BRANCH` are set correctly
- Failed script logs warning, does not abort remaining scripts

### ListWorktrees
- Returns only CCC-prefixed worktrees (ignores other worktrees)
- Returns empty list when no CCC worktrees exist

### RemoveWorktree
- Removes worktree directory and deletes branch
- Returns error for non-existent worktree

### PruneWorktrees
- Removes all CCC worktrees for the repo
- Returns list of removed paths
- Continues on individual removal failures
