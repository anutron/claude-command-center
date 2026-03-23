# PR Ignore & Settings Design

## Purpose

Add the ability to ignore individual PRs (`i`) and entire repos (`I`) from the PR plugin view, with a settings pane to manage ignored items.

## Storage

All ignore state lives in the database.

### `cc_pull_requests` — Per-PR Ignore

Add `ignored` column:

```sql
ALTER TABLE cc_pull_requests ADD COLUMN ignored BOOLEAN NOT NULL DEFAULT 0
```

- Set to 1 by `i` hotkey on a specific PR
- Togglable — pressing `i` again restores the PR
- Auto-cleaned when the PR is archived (already hidden from view)

### `cc_ignored_repos` — Repo-Level Ignore

New table:

```sql
CREATE TABLE IF NOT EXISTS cc_ignored_repos (
    repo TEXT PRIMARY KEY
)
```

- Inserted by `I` hotkey using the selected PR's repo
- Persists until manually removed from the settings pane
- No auto-cleanup — repo ignores are durable preferences

## Hotkeys

### `i` — Ignore/Restore PR

- Toggles `ignored` flag on the selected PR in `cc_pull_requests`
- Flash message: "PR ignored" / "PR restored"
- PR disappears from (or reappears in) the current tab immediately

### `I` (Shift-i) — Ignore Repo

- Inserts `pr.Repo` into `cc_ignored_repos`
- Flash message: "{repo} ignored — all PRs hidden"
- All PRs from that repo disappear from view immediately
- Re-filters the loaded PR list in memory (no re-fetch needed)

## Filtering

`DBLoadPullRequests` query updated:

```sql
WHERE state != 'archived'
  AND ignored = 0
  AND repo NOT IN (SELECT repo FROM cc_ignored_repos)
```

This means:
- Ignored PRs never load into the plugin
- PRs from ignored repos never load into the plugin
- Agent triggers never fire for ignored PRs (they aren't in the loaded list)

## Settings Pane

New content pane for the "PRs" plugin entry in the PLUGINS section of Settings. When the user selects PRs and presses enter, they see:

### Ignored Repos

List of repos from `cc_ignored_repos`:

```
IGNORED REPOS
  thanx/thanx-cortex          [x remove]
  thanx/thanx-snowflake       [x remove]
```

Selecting a repo and pressing enter (or `x`) removes it from `cc_ignored_repos`, restoring all PRs from that repo on next load.

### Ignored PRs

List of currently ignored open PRs (from `cc_pull_requests WHERE ignored = 1 AND state != 'archived'`):

```
IGNORED PRS
  thanx/thanx-dbt#774  Apply masking policies...    [x restore]
```

Selecting a PR and pressing enter (or `x`) sets `ignored = 0`, restoring it.

## DB Functions

### New

- `DBAddIgnoredRepo(db, repo string) error` — INSERT OR IGNORE into `cc_ignored_repos`
- `DBRemoveIgnoredRepo(db, repo string) error` — DELETE from `cc_ignored_repos`
- `DBLoadIgnoredRepos(db) ([]string, error)` — SELECT all ignored repos
- `DBSetPRIgnored(db, id string, ignored bool) error` — UPDATE `cc_pull_requests` SET `ignored = ?` WHERE `id = ?`
- `DBLoadIgnoredPRs(db) ([]db.PullRequest, error)` — SELECT ignored open PRs (for settings pane)

### Modified

- `DBLoadPullRequests` — add `AND ignored = 0 AND repo NOT IN (SELECT repo FROM cc_ignored_repos)` to WHERE clause

## Schema Migration

In `migrateSchema`:

```go
_, _ = db.Exec(`ALTER TABLE cc_pull_requests ADD COLUMN ignored BOOLEAN NOT NULL DEFAULT 0`)
_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS cc_ignored_repos (repo TEXT PRIMARY KEY)`)
```

## Key Bindings Update

```
{Key: "i", Description: "Ignore PR"},
{Key: "I", Description: "Ignore repo"},
```

Hint bar: `1-4 tab  j/k nav  enter review/respond  o open  w watch  i ignore  r refresh`

## Test Cases

### DB Tests
- `TestDBSetPRIgnored` — toggle ignored flag, verify round-trip
- `TestDBLoadPullRequests_FiltersIgnored` — ignored PRs excluded from load
- `TestDBLoadPullRequests_FiltersIgnoredRepos` — PRs from ignored repos excluded
- `TestDBIgnoredRepos_AddRemoveLoad` — CRUD for `cc_ignored_repos`

### PR Plugin Tests
- `TestNeedsAgent_SkipsIgnored` — already handled (ignored PRs not in loaded list)

### Settings Tests
- Nav item count updated to reflect new PR settings pane
