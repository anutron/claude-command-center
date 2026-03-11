---
name: wind-down
description: Save current session context to disk so you can close everything and resume later with /wind-up.
---

# Wind Down

Dump the current session's context to a file so it can be resumed later with `/wind-up`. Captures enough detail that a fresh Claude session can pick up exactly where this one left off. If there are uncommitted changes, saves them to a WIP branch.

## Arguments

- `$ARGUMENTS` - Optional: slug or topic name for the session file (e.g., "disk cleanup investigation")

## Context

- Project: !`pwd`
- Repo: !`basename $(git rev-parse --show-toplevel 2>/dev/null) 2>/dev/null || basename $(pwd)`
- Branch: !`git branch --show-current 2>/dev/null || echo "not a git repo"`
- Uncommitted changes: !`git status --short 2>/dev/null | head -30`
- Recent commits on branch: !`git log origin/HEAD..HEAD --oneline 2>/dev/null | head -15`
- Diff summary: !`git diff --stat 2>/dev/null | tail -10`
- Staged diff summary: !`git diff --cached --stat 2>/dev/null | tail -10`

## Instructions

### Step 1: Save Uncommitted Changes to WIP Branch

Check the "Uncommitted changes" context above.

**If there ARE uncommitted changes (staged, unstaged, or untracked files):**

1. Record the current branch name from context
2. Generate a slug (use `$ARGUMENTS` if provided, otherwise derive from conversation)
3. Create and switch to a WIP branch:
   ```bash
   git checkout -b "wip/wind-down/$(date +%Y-%m-%d-%H%M)-<slug>"
   ```
4. Stage everything and commit:
   ```bash
   git add -A && git commit -m "WIP: wind-down -- <one-line summary>"
   ```
5. Record the commit SHA:
   ```bash
   git rev-parse HEAD
   ```
6. Switch back to the original branch:
   ```bash
   git checkout <original-branch>
   ```

Save the WIP branch name and commit SHA for the session file.

**If there are NO uncommitted changes:**
Skip this step entirely. No WIP branch needed.

### Step 2: Generate Context Dump

Review the full conversation history and generate a comprehensive markdown document:

```markdown
# Session: [descriptive title]

## What We Were Working On
[Background, goal, what prompted this work -- 2-4 sentences]

## What Was Done
- [Completed work with specific file paths and what changed]
- [Be concrete: "Created ~/.claude/skills/disk-cleanup.md with scan targets"]

## Current State
[What's working, what's broken, where exactly we left off. Be specific about the last thing done and what the immediate next action would be.]

## Key Decisions Made
- [Decision]: [Rationale]

## Remaining Work
- [ ] [Specific actionable next step]
- [ ] [Another step]

## Important Context
- [Gotchas, constraints, things that aren't obvious]
- [User preferences expressed during the session]
- [Things we tried that didn't work and why]

## Files to Read First
1. [Most important file to understand current state]
2. [Next most important]

## Git State
- **Original branch**: [branch name]
- **WIP branch**: [wip/wind-down/... or "none"]
- **WIP commit**: [SHA or "none"]
- **Changes saved**: [brief diff stat summary, or "working directory was clean"]
```

### Step 3: Derive Summary and Slug

- **Summary**: Single sentence, max 80 chars, capturing what this session was about
- **Slug**: Use `$ARGUMENTS` if provided. Otherwise, derive a short kebab-case slug from the topic

### Step 4: Save the File

Pipe the markdown body to the paused-sessions CLI:

```bash
echo '<your markdown body>' | paused-sessions save \
  --project "<project path>" \
  --repo "<repo name>" \
  --branch "<branch>" \
  --summary "<summary>" \
  --slug "<slug>"
```

### Step 5: Save a Bookmark

Find the current session ID:

```bash
ls -t ~/.claude/projects/$(pwd | sed 's|/|-|g')/*.jsonl 2>/dev/null | head -1 | xargs basename | sed 's/.jsonl$//'
```

If found, save a bookmark:

```bash
ccc add-bookmark \
  --session-id "<uuid>" \
  --project "<project path>" \
  --repo "<repo name>" \
  --branch "<branch>" \
  --label "<slug>" \
  --summary "<summary>"
```

### Step 6: Create a Todo (if $ARGUMENTS provided)

If `$ARGUMENTS` was provided, add a todo to the Command Center so it shows up as an actionable item:

```bash
ccc add-todo \
  --title "<ARGUMENTS TEXT>" \
  --source "wind-down" \
  --context "<repo> (<branch>)" \
  --detail "<summary>" \
  --project-dir "<project path>" \
  --session-id "<session uuid>"
```

Replace the `<placeholder>` values with the actual values from previous steps.

### Step 7: Confirm

Tell the user:
- The file path where the session was saved
- The one-line summary
- Whether a WIP branch was created (and its name)
- If a todo was created, mention it shows up in the Command Center
- That they can resume with `/wind-up`
