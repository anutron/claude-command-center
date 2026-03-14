---
name: bookmark
description: Save a reference to this session for easy resume. No context dump — just a pointer to the real session.
---

# Bookmark

Save the current Claude Code session as a bookmark so it can be resumed later. Unlike `/wind-down`, this does NOT dump context — it just saves a pointer to the real session.

If `$ARGUMENTS` is provided, also creates a todo in the Command Center so the bookmark appears as an actionable item.

## Arguments

- `$ARGUMENTS` - Optional: label for the bookmark (e.g., "release the foo changes"). Also becomes a todo title if provided.

## Context

- Project: !`pwd`
- Repo: !`basename $(git rev-parse --show-toplevel 2>/dev/null) 2>/dev/null || basename $(pwd)`
- Branch: !`git branch --show-current 2>/dev/null || echo "not a git repo"`
- Claude sessions dir: !`echo ~/.claude/projects/$(pwd | sed 's|/|-|g')`
- Is worktree: !`git rev-parse --git-common-dir 2>/dev/null | grep -q '\.git$' && echo "no" || echo "yes"`
- Main repo: !`git worktree list 2>/dev/null | head -1 | awk '{print $1}'`

## Instructions

### Step 1: Detect Worktree

Determine if the current directory is a git worktree (not the main working tree):

```bash
# Check if this is a worktree by comparing git-common-dir to git-dir
GIT_COMMON=$(git rev-parse --git-common-dir 2>/dev/null)
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
if [ "$GIT_COMMON" != "$GIT_DIR" ] && [ -n "$GIT_COMMON" ]; then
  IS_WORKTREE=true
  # Main repo is the parent of .git/worktrees
  SOURCE_REPO=$(cd "$GIT_COMMON/.." && pwd)
  WORKTREE_PATH=$(pwd)
  # Claude stores sessions under the main repo's project dir
  SESSIONS_DIR=~/.claude/projects/$(echo "$SOURCE_REPO" | sed 's|/|-|g')
else
  IS_WORKTREE=false
  SESSIONS_DIR=~/.claude/projects/$(pwd | sed 's|/|-|g')
fi
```

### Step 2: Find the Current Session ID

The current session's JSONL file is the most recently modified `.jsonl` in the sessions directory.

Run:
```bash
ls -t "$SESSIONS_DIR"/*.jsonl 2>/dev/null | head -1 | xargs basename | sed 's/.jsonl$//'
```

This gives you the session UUID.

If no session file is found, tell the user the bookmark can't be created (session not persisted yet).

### Step 3: Generate Label and Summary

- **Label**: Use `$ARGUMENTS` if provided. Otherwise, generate a short label from the conversation context (what we were working on).
- **Summary**: Write a one-line summary (max 80 chars) of the session's work.

### Step 4: Save the Bookmark

For **worktree sessions**, use `--source-repo` (main repo path) as the project (where Claude looks for sessions), and `--worktree-path` to record the worktree directory:

```bash
ccc add-bookmark \
  --session-id "<uuid>" \
  --project "<SOURCE_REPO>" \
  --repo "<repo name>" \
  --branch "<branch>" \
  --label "<label>" \
  --summary "<summary>" \
  --worktree-path "<WORKTREE_PATH>" \
  --source-repo "<SOURCE_REPO>"
```

For **normal sessions** (not a worktree):

```bash
ccc add-bookmark \
  --session-id "<uuid>" \
  --project "<project path>" \
  --repo "<repo name>" \
  --branch "<branch>" \
  --label "<label>" \
  --summary "<summary>"
```

### Step 5: Create a Todo (if $ARGUMENTS provided)

If `$ARGUMENTS` was provided, add a todo to the Command Center so it shows up as an actionable item that resumes this session:

```bash
ccc add-todo \
  --title "<ARGUMENTS TEXT>" \
  --source "bookmark" \
  --context "<repo> (<branch>)" \
  --detail "<summary>" \
  --project-dir "<project path used in step 4>" \
  --session-id "<session uuid>"
```

Replace the `<placeholder>` values with the actual values from previous steps.

### Step 6: Confirm

Tell the user:
- The bookmark was saved
- The label/summary
- If this was a worktree session, note that resuming will open Claude in the main repo (the session context knows about the worktree)
- If a todo was created, mention it shows up in the Command Center
- They can resume from the Command Center's sessions view
