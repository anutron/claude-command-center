---
name: wind-up
description: Resume a previously paused session saved by /wind-down. Filters to current project.
---

# Wind Up

Resume a paused session. Only shows sessions matching the current project directory. If the session saved a WIP branch, restores uncommitted changes to the working directory.

## Arguments

- `$ARGUMENTS` - Optional: filename of a specific session to resume

## Context

- Current project: !`pwd`
- Current branch: !`git branch --show-current 2>/dev/null || echo "not a git repo"`
- Available sessions for this project: !`paused-sessions list --project "$(pwd)" 2>/dev/null || echo "NO_SESSIONS"`
- Session count: !`paused-sessions list --project "$(pwd)" 2>/dev/null | wc -l | tr -d ' '`

## Instructions

### Step 1: Check for Sessions

Parse the available sessions from context. Each line is: `filename|created|repo|branch|summary|project`

**If session count is 0 or context shows NO_SESSIONS:**
Tell the user: "No paused sessions for this project. Run `/paused-sessions` to see sessions across all projects."
Stop here.

**If `$ARGUMENTS` contains a filename:**
Use that filename directly -- skip to Step 3.

**If exactly 1 session:**
Tell the user which session was found (date, summary) and proceed to Step 3 automatically.

**If multiple sessions:**
Present a numbered list showing each session's date, branch, and summary. Ask the user which one to resume.

### Step 2: Wait for Selection (only if multiple)

Wait for user to pick a session number.

### Step 3: Resume the Session

Run:
```bash
paused-sessions resume "<filename>"
```

This outputs the full session file content and moves it to the resumed archive.

### Step 4: Restore WIP Branch (if present)

Parse the session file output. Look for the "Git State" section.

**If a WIP branch and commit are listed:**

1. Verify the WIP branch exists:
   ```bash
   git branch --list "wip/wind-down/*"
   ```

2. Check we're on the correct original branch. If not:
   ```bash
   git checkout <original-branch>
   ```

3. Cherry-pick the WIP commit to apply the saved changes:
   ```bash
   git cherry-pick <wip-commit-sha>
   ```

4. Reset to undo the commit but keep all changes in the working directory:
   ```bash
   git reset HEAD^
   ```

5. Delete the WIP branch:
   ```bash
   git branch -D <wip-branch-name>
   ```

**If cherry-pick fails (conflicts):**
- Tell the user: "WIP restore had conflicts. The WIP branch `<name>` is still intact."
- Run `git cherry-pick --abort` to clean up
- Let the user decide how to proceed (manual merge, skip restore, etc.)

**If no WIP branch listed, or WIP branch was already deleted:**
Skip this step. Resume normally.

### Step 5: Absorb Context

Read the session output carefully. Internalize:
- What was being worked on and why
- What was already done
- Current state and where things left off
- Key decisions and their rationale
- Remaining work items
- Important gotchas and context

### Step 6: Brief the User

Provide a concise summary:
1. **Resumed**: [session title and date]
2. **Where we left off**: [1-2 sentences on current state]
3. **Changes restored**: [whether WIP branch was applied, or "working directory was clean"]
4. **Suggested next step**: [the first item from "Remaining Work"]

Ask the user if they want to continue with the suggested next step or do something else.
