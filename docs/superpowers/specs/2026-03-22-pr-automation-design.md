# Design: PR Automation — Auto-Review & Auto-Respond Agents

## Purpose

Automate the two most common PR workflows: reviewing others' PRs and responding to feedback on your own. When a PR enters the "review" or "respond" category, CCC automatically spawns a sandboxed Claude agent to do the work. The agent streams output, bookmarks its session, and flags the PR as ready for your attention. You engage when you're ready — resuming the session to direct next steps (post comments, push changes, etc.).

## Problem

Today the PR plugin is passive — it shows PRs in categories but takes no action. Every PR review or response requires manual initiation. This means:

- Review requests pile up unnoticed
- Feedback on your PRs sits until you context-switch to address it
- The initial analysis/response work (reading diffs, understanding changes, drafting responses) is repetitive and delegatable

## Two Automation Workflows

### Review Automation (incoming review requests)

**Trigger:** PR appears in "review" category with a new HEAD SHA.

**Action:** Spawn Claude agent in the matching local repo with `/pr-review-toolkit:review-pr <url>`. Agent reads the diff, analyzes the changes, and prepares a review. Runs to completion — no human intercept before it finishes.

**On completion:** Session bookmarked, PR flagged `✓ ready`. You resume the session to see the review and direct Claude to post comments (or not).

**Trust model:** Fire-and-forget, but read-only analysis. The agent doesn't post comments or modify code autonomously.

### Respond Automation (feedback on your PRs)

**Trigger:** PR appears in "respond" category with a new HEAD SHA.

**Action:** Spawn Claude agent in a worktree, checking out the PR branch. Runs `/pr-respond --apply <url>`. Agent reads the review feedback, makes code changes, and commits locally. Does NOT push.

**On completion:** Session bookmarked, PR flagged `✓ ready`. You resume the session to review what the agent did, then push if satisfied.

**Trust model:** Autonomous and triggered by external data (PR comments from other people). Sandboxed — filesystem writes restricted to the worktree, network limited to GitHub domains, no MCP servers.

## Section 1: PR Persistence Overhaul

### Problem

PRs are currently ephemeral — `DBSavePullRequests` does `DELETE FROM cc_pull_requests` then re-inserts the fresh batch every refresh. This makes it impossible to track per-PR state like agent sessions or whether we've already acted on a PR.

### Schema Changes

The `cc_pull_requests` table keeps all existing columns and gains:

| Column | Type | Default | Purpose |
|---|---|---|---|
| `state` | TEXT | `"open"` | `"open"` or `"archived"` — replaces deletion |
| `head_sha` | TEXT | `""` | Current HEAD SHA from GitHub (`headRefOid`) |
| `agent_session_id` | TEXT | `""` | Claude session UUID for active/last agent run |
| `agent_status` | TEXT | `""` | `""`, `"pending"`, `"running"`, `"completed"`, `"failed"` |
| `agent_category` | TEXT | `""` | Which category triggered the agent (`"review"` or `"respond"`) |
| `agent_head_sha` | TEXT | `""` | HEAD SHA when the agent last ran |
| `agent_summary` | TEXT | `""` | Extracted summary when agent completes |

### Save Strategy

`DBSavePullRequests` changes from delete-all/re-insert to merge-based:

1. **Upsert** each fresh PR by ID (`owner/repo#number`):
   - Update all GitHub-sourced fields (title, review_decision, category, ci_status, head_sha, etc.)
   - **Preserve** agent columns (`agent_session_id`, `agent_status`, `agent_category`, `agent_head_sha`, `agent_summary`)
   - Set `state = "open"`
2. **Archive** PRs not in the fresh batch — set `state = "archived"` (do not delete)
3. **Reactivate** archived PRs that reappear — set `state = "open"`

### Load Changes

`DBLoadPullRequests` filters to `state = "open"` by default.

### HEAD SHA Fetch

Add `headRefOid` to the `gh pr view --json` fields list in `pr_fetch.go`. Map to `head_sha` on the `PullRequest` struct and DB column.

### Version-Based Trigger Detection

On each PR data load in the plugin, scan for PRs where:

- `category` is `"review"` or `"respond"`
- `head_sha != agent_head_sha` OR `category != agent_category` (or `agent_head_sha` is empty)
- `agent_status` is not `"running"` or `"pending"`

This correctly handles the full lifecycle:

1. PR submitted (V1) → enters "review" → `agent_head_sha` is empty → agent fires
2. Reviewer responds to V1 → enters "respond" → same SHA as review run, but `agent_category` was "review" and now category is "respond" → different category, agent fires
3. Agent commits (towards V2) → you push (V2, new SHA on GitHub) → enters "respond" again → `head_sha != agent_head_sha` → new agent fires
4. Cycle repeats

The version signal is the HEAD SHA, and the trigger also fires when the category changes (a PR moving from "review" to "respond" has new work to do even at the same SHA).

## Section 2: Shared Agent Runner

### Extraction

The agent runner currently lives inside the command center plugin. Extract it into `internal/agent/` as a shared service passed via `plugin.Context`. Both the command center (for todos) and the PR plugin (for PR agents) call into it.

### Interface

```go
type Runner interface {
    LaunchOrQueue(req Request) error
    Watch(id string) tea.Cmd          // read-only event stream
    Attach(id string) tea.Cmd         // interactive takeover
    Resume(sessionID string) LaunchAction  // resume bookmarked session
    Status(id string) *SessionStatus  // current status + summary
    Active() []Session                // all running/queued sessions
}

type Request struct {
    ID             string            // unique key (todo ID or PR ID)
    Prompt         string
    ProjectDir     string
    Worktree       bool
    AllowedDomains []string          // network sandbox whitelist
    AllowedPaths   []string          // extra filesystem write paths
    AllowMCP       bool              // whether MCP servers are available
    Permission     string            // claude permission mode
    OnComplete     func(Result)      // callback with session details
}

type Result struct {
    SessionID string
    Summary   string
    ExitCode  int
    LogPath   string
    Status    string  // "completed" or "failed"
}
```

### Core Guarantees

Every agent spawned through the runner gets:

- **Sandboxed** — always. Filesystem writes restricted to workdir (+ any `AllowedPaths`). Network restricted to `AllowedDomains`. Enforced via macOS Seatbelt / Linux bubblewrap.
- **Streamed** — always. Stream-JSON output captured, parsed into events, stored.
- **Observable** — standard viewer for any running agent. Watch (read-only tail), attach (interactive), resume (bookmarked session).
- **Bookmarked on completion** — session always persisted with summary extraction.
- **Queued** — shared concurrency pool (default 3), FIFO.

### Sandbox Configuration

```yaml
agent:
  max_concurrent: 3
  todo_write_learned_paths: true
  todo_extra_write_paths: []
  autonomous_allowed_domains:
    - github.com
    - api.github.com
```

**Todo agents (interactive profile):**

- `todo_write_learned_paths: true` → write access to all learned paths (derived automatically)
- `todo_extra_write_paths` → additional paths beyond learned (e.g., shared output directories)
- Network: `["*"]` — broad access, human-reviewed prompt
- MCP: enabled — Slack, Gmail, etc. available

**PR agents (autonomous profile):**

- Filesystem writes: worktree only (no extra paths)
- Network: `autonomous_allowed_domains` from config (defaults to GitHub)
- MCP: disabled — no access to Slack, Gmail, etc.

**Trust boundary rationale:**

- Todo prompts are human-reviewed before launch → broad access is appropriate
- PR agents are machine-triggered from external data (PR comments written by other people) → minimal blast radius required

### Settings Page

Under the existing Agent section in Settings:

- Toggle: "Allow todo agents to write to all session paths" (`todo_write_learned_paths`)
- List: "Additional write paths" (`todo_extra_write_paths`) — add/remove
- List: "Autonomous agent allowed domains" (`autonomous_allowed_domains`) — add/remove

### Setup & Defaults

No configuration required for new users:

- `todo_write_learned_paths` defaults to `true` — as users add session paths, write permissions grow automatically
- `autonomous_allowed_domains` defaults to `["github.com", "api.github.com"]`
- Doctor check validates: learned paths exist (warns if empty), autonomous profile isn't accidentally broad

## Section 3: Review Automation

### Trigger Flow

1. PR data loads (via refresh event or 30s tick)
2. Plugin scans for PRs where `category = "review"` and `head_sha != agent_head_sha` and `agent_status` not in `("running", "pending")`
3. `resolveRepoDir(pr.Repo)` maps GitHub repo to local directory
4. If no local dir → set visual flag `⚠ no repo`, skip
5. If resolved → call `agentRunner.LaunchOrQueue(request)`

### Agent Request

```go
Request{
    ID:             pr.ID,                    // "owner/repo#123"
    Prompt:         "/pr-review-toolkit:review-pr " + pr.URL,
    ProjectDir:     resolvedDir,
    Worktree:       false,                    // read-only analysis
    AllowedDomains: cfg.Agent.AutonomousAllowedDomains,
    AllowedPaths:   nil,                      // workdir only
    AllowMCP:       false,
    Permission:     "default",
    OnComplete:     p.onReviewComplete,
}
```

### On Launch

Update PR row:

- `agent_status = "running"`
- `agent_category = "review"`
- `agent_head_sha = head_sha`

### On Completion

`onReviewComplete` callback:

- `agent_status = "completed"` (or `"failed"`)
- `agent_session_id = result.SessionID`
- `agent_summary = result.Summary`
- Bookmark the session via `DBInsertBookmark`

### Enter Key (review tab)

| PR State | Action |
|---|---|
| Agent completed | Resume bookmarked session (`--resume <agent_session_id>`) |
| Agent running | Attach to live session |
| Agent pending | Flash: "Agent queued, waiting for slot..." |
| Agent failed | Resume session to see what went wrong |
| No agent | Launch `/pr-review-toolkit:review-pr <url>` (manual) |
| No local repo | Flash: "No local repo found — add a session path first" |

## Section 4: Respond Automation

### Trigger Flow

Same as review, but for `category = "respond"`.

### Agent Request

```go
Request{
    ID:             pr.ID,
    Prompt:         "/pr-respond --apply " + pr.URL,
    ProjectDir:     resolvedDir,
    Worktree:       true,                     // isolated worktree
    AllowedDomains: cfg.Agent.AutonomousAllowedDomains,
    AllowedPaths:   nil,                      // worktree only
    AllowMCP:       false,
    Permission:     "default",
    OnComplete:     p.onRespondComplete,
}
```

### Pre-Launch

The agent runner handles worktree setup (existing `worktree.PrepareWorktree`). The agent's prompt includes checking out the correct branch — the `/pr-respond --apply` command handles `git fetch` and branch checkout as part of its workflow.

### On Completion

Same pattern as review — update DB, bookmark session. The agent has committed changes locally in the worktree but has NOT pushed. The user resumes the session to review and push.

### Enter Key (respond tab)

| PR State | Action |
|---|---|
| Agent completed | Resume bookmarked session (in the worktree) |
| Agent running | Attach to live session |
| Agent pending | Flash: "Agent queued, waiting for slot..." |
| Agent failed | Resume session to see what went wrong |
| No agent | Launch `/pr-respond <url>` (interactive, no --apply) |
| No local repo | Flash: "No local repo found — add a session path first" |

## Section 5: View & UX

### PR Row Visual Flags

Each PR row gets an agent status indicator appended after the contextual detail columns:

| State | Indicator | Color |
|---|---|---|
| No agent | (nothing) | — |
| Pending | `⏳ queued` | pending/yellow |
| Running | `⏳ running` | pending/yellow |
| Completed | `✓ ready` | success/green |
| Failed | `✗ failed` | failure/red |
| No local repo | `⚠ no repo` | muted |

The `✓ ready` flag is the primary "needs your attention" signal.

### Key Bindings Update

| Key | Description | Promoted |
|---|---|---|
| 1/2/3/4 | Switch sub-tab | yes |
| left/right, h/l | Cycle sub-tabs | yes |
| j/k | Navigate PR list | yes |
| enter | Review/respond (resume agent or launch manual) | yes |
| o | Open PR in browser | yes |
| w | Watch running agent (read-only stream viewer) | yes |
| r | Refresh | yes |

### Hint Bar

```
1-4 tab  j/k nav  enter review/respond  o open  w watch  r refresh
```

### Agent Spawn Evaluation Timing

Agent spawn checks run on every PR data load:

- After `data.refreshed` event (ai-cron completed)
- On 30-second tick (periodic re-evaluation)

This means a newly resolvable PR (e.g., after adding a learned path) triggers within 30 seconds, not 5 minutes.

## Data Flow Summary

```
ai-cron                     DB                      PR Plugin              Agent Runner
  |                          |                          |                       |
  |-- fetch PRs from GH --->|                          |                       |
  |-- upsert (merge) ------>|                          |                       |
  |-- archive missing ----->|                          |                       |
  |-- publish data.refreshed                           |                       |
  |                          |<--- load open PRs ------|                       |
  |                          |                          |-- scan for triggers   |
  |                          |                          |-- resolve repo dir    |
  |                          |                          |-- LaunchOrQueue ----->|
  |                          |<--- update agent cols ---|                       |
  |                          |                          |                       |-- spawn claude
  |                          |                          |                       |-- stream events
  |                          |                          |<-- OnComplete --------|
  |                          |<--- update status/summary|                       |
  |                          |<--- insert bookmark -----|                       |
  |                          |                          |                       |
  |                          |                  [user hits enter]               |
  |                          |                          |-- Resume(sessionID)-->|
```

## Test Cases

### PR Persistence

- Upsert preserves agent columns when GitHub fields update
- PRs missing from fresh batch get `state = "archived"`, not deleted
- Archived PRs reappearing get `state = "open"`
- `DBLoadPullRequests` returns only `state = "open"` PRs
- `head_sha` populated from `headRefOid` in GitHub response

### Trigger Detection

- PR in "review" with no `agent_head_sha` → triggers
- PR in "review" with `agent_head_sha == head_sha` AND `agent_category == "review"` → skips
- PR in "review" with `agent_head_sha != head_sha` → triggers (new code)
- PR moving from "review" to "respond" at same SHA → triggers (different category)
- PR with `agent_status = "running"` → skips (already active)
- PR with no local repo → skips, shows `⚠ no repo`

### Agent Runner

- Sandbox always enabled — no agent runs unsandboxed
- Todo agents get learned paths as write paths when `todo_write_learned_paths` is true
- PR agents get worktree-only filesystem access
- PR agents get only configured autonomous domains for network
- PR agents have `allow_mcp: false`
- Concurrency limit respected — excess agents queued
- OnComplete callback fires with correct session ID and summary
- Session bookmarked on completion
- Summary extraction follows existing pattern (result event > last assistant turn > fallback)

### View & Interaction

- PR rows show correct status indicator per agent state
- Enter on completed agent resumes session
- Enter on running agent attaches
- Enter with no agent launches manual session (review vs respond prompt varies)
- `w` on running agent opens stream viewer
- `⚠ no repo` shown when `resolveRepoDir` returns empty

### Respond-Specific

- Agent runs in worktree (isolated from main checkout)
- Agent does NOT push (commits are local)
- Worktree path stored for accurate session resumption
