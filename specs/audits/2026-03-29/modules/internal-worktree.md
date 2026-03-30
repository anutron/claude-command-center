# Spec Audit: internal/worktree

**Date:** 2026-03-29
**Spec:** specs/core/worktree.md

---

## internal/worktree/worktree.go

### ProjectConfig / WorktreeConfig / SetupConfig types

- **[COVERED]** worktree.md "Config Format" / "ProjectConfig Type": exact match for all fields and YAML tags

### LoadProjectConfig

- **[COVERED]** worktree.md "Functions" table and "Error Handling": reads .ccc/config.yaml, returns nil,nil if missing, error for invalid YAML
- **[COVERED]** worktree.md: default BranchPrefix = "ccc" — code applies this default on line 65

### PrepareWorktree

- **[COVERED]** worktree.md "PrepareWorktree Flow" steps 1-9: git repo root, load config, base branch, branch name, worktree path, git worktree add, collision retry, setup
- **[CONTRADICTS]** worktree.md says signature is `(repoRoot string)` but code signature is `(dir string)` where dir is any directory inside a git repo (internally resolves to repo root). The spec "Functions" table says input is "repoRoot" but the spec "Interface Inputs" says "Repo root: Detected via git rev-parse --show-toplevel". The code takes `dir` (any path) and resolves it. Spec should say `dir` not `repoRoot`.
- **[COVERED]** worktree.md "Branch Naming": `<prefix>/<YYYY-MM-DD>-<4hex>` — matches code exactly
- **[COVERED]** worktree.md: "Retry once on branch name collision" — code retries with `for attempt := 0; attempt < 2`
- **[COVERED]** worktree.md "Worktree Location": `.claude/worktrees/` relative to repo root

### ListWorktrees

- **[COVERED]** worktree.md "ListWorktrees": parses `git worktree list --porcelain`, filters by prefix, returns WorktreeInfo
- **[COVERED]** worktree.md: "Returns empty list when no CCC worktrees exist" — returns nil,nil if dir doesn't exist
- **[UNCOVERED-BEHAVIORAL]** Sorted newest first by `CreatedAt` (filesystem ModTime). Spec says "CreatedAt is parsed from the branch name date" but code uses `info.ModTime()` not branch name parsing. **Intent question:** Clarify CreatedAt source — spec says branch name, code uses filesystem mtime.
- **[UNCOVERED-BEHAVIORAL]** Symlink resolution (`filepath.EvalSymlinks`) for consistent matching with git's output. Not in spec.

### RemoveWorktree

- **[COVERED]** worktree.md "RemoveWorktree": `git worktree remove --force`, then `git branch -D`
- **[UNCOVERED-BEHAVIORAL]** Resolves symlinks before looking up branch in branchMap. Not in spec.

### PruneWorktrees

- **[COVERED]** worktree.md "PruneWorktrees": removes all CCC worktrees, returns removed paths, continues on individual failures

### runSetup

- **[COVERED]** worktree.md "Setup Execution" Phase 1 (Symlinks): source/target, parent dirs, warnings on missing source
- **[COVERED]** worktree.md "Setup Execution" Phase 2 (Scripts): sh -c, working dir, env vars (CCC_REPO_PATH, CCC_WORKTREE_PATH, CCC_BRANCH), 120s timeout, warning on failure
- **[UNCOVERED-BEHAVIORAL]** Code does not check for pre-existing symlink target — it calls `os.Symlink` directly (which will error if target exists). Spec says "If target already exists: log warning, skip" but code does not explicitly check. The os.Symlink error would produce a log message via the `log.Printf` pattern, but the message wouldn't say "skip".

### gitRepoRoot

- **[COVERED]** worktree.md "Interface Inputs": "Detected via git rev-parse --show-toplevel"
- **[UNCOVERED-BEHAVIORAL]** Resolves symlinks for consistent paths (macOS /var -> /private/var). Not in spec.

### parseWorktreeList

- **[COVERED]** worktree.md: "Run git worktree list --porcelain" — implementation matches

---

## Spec -> Code Direction Gaps

1. **worktree.md "WorktreeInfo"** says `CreatedAt` is "parsed from the branch name date" but code uses filesystem ModTime — **CONTRADICTS**.
2. No other spec->code gaps found.

---

## Summary

- **CONTRADICTS: 2** — PrepareWorktree parameter name (dir vs repoRoot), CreatedAt source (mtime vs branch name)
- **UNCOVERED-BEHAVIORAL: 3** — Symlink resolution in list/remove/gitRepoRoot, pre-existing symlink target handling
- **COVERED: ~15 behavioral paths** — Good coverage overall, with two factual contradictions to fix.
