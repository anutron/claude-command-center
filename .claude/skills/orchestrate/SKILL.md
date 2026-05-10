---
name: orchestrate
description: From a fresh worker session, intake an orchestrator handoff. Parses the PASTE INTO block, sets the session topic, and copies a "checkin" handoff back to clipboard so the orchestrator can register this session as a thread. Use right after pasting an orchestrator handoff into a new session.
user_invocable: true
---

# Orchestrate (worker intake)

The bookend on the worker side of an orchestrator -> worker handoff. Used when:

- The orchestrator session ran something like `/handoff <task>` (or emitted a `paste-header` block) and put a payload on your clipboard.
- You opened a new terminal in the right project/worktree and pasted the payload.
- You want to register this session with the orchestrator without typing the full `/ask-orchestrator give me a checkin ...` ceremony.

`/orchestrate` does three things:

1. Parses the PASTE INTO block from the handoff payload.
2. Sets the session topic to the `ccc topic` value the orchestrator expects.
3. Generates a "checkin" HANDOFF TO ORCHESTRATOR block on your clipboard so you can paste it back to the orchestrator terminal.

It does **not** start executing the task. After running it, paste the checkin to the orchestrator, then tell Claude "go" (or whatever) to begin the actual work.

## Arguments

- `$ARGUMENTS` — optional. May be (a) the entire pasted handoff content, or (b) a free-form instruction like "go" or "start". If args contain a `PASTE INTO:` line, treat them as the handoff payload.

## Step 1: Locate the handoff payload

Try sources in this order. Stop at the first one that contains a line matching `PASTE INTO:`:

1. **`$ARGUMENTS`** — if it contains `PASTE INTO:`, use it.
2. **Recent conversation** — scan the previous 1–2 user messages for a block containing `PASTE INTO:`. Use the matching block.
3. **Clipboard** — last resort:

   ```bash
   pbpaste
   ```

   If the clipboard contains `PASTE INTO:`, use it.

If none of the three sources contain a PASTE INTO block, tell the user:

> I can't find an orchestrator handoff to intake. Paste the orchestrator's PASTE INTO block (and instructions) and re-run `/orchestrate`, or pass the content as args.

Then stop.

## Step 2: Parse the PASTE INTO block

Extract these fields from the block (regex on the lines):

- **Thread name** — the value after `PASTE INTO:` on the header line (between the dashes)
- **Project** — `Project:  <path>`
- **Worktree** — `Worktree: <path>` (may be `(none)`)
- **Expected topic** — `ccc topic: "<text>"` (the quoted value)

Also capture everything below the PASTE INTO block as the **task body** — that's the actual work the orchestrator wants done. You'll summarize it for the user in Step 6.

If any required field is missing (thread name, expected topic), abort and ask the user to re-paste.

## Step 3: Sanity-check the local environment

```bash
PWD_NOW=$(pwd)
BRANCH_NOW=$(git branch --show-current 2>/dev/null || echo "")
```

Compare to the parsed Project / Worktree:

- If `Worktree` is set and not `(none)` and `$PWD_NOW` is not under that worktree path → warn the user that they may be in the wrong directory. Ask whether to proceed.
- If `Worktree` is unset and `$PWD_NOW` is not under `Project` → warn similarly.
- Otherwise proceed silently.

Don't `cd` for the user — surface the mismatch so they can decide.

## Step 4: Set the session topic

Write the topic file directly (same pattern as `orchestrator` and `set-topic`):

```bash
SESSION_ID=$(cat ~/.claude/session-topics/pid-$PPID.map 2>/dev/null)
if [ -z "$SESSION_ID" ]; then
  echo "Could not resolve session ID — /orchestrate needs a Claude session"
  exit 1
fi
printf '%s' "$EXPECTED_TOPIC" > ~/.claude/session-topics/${SESSION_ID}.txt
```

Use the parsed `ccc topic` value verbatim — that's what the orchestrator expects to see.

If a topic is already set on this session and it differs from the expected topic, ask before overwriting:

> A topic is already set: `<existing>`. The orchestrator expects `<expected>`. Overwrite?

## Step 5: Find the owning orchestrator

Scan orchestrator state files for the thread name:

```bash
for STATE in ~/.claude/orchestrators/*/state.md; do
  if grep -qF "## $THREAD_NAME" "$STATE"; then
    echo "$(basename "$(dirname "$STATE")")"
  fi
done
```

- **Exactly one match** — use that orchestrator name.
- **Multiple matches** — show the user the list (with `started_at` from each `state.md` frontmatter) and ask which one.
- **No match** — the orchestrator hasn't registered this thread yet. Ask the user to confirm the orchestrator name; default to the result of `ccc orchestrator list --json` (single active) or prompt to pick.

## Step 6: Build the checkin handoff and copy to clipboard

Gather local context:

```bash
PROJECT=$(pwd)
REPO=$(basename "$(git rev-parse --show-toplevel 2>/dev/null || pwd)")
BRANCH=$(git branch --show-current 2>/dev/null || echo "")
WORKTREE=""
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  TOPLEVEL=$(git rev-parse --show-toplevel)
  COMMON_DIR=$(git rev-parse --git-common-dir 2>/dev/null)
  if [ -n "$COMMON_DIR" ] && [ "$(dirname "$COMMON_DIR")" != "$TOPLEVEL" ]; then
    WORKTREE="$TOPLEVEL"
  fi
fi
FROM_SESSION="${CCC_SESSION_ID:-$SESSION_ID}"
```

Compose the checkin block:

```
─── HANDOFF TO ORCHESTRATOR: <orchestrator-name> ───
  Type:       checkin (starting work)
  Thread:     <thread-name>
  From session: <from-session>
  Project:    <project>
  Repo:       <repo>
  Branch:     <branch>
  Worktree:   <worktree or "(none)">
  Status:     starting
  ────────────────────────────────────────────────
  Picked up the handoff and set topic to "<expected-topic>". Beginning work
  now. Will report back when blocked, when a decision is needed, or on
  completion.

  Suggested orchestrator action:
    ccc orchestrator thread add \
      --name "<thread-name>" \
      --project "<project>" \
      --branch "<branch>" \
      --session-id "<from-session>" \
      --worktree "<worktree>" \
      --status "in-progress"
    (or thread set-status if the thread already exists)
```

Copy to clipboard:

```bash
printf '%s' "$BLOCK" | pbcopy
```

If `pbcopy` fails or isn't available, print the block to the screen and tell the user to copy manually.

## Step 7: Summarize and hand control back to the user

Print a tight summary:

- **Topic set:** `<expected-topic>`
- **Orchestrator:** `<name>`
- **Thread:** `<thread-name>`
- **Task (one sentence):** distilled from the task body
- **Clipboard:** checkin block ready to paste back to the orchestrator

Then:

> Paste the checkin into the `<orchestrator-name>` terminal. When you're ready to start the actual work, say "go" (or describe how you'd like to proceed) and I'll dive in.

Do **not** start executing the task in this turn. The user will tell you when to begin — they may want to paste the checkin first so the orchestrator registers the thread, or they may want to adjust scope before starting.

## Notes

- **Read-only on orchestrator state.** This skill never mutates orchestrator files. It reads `state.md` to find the owning orchestrator and emits a clipboard block — registering the thread is the orchestrator's job.
- **Session topic is the contract.** The orchestrator's `paste-header` declares the expected topic. We honor that exactly so future `/ask-orchestrator` calls from this session land on the right thread.
- **Don't include secrets or large file dumps in the checkin.** It's a short status message, not a payload. The orchestrator already has the task text from its own side.
