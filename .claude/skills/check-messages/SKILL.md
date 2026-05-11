---
name: check-messages
description: Show unread orchestrator inbox messages for this session — works on both sides of the orchestrator/worker relationship. From an orchestrator session, lists messages addressed to "orchestrator". From a worker session, resolves the role from worktree/branch and lists messages addressed to that role. Use when checking for new updates.
user_invocable: true
---

# Check Messages

Show what's waiting in the inbox for the current side of the conversation.

## Step 1: Figure out which side we're on

```bash
SESSION_ID="${CCC_SESSION_ID:-$(cat ~/.claude/session-topics/pid-$PPID.map 2>/dev/null)}"
TOPIC=""
if [ -n "$SESSION_ID" ]; then
  TOPIC=$(cat ~/.claude/session-topics/${SESSION_ID}.txt 2>/dev/null)
fi
```

- If `$TOPIC` starts with `ORCHESTRATE: `, we're on the **orchestrator side**. The orchestrator name is everything after the prefix. Recipient is `orchestrator`.
- Otherwise we're on the **worker side**. Resolve role + orchestrator:

  ```bash
  PWD_NOW=$(pwd)
  ccc orchestrator inbox resolve-role --worktree "$PWD_NOW" --project "$PWD_NOW" --json
  ```

  Match the resulting `{orchestrator, role}` the same way `/orchestrate` does:
  - Empty → ask the user which orchestrator + role to check, or tell them no thread exists for this worktree.
  - One match → use it.
  - Many matches → ask which one.

After this step we have:
- `$ORCH_NAME`
- `$RECIPIENT` (= `orchestrator` on the orchestrator side, or the role name on the worker side)

## Step 2: Read unread messages

Pass `--orchestrator` explicitly. On the orchestrator side the flag is redundant (the topic would resolve it anyway), but using it uniformly keeps the recipe identical on both sides:

```bash
ccc orchestrator inbox list --orchestrator "$ORCH_NAME" --unread --to "$RECIPIENT" --json
```

## Step 3: Render the messages

Render each message as a tight block:

```
─── #<id>  <kind>  from <from>  at <ts> ───
  <body>
  [metadata if present: project / branch / worktree / session_id / topic]
```

If there are no unread messages, just say:

> No new messages for `<recipient>` in orchestrator `<orch>`.

If there are many (more than ~5), show the first few and offer to show the rest.

## Step 4: Offer to mark read

After rendering, ask:

> Mark these as read? (yes / no / up-to <id>)

Defaults:

- **yes** → `ccc orchestrator inbox mark-read --orchestrator "$ORCH_NAME" --to "$RECIPIENT"` (advances cursor to the highest existing id)
- **no** → leave the cursor alone
- **up-to N** → `ccc orchestrator inbox mark-read --orchestrator "$ORCH_NAME" --to "$RECIPIENT" --up-to N`

## Step 5: Suggest follow-ups (optional)

After showing the messages, suggest natural next actions based on `kind`:

- **checkin** from a worker → "Want me to update the thread status?" / "Send any guidance back?"
- **question** from a worker → "Want to discuss this and log a decision?"
- **handoff** to a worker (worker side) → "Want me to run `/orchestrate <role>` to claim it?" (rarely the case — usually `/orchestrate` is invoked directly)
- **update** → no automatic suggestion; just informational

Don't act on these suggestions automatically. Wait for the user.

## Notes

- **Read-only by default until the mark-read step.** Listing doesn't mutate state — the cursor only moves when the user confirms.
- **Broadcast messages (`to: "*"`) appear for all recipients.** That's intentional and rare; useful for "everyone restart" announcements.
- **No clipboard interaction.** This skill is purely inbox-driven.
