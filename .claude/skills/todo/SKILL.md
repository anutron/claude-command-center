---
name: todo
description: Capture a todo item to the Claude Command Center. Use when the user says "todo", "remind me", "add a task", or wants to capture something for later.
user-invocable: true
---

# Add Todo

Capture a todo item to the Claude Command Center dashboard.

## Arguments

- `$ARGUMENTS` - Required: the todo title, and optionally details

## Instructions

Parse `$ARGUMENTS` into a title and optional details. If the user provided a single phrase, that's the title. If they provided more context, split into title (concise) and detail (the rest).

Run:

```bash
ccc add-todo \
  -title "<title>" \
  -source "claude-code" \
  -project-dir "$(pwd)" \
  ${detail:+-detail "<detail>"} \
  ${due:+-due "<YYYY-MM-DD>"} \
  ${who_waiting:+-who-waiting "<name>"}
```

Optional fields — only include if the user mentioned them:
- `-detail` — extra context beyond the title
- `-due` — if the user mentioned a deadline, convert to YYYY-MM-DD
- `-who-waiting` — if someone is waiting on this

Confirm with one line: "Added: **<title>**"
