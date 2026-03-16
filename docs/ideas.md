# Ideas & Future Explorations

Parking lot for ideas that come up during development. Not committed to — just captured.

---

## Slack Channel as Todo Agent Intake

Private Slack channel dedicated to the user's todo agent. The user forwards threads, drops links, or types natural language commands into this channel.

**Flow:**
1. User forwards a Slack thread to their private todo-agent channel
2. Refresh agent picks it up (like the todo label in email)
3. Refresh interprets it as a command — could create a todo, book a calendar event, etc.
4. Agent posts a reply in the Slack thread with: "I have this todo with the following prompt: `<prompt>` in project `<dir>`"
5. User gives an emoji response (e.g., thumbs up) to approve
6. Next refresh cycle detects the emoji → kicks off the headless session
7. When the agent finishes, CCC posts back to the thread that it's ready for review

**Safety**: 100% of these funnel into todos with prompts the user reviews. No autonomous execution without prompt review. The Slack interaction is just a more convenient approval interface.

**Related**: Todo Agent Launcher (implemented 2026-03-14) provides the headless session infrastructure this builds on.

---

## Live Agent Session Viewer *(in progress)*

Bidirectional stream-json viewer built into the TUI. Watch agent activity live, send messages/answers via stdin pipe, or join interactively and re-queue headless.

**Plan:** `~/.claude/plans/ticklish-floating-dragon.md`

---

## Smart Launch Mode Suggestion

The prompt-generation skill (`/todo-agent`) suggests Worktree mode when it detects the task involves code modifications to the target repo. Normal mode for everything else (research, docs, external API calls). User always overrides in the task runner.

---

## Status Line Updates for Spawned Claude Instances

When a Claude instance is spawned from a todo (headless or interactive), update the user's Claude Code status line to reflect it. This gives visibility into agent activity without switching tabs or checking the todo list.

**Possible display:**
- `🤖 Running: "Fix auth bug" (2m)` — active agent with task name and elapsed time
- `🤖 2 agents running` — summary when multiple are active
- Show in CCC's own status bar and/or pipe to Claude Code's status line config

**Prerequisite**: Todo Agent Launcher's session tracking. Needs a way to detect agent start/completion events — could use the EventBus or poll session status.
