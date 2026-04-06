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

---

## CCC as a Global MCP Server

Expose CCC's data and tools as an MCP server available to all Claude sessions on the machine — not just agents CCC spawns, but any manual session in any iTerm tab.

**Tool surface (brainstorm):**
- Read calendar, PRs, todos, slack context
- Query agent status (what's running, what's blocked)
- Create/update todos from any session
- Agent coordination — "what are other agents working on?" to avoid conflicts

**Key question:** What's the right boundary between read-only queries and actionable tools? Need to design the tool surface carefully.

**Origin:** Inspired by Argus's MCP injection pattern (auto-injects KB tools into every agent), but broader — CCC would be a live system API, not just a knowledge base.

**Prerequisite:** Daemon (#2 below) would be the natural host for the MCP HTTP server, since it's already a persistent process.

---

## Terminal Tooling for Multi-Agent Workflows

Recommendations from a fellow engineer for improving the agentic coding flow. Three tools worth tracking:

### diffnav — TUI diff pager with file tree
- **What:** Git diff pager with a GitHub-style file tree sidebar for navigating diffs. Built on delta.
- **URL:** https://github.com/dlvhdr/diffnav
- **Why:** Reviewing agent-generated diffs across many files is painful with raw `git diff`. File tree navigation is a big UX win.
- **Install:** `brew install dlvhdr/formulae/diffnav`
- **Status:** Open source (MIT), Go, actively maintained. Same author as `gh-dash`.
- **Verdict:** Adopt immediately — low cost, high payoff.

### Worktrunk — Ergonomic git worktree CLI
- **What:** Rust CLI (`wt`) that wraps git worktrees with commands designed for parallel agent work. `wt switch -c -x claude feature-a` replaces the multi-step worktree+cd+claude dance. Includes status summaries, merge workflows, and auto-cleanup.
- **URL:** https://worktrunk.dev/
- **Why:** CCC already has worktree conventions (`.claude/worktree/`, `/close-worktree`), but Worktrunk may handle merge/cleanup better — especially the "never remove a worktree from inside it" footgun.
- **Install:** `brew install worktrunk` or `cargo install worktrunk`
- **Status:** Open source, Rust.
- **Verdict:** Evaluate next time parallel worktree work is happening. Could replace or supplement CCC's hand-rolled worktree management.

### Cmux — Terminal emulator for multi-agent sessions
- **What:** Native macOS terminal emulator (built on Ghostty) purpose-built for running multiple AI coding agents. Notification rings when an agent needs attention, socket API for automation.
- **URL:** https://cmux.com/
- **Why:** Solves "which of my 5 Claude sessions needs me?" problem.
- **Verdict:** Skip for now — CCC already provides session awareness, and adopting Cmux means replacing the terminal. Revisit if session-juggling pain increases.
