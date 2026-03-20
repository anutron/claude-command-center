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

## Data Flow

1. `ccc-refresh` runs `gh search prs --author=@me --state=open` and `gh search prs --review-requested=@me --state=open`
2. For each PR, fetches detail via `gh pr view` (reviews, reviewRequests, statusCheckRollup, comments)
3. Merges authored + review-requested lists, deduplicates by `owner/repo#number` key
4. Computes category for each PR using `computeCategory` (see Category Assignment below)
5. Writes results to `cc_pull_requests` SQLite table via `DBSavePullRequests`
6. Plugin's `Refresh()` method loads from DB via `DBLoadPullRequests` (every 30s or on `r` key)
7. `prsLoadedMsg` delivers data to `HandleMessage`, which updates state and clamps cursors

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
| enter/o | Open selected PR in browser (via URL or `gh pr view --web`) | yes |
| r | Force refresh from DB | yes |

## Hint Bar

```
1-4 switch tab   <-/-> cycle   j/k navigate   enter/o open   r refresh
```

## View

### Tab Bar

Rendered as `[1] Waiting (3)  [2] Respond (1)  [3] Review (2)  [4] Stale (0)` with active tab highlighted.

### PR Row

Each row shows `repo#number  Title  <contextual detail>` with a pulsing `>` cursor on the selected row.

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

## Migrations

None — `cc_pull_requests` table is created in core `schema.go`.

## Event Bus

None — the PR plugin does not publish or subscribe to events.

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
