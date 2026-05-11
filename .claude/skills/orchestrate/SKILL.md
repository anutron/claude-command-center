---
name: orchestrate
description: From a fresh worker session, claim an orchestrator-assigned role and intake its handoff. Reads the role's pending handoff from the orchestrator's inbox, sets the worker's session topic, and writes a checkin back to the inbox. Use right after opening a worker terminal in the target worktree.
user_invocable: true
---

# Orchestrate (worker intake)

The bookend on the worker side of an orchestrator → worker handoff. Used when:

- The orchestrator session has written one or more handoff messages to its inbox, each addressed to a role name like `a`, `b`, or `wave-0b`.
- The user has opened a fresh worker terminal in the target worktree.
- The user types `/orchestrate <role>` (or just `/orchestrate` if role resolution by worktree finds a single match).

`/orchestrate` does three things:

1. Finds the right orchestrator + handoff message for this role.
2. Sets the session topic to whatever the orchestrator declared as the worker topic.
3. Writes a `checkin` message back to the inbox with this terminal's project/branch/worktree/session-id.

It does **not** start executing the task. After it runs, the orchestrator's next `/check-messages` (or `ccc orchestrator inbox list --unread --to orchestrator`) will see the checkin. When the user says "go," begin the actual work.

## Arguments

- `$ARGUMENTS` — optional role name (`a`, `wave-0b`, etc.). If omitted, the skill tries to infer the role by resolving the current worktree against active orchestrators' threads.

## Step 1: Resolve which orchestrator and role this terminal belongs to

```bash
PWD_NOW=$(pwd)
ccc orchestrator inbox resolve-role --worktree "$PWD_NOW" --project "$PWD_NOW" --json
```

The output is a JSON array of `{orchestrator, role, project, worktree}` entries.

- **If `$ARGUMENTS` was provided**, filter to entries with matching `role`. If exactly one matches, use it. If none match, ask the user whether to proceed anyway (orchestrator may not have created the thread yet); they can paste the orchestrator name explicitly.
- **If `$ARGUMENTS` was empty**, the array determines the action:
  - Empty array → ask the user for the orchestrator name and role.
  - Exactly one entry → use it.
  - Multiple entries → show the list and ask which one.

After this step you have `$ORCH_NAME` and `$ROLE`.

## Step 2: Read the pending handoff

Worker sessions don't have an `ORCHESTRATE:` topic, so pass `--orchestrator` explicitly. No env-var trick needed.

```bash
ccc orchestrator inbox list --orchestrator "$ORCH_NAME" --to "$ROLE" --kind handoff --json
```

From the JSON array, pick the message with the highest `id` whose `from == "orchestrator"`. That's the latest handoff.

If no handoff message exists for this role, tell the user:

> No handoff message for role `<role>` in orchestrator `<orch>`. The orchestrator may not have written one yet, or this role may already have been intook by a previous session. Want me to check for unread messages instead?

Extract from the chosen message:

- `body` — the task description
- `topic` — the worker topic to set (may be empty)
- `project`, `branch`, `worktree` — target metadata
- `id` — needed later for `mark-read`

## Step 3: Sanity-check the local environment

Compare current pwd / branch to the handoff's `project` / `branch` / `worktree`. If there's a mismatch, surface it as a warning and ask whether to proceed — never `cd` for the user.

## Step 4: Set the session topic

```bash
SESSION_ID="${CCC_SESSION_ID:-$(cat ~/.claude/session-topics/pid-$PPID.map 2>/dev/null)}"
if [ -z "$SESSION_ID" ]; then
  echo "Could not resolve session ID — /orchestrate needs a Claude session"
  exit 1
fi
WORKER_TOPIC="${HANDOFF_TOPIC:-$ROLE}"
printf '%s' "$WORKER_TOPIC" > ~/.claude/session-topics/${SESSION_ID}.txt
```

If a topic is already set on this session and it differs, ask before overwriting.

## Step 5: Write the checkin

```bash
PROJECT=$(pwd)
BRANCH=$(git branch --show-current 2>/dev/null || echo "")
WORKTREE=""
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  TOPLEVEL=$(git rev-parse --show-toplevel)
  COMMON=$(git rev-parse --git-common-dir 2>/dev/null)
  if [ -n "$COMMON" ] && [ "$(dirname "$COMMON")" != "$TOPLEVEL" ]; then
    WORKTREE="$TOPLEVEL"
  fi
fi

ccc orchestrator inbox send \
  --orchestrator "$ORCH_NAME" \
  --to orchestrator \
  --from "$ROLE" \
  --kind checkin \
  --project "$PROJECT" \
  --branch "$BRANCH" \
  --worktree "$WORKTREE" \
  --session-id "$SESSION_ID" \
  --body "Picked up handoff. Topic set to \"$WORKER_TOPIC\". Ready to start."
```

## Step 6: Mark the handoff read

```bash
ccc orchestrator inbox mark-read --orchestrator "$ORCH_NAME" --to "$ROLE" --up-to "$HANDOFF_ID"
```

## Step 7: Summarize and hand control back to the user

Print a tight summary:

- **Orchestrator:** `<orch>`
- **Role:** `<role>`
- **Topic set:** `<worker-topic>`
- **Task (one sentence):** distilled from the handoff body
- **Checkin sent.**

Then:

> Checkin is in the orchestrator's inbox. When you're ready to start the work, say "go" (or describe how you'd like to proceed) and I'll dive in.

Do **not** start executing the task in this turn. The user will tell you when to begin.

## Notes

- **Pass `--orchestrator $ORCH_NAME` to every inbox call.** Worker sessions have their own topic (the worker topic, e.g. `wave-0b`), not `ORCHESTRATE: ...`. The flag bypasses topic resolution so we never have to fake one.
- **No clipboard handling here.** This is the inbox-based version of the workflow. The clipboard `PASTE INTO` flow has been retired in favor of durable, queryable messages.
- **Don't include secrets or large file dumps in the checkin body.** Keep it a short status sentence. The orchestrator already has the task body.
