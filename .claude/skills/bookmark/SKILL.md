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

## Instructions

### Step 1: Find the Current Session ID

The current session's JSONL file is the most recently modified `.jsonl` in the Claude sessions directory for this project.

Run:
```bash
ls -t ~/.claude/projects/$(pwd | sed 's|/|-|g')/*.jsonl 2>/dev/null | head -1 | xargs basename | sed 's/.jsonl$//'
```

This gives you the session UUID.

If no session file is found, tell the user the bookmark can't be created (session not persisted yet).

### Step 2: Generate Label and Summary

- **Label**: Use `$ARGUMENTS` if provided. Otherwise, generate a short label from the conversation context (what we were working on).
- **Summary**: Write a one-line summary (max 80 chars) of the session's work.

### Step 3: Save the Bookmark

Run:
```bash
ccc add-bookmark \
  --session-id "<uuid>" \
  --project "<project path>" \
  --repo "<repo name>" \
  --branch "<branch>" \
  --label "<label>" \
  --summary "<summary>"
```

### Step 4: Create a Todo (if $ARGUMENTS provided)

If `$ARGUMENTS` was provided, add a todo to the Command Center so it shows up as an actionable item that resumes this session:

```bash
ccc add-todo \
  --title "<ARGUMENTS TEXT>" \
  --source "bookmark" \
  --context "<repo> (<branch>)" \
  --detail "<summary>" \
  --project-dir "<project path>" \
  --session-id "<session uuid>"
```

Replace the `<placeholder>` values with the actual values from previous steps.

### Step 5: Confirm

Tell the user:
- The bookmark was saved
- The label/summary
- If a todo was created, mention it shows up in the Command Center
- They can resume from the Command Center's sessions view
