---
name: paused-sessions
description: List all paused sessions across all projects. Dashboard view — use /wind-up to actually resume.
---

# Paused Sessions

List all paused sessions across all projects. This is the cross-project dashboard — to resume a session, `cd` to its project directory and run `/wind-up`.

## Arguments

- `$ARGUMENTS` - Optional: "clean" to prune old resumed sessions, or "clean --days N" for custom age

## Context

- All paused sessions: !`paused-sessions list --all 2>/dev/null || echo "NO_SESSIONS"`

## Instructions

### If `$ARGUMENTS` contains "clean":

Run `paused-sessions clean` (or `paused-sessions clean --days N` if a number was specified).
Report how many old resumed sessions were deleted.

### Otherwise: Display Dashboard

Parse the session list from context. Each line is: `filename|created|repo|branch|summary|project`

**If no sessions:**
Tell the user: "No paused sessions. Use `/wind-down` to save a session before closing."

**If sessions exist:**
Display a formatted table:

```
| Date       | Project          | Branch | Summary                          |
|------------|------------------|--------|----------------------------------|
| 2026-03-04 | AI-RON           | main   | Building disk cleanup skill      |
| 2026-03-03 | keystone         | feat-x | Adding new API endpoint          |
```

After the table, remind the user:
- To resume: `cd <project path>` then `/wind-up`
- To clean old sessions: `/paused-sessions clean`
