# SPEC: PR Tracking Plugin (built-in)

## Purpose

Track open pull requests across GitHub in four actionable categories. Surfaces PRs that need the user's attention (respond to reviews, review others' PRs) alongside PRs that are waiting or stale, so nothing falls through the cracks.

## Slug: `prs`

## Routes

- `waiting` — PRs authored by user, waiting on reviewers (default)
- `respond` — PRs where changes have been requested from the user
- `review` — PRs where the user has been requested to review
- `stale` — PRs with no activity for 14+ days

## File Organization

| File | Responsibility |
|------|---------------|
| `prs.go` | Main plugin struct, Init, Refresh, HandleMessage, NavigateTo, Routes |
| `keys.go` | HandleKey, KeyBindings — sub-tab switching, cursor nav, open in browser, refresh |
| `view.go` | View rendering: tab bar with counts, PR list rows, per-category detail columns, hints |
| `category.go` | Category constants, display names, empty-state messages |
| `styles.go` | Row-level styles (success/failure/pending/draft colors) |
| `messages.go` | Internal message types (prsLoadedMsg) |
| `trigger.go` | `needsAgent` pure predicate, `evaluateAgentTriggers` scan + spawn logic |
| `trigger_test.go` | Tests for agent trigger detection (needsAgent conditions) |

**Related files:**

| File | Responsibility |
|------|---------------|
| `internal/refresh/sources/github/pr_fetch.go` | Fetches PRs via `gh` CLI, computes categories |
| `internal/refresh/sources/github/github.go` | GitHub data source, orchestrates PR + notification fetch |
| `internal/db/types.go` | `PullRequest` struct — shared contract between refresh and plugin |
| `internal/db/schema.go` | `cc_pull_requests` table DDL |
| `internal/db/read.go` | `DBLoadPullRequests` — reads PRs from SQLite |
| `internal/db/write.go` | `DBSavePullRequests` — upserts PRs into SQLite |

## State

- `prs []db.PullRequest` — all PRs loaded from DB (all categories)
- `activeTab int` — 0=waiting, 1=respond, 2=review, 3=stale
- `cursors [4]int` — per-tab cursor positions (preserved when switching tabs)
- `lastLoaded time.Time` — timestamp of last successful DB load
- `width, height int` — viewport dimensions
- `frame int` — animation frame counter (for pulsing cursor)

### Per-PR Fields (on `db.PullRequest`)

| Field | Type | Purpose |
|-------|------|---------|
| `state` | string | `"open"` or `"archived"` — replaces deletion on close |
| `ignored` | boolean | Per-PR ignore flag — hidden from all tabs when set |
| `head_sha` | string | Current HEAD SHA from GitHub (`headRefOid`) |
| `agent_session_id` | string | Claude session UUID for active/last agent run |
| `agent_status` | string | `""`, `"pending"`, `"running"`, `"completed"`, `"failed"` |
| `agent_category` | string | Which category triggered the agent (`"review"` or `"respond"`) |
| `agent_head_sha` | string | HEAD SHA when the agent last ran |
| `agent_summary` | string | Extracted summary when agent completes |

## Data Flow

1. `ai-cron` runs `gh search prs --author=@me --state=open` and `gh search prs --review-requested=@me --state=open`
2. For each PR, fetches detail via `gh pr view` (reviews, reviewRequests, statusCheckRollup, comments)
3. Merges authored + review-requested lists, deduplicates by `owner/repo#number` key
4. Computes category for each PR using `computeCategory` (see Category Assignment below)
5. Writes results to `cc_pull_requests` SQLite table via `DBSavePullRequests`
6. Plugin's `Refresh()` method loads from DB via `DBLoadPullRequests` (every 30s or on `r` key)
7. `prsLoadedMsg` delivers data to `HandleMessage`, which updates state and clamps cursors
8. On `enter`, plugin resolves `owner/repo` to a local directory by scanning learned paths' `.git/config` for matching remote URLs, then launches Claude with `/pr-review-toolkit:review-pr <url>`
9. **Trigger detection**: On every PR data load, scan for PRs where `category in ("review", "respond")` AND (`head_sha != agent_head_sha` OR `category != agent_category`) AND `agent_status` not in `("running", "pending")`. Matching PRs are queued for agent spawn.
10. **Agent spawn**: For each triggered PR, resolve local repo dir, then call `agentRunner.LaunchOrQueue(request)`. Update PR row with `agent_status = "running"`, `agent_category`, `agent_head_sha = head_sha`.
11. **OnComplete**: Agent runner fires callback — update `agent_status` to `"completed"` or `"failed"`, set `agent_session_id` and `agent_summary`, insert bookmark via `DBInsertBookmark`.

## Category Assignment

Categories are assigned in priority order by `computeCategory`:

1. **review** — `my_role` is "reviewer" or "both" AND current user is in `pending_reviewer_logins`
2. **respond** — `my_role` is "author" or "both" AND `review_decision` = "CHANGES_REQUESTED"
3. **stale** — `last_activity_at` older than 14 days
4. **waiting** — default (authored PRs waiting on reviewers)

A PR gets exactly one category. The first matching rule wins.

## Key Bindings

| Key | Description | Promoted |
|-----|-------------|----------|
| 1/2/3/4 | Switch to sub-tab by number | yes |
| left/right, h/l | Cycle sub-tabs | yes |
| up/down, j/k | Navigate PR list (wraps around) | yes |
| enter | Context-aware: resume completed agent, attach to running agent, or launch manual review/respond session — falls back to browser if no local repo found | yes |
| o | Open selected PR in browser (via URL or `gh pr view --web`) | yes |
| w | Watch running agent (read-only stream viewer) | yes |
| i | Toggle ignore on selected PR. Sets `ignored=1` in DB. Flash: "PR ignored: {title}" | yes |
| I | Ignore repo of selected PR. Inserts into `cc_ignored_repos`. Flash: "{repo} ignored — all PRs hidden" | yes |
| r | Force refresh from DB | yes |

### Enter Key Behavior (context-aware)

| PR Agent State | Action |
|---|---|
| Agent completed | Resume bookmarked session (`--resume <agent_session_id>`) |
| Agent running | Attach to live session |
| Agent pending | Flash: "Agent queued, waiting for slot..." |
| Agent failed | Resume session to see what went wrong. Requires a local repo directory to be configured; silently no-ops if the directory is missing. |
| No agent (review tab) | Launch `/pr-review-toolkit:review-pr <url>` (manual) |
| No agent (respond tab) | Launch `/pr-respond <url>` (interactive, no --apply) |
| No local repo | Flash: "No local repo found — add a session path first" |

## Ignore

### Per-PR Ignore
- Toggled via `i` key on the selected PR
- Stored as `ignored` column on `cc_pull_requests` (boolean, default 0)
- Ignored PRs are filtered at the DB query level — they never appear in any tab
- Auto-cleaned when PR is archived (no stale ignore flags accumulate)

### Per-Repo Ignore
- Set via `I` key on the selected PR's repo
- Stored in `cc_ignored_repos` table (`repo TEXT PRIMARY KEY`)
- All PRs from ignored repos are filtered at the DB query level
- Managed (un-ignored) via the Settings pane

### Filtering
- Ignored items are filtered at DB query level — they never appear in any tab and never trigger agents

## Hint Bar

```
1-4 tab  j/k nav  enter review/respond  o open  w watch  i ignore  r refresh
```

## View

### Tab Bar

Rendered as `[1] Waiting (3)  [2] Respond (1)  [3] Review (2)  [4] Stale (0)` with active tab highlighted.

### PR Row

Each row shows `repo#number  Title  <contextual detail>  <agent status>` with a pulsing `>` cursor on the selected row.

#### Agent Status Indicators

| State | Indicator | Color |
|---|---|---|
| No agent | (nothing) | — |
| Pending | `⏳ queued` | pending/yellow |
| Running | `⏳ running` | pending/yellow |
| Completed | `✓ ready` | success/green |
| Failed | `✗ failed` | failure/red |
| No local repo | `⚠ no repo` | failure/red |

Contextual detail varies by category:

- **Waiting:** reviewer statuses (pending/approved indicators), CI status, age
- **Respond:** unresolved thread count, review decision badge, who requested changes
- **Review:** PR author, age, draft indicator
- **Stale:** age, draft indicator, CI status

### Empty States

Each category has a distinct empty message:

- Waiting: "No PRs waiting on reviewers -- you're all clear!"
- Respond: "No PRs need your response right now."
- Review: "No PRs waiting for your review."
- Stale: "No stale PRs -- everything is moving."

## Settings Pane

Accessible via Settings > PLUGINS > PRs:

- **Ignored Repos**: Lists all repos in `cc_ignored_repos` with un-ignore support (removes row from table)
- **Ignored PRs**: Lists all PRs with `ignored = 1` with un-ignore support (sets `ignored = 0`)

## Migrations

None — `cc_pull_requests` table is created in core `schema.go`.

## Event Handling

**Handles `plugin.NotifyMsg`:**

- `data.refreshed` — dispatches async `Refresh()` cmd to reload PR data from DB; also triggers agent evaluation (scan for PRs needing agent spawn)
- `agent.finished` — daemon-managed agent completed; parses `{"id", "exit_code"}` payload, updates PR status to `"completed"` or `"failed"` in DB and in-memory, emits `AgentStateChangedMsg`
- `agent.started` — daemon-managed agent started; parses `{"id"}` payload, updates PR status to `"running"` in DB and in-memory, emits `AgentStateChangedMsg`

## Agent Automation

### Trigger Condition

On every PR data load (after `data.refreshed` or 30s tick), scan for PRs matching ALL of:

1. `category` is `"review"` or `"respond"`
2. `head_sha != agent_head_sha` OR `category != agent_category` (or `agent_head_sha` is empty)
3. `agent_status` is not `"running"` or `"pending"`

### Lifecycle

1. PR submitted (V1) → enters "review" → `agent_head_sha` is empty → agent fires
2. Reviewer responds to V1 → enters "respond" → same SHA but `agent_category` was "review" → different category, agent fires
3. Agent commits (towards V2) → you push (V2, new SHA) → enters "respond" again → `head_sha != agent_head_sha` → new agent fires
4. Cycle repeats

### Deduplication & Safety

Three layers prevent runaway agent spawning:

1. **`needsAgent` predicate** — returns false if `agent_status` is `"running"` or `"pending"` (DB-level guard)
2. **`evaluateAgentTriggers` active-set check** — queries `agentRunner.Active()` and skips PRs with an existing active session (in-memory guard, protects against DB read lag)
3. **`LaunchOrQueue` dedup** — rejects requests whose ID is already in the active session map or the queue (runner-level guard, prevents any caller from double-launching)

### Agent Status Lifecycle (PR Plugin)

The PR plugin handles agent lifecycle messages in `HandleMessage` via two paths:

**Local runner path** (direct `agent.*Msg` types from `CheckProcesses`):

| Message | DB Update | In-Memory Update |
|---------|-----------|-----------------|
| `SessionStartedMsg` | `agent_status = "running"` | Update `p.prs` status |
| `SessionIDCapturedMsg` | `agent_session_id = <id>` | Update `p.prs` session ID |
| `SessionFinishedMsg` | `agent_status = "completed"/"failed"`, `agent_summary` | Update `p.prs` status |

**Daemon path** (`plugin.NotifyMsg` events from daemon subscription):

| NotifyMsg Event | DB Update | In-Memory Update |
|---------|-----------|-----------------|
| `agent.started` | `agent_status = "running"` | Update `p.prs` status |
| `agent.finished` | `agent_status = "completed"/"failed"` | Update `p.prs` status |

Messages are matched to PRs by checking if `msg.ID` exists in `p.prs` (PR IDs are `"owner/repo#number"`).

### Sandbox Constraints

PR agents run with restricted permissions (autonomous profile):

- **Filesystem**: Worktree only (respond) or workdir only (review) — no extra write paths
- **Network**: `autonomous_allowed_domains` from config (defaults to `github.com`, `api.github.com`)
- **MCP**: Disabled — no access to Slack, Gmail, etc.

### OnComplete Behavior

1. Update PR row: `agent_status = "completed"` (or `"failed"`), `agent_session_id`, `agent_summary`
2. Insert bookmark via `DBInsertBookmark` for session resumption
3. PR row displays `✓ ready` or `✗ failed` indicator

## Test Cases

### Category Assignment (`pr_fetch_test.go`)

- PR with `my_role=reviewer` and user in `pending_reviewer_logins` → "review"
- PR with `my_role=author` and `review_decision=CHANGES_REQUESTED` → "respond"
- PR with `last_activity_at` older than 14 days → "stale"
- PR with `my_role=author` and no special conditions → "waiting"
- Priority: a PR matching both "review" and "stale" gets "review" (first match wins)
- PR with `my_role=both` gets "review" if also in pending reviewers

### Deduplication

- Same PR appearing in both authored and review-requested results: merged with `my_role=both`, not duplicated

### DB Round-Trip (`db_test.go`)

- `DBSavePullRequests` + `DBLoadPullRequests` preserves all fields
- JSON slice fields (`reviewer_logins`, `pending_reviewer_logins`) round-trip correctly through SQLite

### CI Status Computation

- All checks passing → "success"
- Any failure/error → "failure"
- Any pending with no failures → "pending"
- No checks → empty string

### View Rendering

- Empty state renders hint message (no panic on zero PRs)
- Tab bar shows correct counts per category
- Cursor clamps to list bounds on data refresh

### Partial Fetch Failure

- Detail fetch failure for one PR: that PR still appears with empty detail fields (reviewDecision, CI, etc.)
- Search fetch failure: returns error, no PRs written to DB for that query (other query's results still saved)
