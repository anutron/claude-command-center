# PR Ignore & Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `i`/`I` hotkeys to ignore PRs and repos from the PR plugin, with a settings pane to manage ignored items.

**Architecture:** New `cc_ignored_repos` table + `ignored` column on `cc_pull_requests`. PR plugin gets `i`/`I` hotkeys that write to DB. Settings pane under the PRs plugin entry shows ignored repos/PRs with un-ignore support. `DBLoadPullRequests` filters out ignored items at the query level.

**Tech Stack:** Go, SQLite, bubbletea, huh (forms)

**Spec:** `docs/superpowers/specs/2026-03-22-pr-ignore-and-settings-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/builtin/settings/content_prs.go` | PR plugin settings pane (ignored repos + ignored PRs lists) |

### Modified Files

| File | Change |
|------|--------|
| `internal/db/schema.go:250-258` | Add `ignored` column + `cc_ignored_repos` table |
| `internal/db/write.go` | Add `DBSetPRIgnored`, `DBAddIgnoredRepo`, `DBRemoveIgnoredRepo` |
| `internal/db/read.go:249-257` | Filter ignored PRs and repos in `DBLoadPullRequests`; add `DBLoadIgnoredRepos`, `DBLoadIgnoredPRs` |
| `internal/db/write_pr_test.go` | Add ignore toggle test |
| `internal/db/read_pr_test.go` | Add ignore filter tests |
| `internal/builtin/prs/keys.go` | Add `i` and `I` hotkey cases |
| `internal/builtin/prs/prs.go` | Add `flashMessage`/`flashMessageAt` fields |
| `internal/builtin/prs/view.go` | Render flash message in hints area |
| `internal/builtin/settings/nav.go:66-71` | Add "prs" to `pluginDescriptions` |
| `internal/builtin/settings/settings.go:938-979` | Route `prs` slug to `buildPRSettingsForm` |
| `specs/builtin/prs.md` | Document `i`/`I` keys, ignored state, settings pane |
| `specs/core/db.md` | Document `cc_ignored_repos` table and `ignored` column |

---

## Task 0: Update Specs

**Files:**
- Modify: `specs/builtin/prs.md`
- Modify: `specs/core/db.md`

- [ ] **Step 1: Update `specs/builtin/prs.md`**

Add to **State** section:
- `ignored` boolean field on PullRequest (per-PR ignore flag)

Add to **Key Bindings** section:
- `i` — Toggle ignore on selected PR. Sets `ignored=1` in DB. Flash: "PR ignored" / "PR restored"
- `I` — Ignore repo of selected PR. Inserts into `cc_ignored_repos`. Flash: "{repo} ignored — all PRs hidden"

Add new **Ignore** section:
- Per-PR ignore: toggled via `i`, stored as `ignored` column, auto-cleaned when PR archived
- Per-repo ignore: set via `I`, stored in `cc_ignored_repos`, managed in settings pane
- Ignored items filtered at DB query level — never appear in any tab, never trigger agents

Update **Hint Bar**: `1-4 tab  j/k nav  enter review/respond  o open  w watch  i ignore  r refresh`

Add **Settings Pane** section:
- Accessible via Settings > PLUGINS > PRs
- Shows ignored repos list with un-ignore support
- Shows ignored PRs list with un-ignore support

- [ ] **Step 2: Update `specs/core/db.md`**

Document:
- New column: `cc_pull_requests.ignored BOOLEAN NOT NULL DEFAULT 0`
- New table: `cc_ignored_repos (repo TEXT PRIMARY KEY)`
- New functions: `DBSetPRIgnored`, `DBAddIgnoredRepo`, `DBRemoveIgnoredRepo`, `DBLoadIgnoredRepos`, `DBLoadIgnoredPRs`
- Modified: `DBLoadPullRequests` filters `ignored = 0` and `repo NOT IN (SELECT repo FROM cc_ignored_repos)`

- [ ] **Step 3: Commit**

```bash
git add specs/
git commit -m "Update specs for PR ignore hotkeys and settings pane"
```

---

## Task 1: Schema Migration — Add Ignored Column and Table

**Files:**
- Modify: `internal/db/schema.go:250-258`

- [ ] **Step 1: Add migrations to `migrateSchema`**

After the existing PR automation columns block (line ~257), add:

```go
// PR ignore support
_, _ = db.Exec(`ALTER TABLE cc_pull_requests ADD COLUMN ignored BOOLEAN NOT NULL DEFAULT 0`)
_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS cc_ignored_repos (repo TEXT PRIMARY KEY)`)
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Run: `go test ./internal/db/ -run TestOpenDB -v`

- [ ] **Step 3: Commit**

```bash
git add internal/db/schema.go
git commit -m "Add ignored column and cc_ignored_repos table to schema"
```

---

## Task 2: DB Write Functions — Ignore Operations

**Files:**
- Modify: `internal/db/write.go`
- Modify: `internal/db/write_pr_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/db/write_pr_test.go`:

```go
func TestDBSetPRIgnored(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "r#1", Repo: "r", Number: 1, Title: "T", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()

	// Ignore
	if err := DBSetPRIgnored(d, "r#1", true); err != nil {
		t.Fatal(err)
	}
	var ignored bool
	d.QueryRow(`SELECT ignored FROM cc_pull_requests WHERE id='r#1'`).Scan(&ignored)
	if !ignored {
		t.Error("expected ignored=true")
	}

	// Restore
	if err := DBSetPRIgnored(d, "r#1", false); err != nil {
		t.Fatal(err)
	}
	d.QueryRow(`SELECT ignored FROM cc_pull_requests WHERE id='r#1'`).Scan(&ignored)
	if ignored {
		t.Error("expected ignored=false")
	}
}

func TestDBIgnoredRepos_AddRemoveLoad(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	// Add
	if err := DBAddIgnoredRepo(d, "org/repo-a"); err != nil {
		t.Fatal(err)
	}
	if err := DBAddIgnoredRepo(d, "org/repo-b"); err != nil {
		t.Fatal(err)
	}
	// Duplicate is no-op
	if err := DBAddIgnoredRepo(d, "org/repo-a"); err != nil {
		t.Fatal(err)
	}

	repos, err := DBLoadIgnoredRepos(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(repos))
	}

	// Remove
	if err := DBRemoveIgnoredRepo(d, "org/repo-a"); err != nil {
		t.Fatal(err)
	}
	repos, _ = DBLoadIgnoredRepos(d)
	if len(repos) != 1 || repos[0] != "org/repo-b" {
		t.Errorf("expected [org/repo-b], got %v", repos)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/ -run "TestDBSetPRIgnored|TestDBIgnoredRepos" -v`
Expected: FAIL (functions not defined)

- [ ] **Step 3: Implement**

Add to `internal/db/write.go`:

```go
// DBSetPRIgnored sets or clears the ignored flag on a pull request.
func DBSetPRIgnored(d *sql.DB, id string, ignored bool) error {
	_, err := d.Exec(`UPDATE cc_pull_requests SET ignored = ? WHERE id = ?`, ignored, id)
	return err
}

// DBAddIgnoredRepo adds a repo to the ignore list.
func DBAddIgnoredRepo(d *sql.DB, repo string) error {
	_, err := d.Exec(`INSERT OR IGNORE INTO cc_ignored_repos (repo) VALUES (?)`, repo)
	return err
}

// DBRemoveIgnoredRepo removes a repo from the ignore list.
func DBRemoveIgnoredRepo(d *sql.DB, repo string) error {
	_, err := d.Exec(`DELETE FROM cc_ignored_repos WHERE repo = ?`, repo)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/db/ -run "TestDBSetPRIgnored|TestDBIgnoredRepos" -v`

- [ ] **Step 5: Commit**

```bash
git add internal/db/write.go internal/db/write_pr_test.go
git commit -m "Add DB functions for PR and repo ignore operations"
```

---

## Task 3: DB Read Functions — Filter Ignored, Load Ignored Lists

**Files:**
- Modify: `internal/db/read.go:249-257`
- Modify: `internal/db/read_pr_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/db/read_pr_test.go`:

```go
func TestDBLoadPullRequests_FiltersIgnoredPRs(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "r#1", Repo: "r", Number: 1, Title: "Visible", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
		{ID: "r#2", Repo: "r", Number: 2, Title: "Ignored", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()

	DBSetPRIgnored(d, "r#2", true)

	prs, err := DBLoadPullRequests(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].ID != "r#1" {
		t.Errorf("expected 1 visible PR, got %d", len(prs))
	}
}

func TestDBLoadPullRequests_FiltersIgnoredRepos(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "good/repo#1", Repo: "good/repo", Number: 1, Title: "Visible", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
		{ID: "bad/repo#1", Repo: "bad/repo", Number: 1, Title: "Hidden", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()

	DBAddIgnoredRepo(d, "bad/repo")

	prs, err := DBLoadPullRequests(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].Repo != "good/repo" {
		t.Errorf("expected 1 PR from good/repo, got %d", len(prs))
	}
}

func TestDBLoadIgnoredPRs(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	now := time.Now()
	tx, _ := d.Begin()
	DBSavePullRequests(tx, []PullRequest{
		{ID: "r#1", Repo: "r", Number: 1, Title: "Ignored", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
		{ID: "r#2", Repo: "r", Number: 2, Title: "Not Ignored", URL: "u", Author: "a",
			CreatedAt: now, UpdatedAt: now, LastActivityAt: now, FetchedAt: now},
	})
	tx.Commit()

	DBSetPRIgnored(d, "r#1", true)

	prs, err := DBLoadIgnoredPRs(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].ID != "r#1" {
		t.Errorf("expected 1 ignored PR, got %d", len(prs))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/ -run "TestDBLoadPullRequests_FiltersIgnored|TestDBLoadIgnoredPRs" -v`

- [ ] **Step 3: Update `DBLoadPullRequests` and add load functions**

In `internal/db/read.go`, update the WHERE clause at line ~257:

```sql
FROM cc_pull_requests
WHERE state != 'archived'
  AND ignored = 0
  AND repo NOT IN (SELECT repo FROM cc_ignored_repos)
ORDER BY last_activity_at DESC
```

Add two new functions:

```go
// DBLoadIgnoredRepos returns all repos in the ignore list.
func DBLoadIgnoredRepos(d *sql.DB) ([]string, error) {
	rows, err := d.Query(`SELECT repo FROM cc_ignored_repos ORDER BY repo`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []string
	for rows.Next() {
		var repo string
		if err := rows.Scan(&repo); err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// DBLoadIgnoredPRs returns all ignored but non-archived PRs (for settings pane).
func DBLoadIgnoredPRs(d *sql.DB) ([]PullRequest, error) {
	rows, err := d.Query(`SELECT id, repo, number, title, url, author
		FROM cc_pull_requests WHERE ignored = 1 AND state != 'archived'
		ORDER BY last_activity_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prs []PullRequest
	for rows.Next() {
		var pr PullRequest
		if err := rows.Scan(&pr.ID, &pr.Repo, &pr.Number, &pr.Title, &pr.URL, &pr.Author); err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}
	return prs, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/db/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/db/read.go internal/db/read_pr_test.go
git commit -m "Filter ignored PRs and repos from load, add ignored list queries"
```

---

## Task 4: PR Plugin — Flash Message Support

**Files:**
- Modify: `internal/builtin/prs/prs.go:26-43`
- Modify: `internal/builtin/prs/view.go:37-39`

- [ ] **Step 1: Add flash message fields to Plugin struct**

In `internal/builtin/prs/prs.go`, add to the Plugin struct (after `frame int`):

```go
flashMessage   string
flashMessageAt time.Time
```

- [ ] **Step 2: Render flash message in view**

In `internal/builtin/prs/view.go`, in the `View` method (line ~39), update the hints rendering to include flash message:

```go
hints := p.renderHints()

// Clear stale flash messages
if p.flashMessage != "" && time.Since(p.flashMessageAt) > 5*time.Second {
	p.flashMessage = ""
}
if p.flashMessage != "" {
	flash := p.styles.Hint.Render("  > " + p.flashMessage)
	hints = flash + "\n" + hints
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`

- [ ] **Step 4: Commit**

```bash
git add internal/builtin/prs/prs.go internal/builtin/prs/view.go
git commit -m "Add flash message support to PR plugin"
```

---

## Task 5: PR Plugin — `i` and `I` Hotkeys

**Files:**
- Modify: `internal/builtin/prs/keys.go:140-159`

- [ ] **Step 1: Add `i` case (ignore/restore PR)**

In `keys.go`, before the `case "r":` block (line ~141), add:

```go
// Ignore/restore individual PR
case "i":
	filtered := p.filteredPRs(p.activeTab)
	if len(filtered) == 0 {
		return plugin.ConsumedAction()
	}
	pr := filtered[p.cursors[p.activeTab]]
	if err := db.DBSetPRIgnored(p.database, pr.ID, true); err != nil {
		p.flashMessage = "Error: " + err.Error()
		p.flashMessageAt = time.Now()
		return plugin.ConsumedAction()
	}
	p.flashMessage = fmt.Sprintf("PR ignored: %s", pr.Title)
	p.flashMessageAt = time.Now()
	// Reload to remove from view
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.Refresh()}

// Ignore entire repo
case "I":
	filtered := p.filteredPRs(p.activeTab)
	if len(filtered) == 0 {
		return plugin.ConsumedAction()
	}
	pr := filtered[p.cursors[p.activeTab]]
	if err := db.DBAddIgnoredRepo(p.database, pr.Repo); err != nil {
		p.flashMessage = "Error: " + err.Error()
		p.flashMessageAt = time.Now()
		return plugin.ConsumedAction()
	}
	p.flashMessage = fmt.Sprintf("%s ignored — all PRs hidden", pr.Repo)
	p.flashMessageAt = time.Now()
	// Reload to remove all repo PRs from view
	return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.Refresh()}
```

Add `"time"` to the imports if not already present.

- [ ] **Step 2: Update KeyBindings and hint bar**

In `KeyBindings()` (line ~149), add:

```go
{Key: "i", Description: "Ignore PR", Promoted: true},
{Key: "I", Description: "Ignore repo", Promoted: true},
```

In `view.go`, update `renderHints` to include `i ignore`:

```go
hints := p.styles.Hint.Render("1-4 tab  j/k nav  enter review/respond  o open  w watch  i ignore  r refresh")
```

(Find the existing hint string and replace it.)

- [ ] **Step 3: Verify build**

Run: `go build ./...`

- [ ] **Step 4: Commit**

```bash
git add internal/builtin/prs/keys.go internal/builtin/prs/view.go
git commit -m "Add i/I hotkeys for ignoring PRs and repos"
```

---

## Task 6: Settings Pane — PR Plugin Settings

**Files:**
- Create: `internal/builtin/settings/content_prs.go`
- Modify: `internal/builtin/settings/settings.go:938-979`

- [ ] **Step 1: Create `content_prs.go`**

Create `internal/builtin/settings/content_prs.go`:

```go
package settings

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/huh"
)

// buildPRSettingsForm creates a form showing ignored repos and ignored PRs.
func (p *Plugin) buildPRSettingsForm() *huh.Form {
	// Load ignored repos
	repos, _ := db.DBLoadIgnoredRepos(p.database)
	var repoLines string
	if len(repos) == 0 {
		repoLines = p.styles.muted.Render("No repos ignored. Press I on a PR to ignore its repo.")
	} else {
		var lines []string
		for _, r := range repos {
			lines = append(lines, fmt.Sprintf("  • %s", r))
		}
		repoLines = strings.Join(lines, "\n")
	}

	// Load ignored PRs
	ignoredPRs, _ := db.DBLoadIgnoredPRs(p.database)
	var prLines string
	if len(ignoredPRs) == 0 {
		prLines = p.styles.muted.Render("No PRs ignored. Press i on a PR to ignore it.")
	} else {
		var lines []string
		for _, pr := range ignoredPRs {
			lines = append(lines, fmt.Sprintf("  • %s  %s", pr.ID, p.styles.muted.Render(pr.Title)))
		}
		prLines = strings.Join(lines, "\n")
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Ignored Repos").
				Description(fmt.Sprintf(
					"%s\n%s",
					p.styles.muted.Render("PRs from these repos are hidden from all tabs:"),
					repoLines,
				)),
			huh.NewNote().
				Title("Ignored PRs").
				Description(fmt.Sprintf(
					"%s\n%s",
					p.styles.muted.Render("These individual PRs are hidden (auto-cleaned when closed):"),
					prLines,
				)),
		),
	).WithShowHelp(false).WithShowErrors(false).WithTheme(p.styles.huhTheme)

	return form
}
```

- [ ] **Step 2: Route `prs` slug to the new form**

In `internal/builtin/settings/settings.go`, in `buildFormForSlug` (line ~966), update the `"plugin"` case:

```go
case "plugin":
	if item.Slug == "prs" {
		form := p.buildPRSettingsForm()
		return form, form.Init()
	}
	form := p.buildPluginForm(item)
	if form != nil {
		return form, form.Init()
	}
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`

- [ ] **Step 4: Commit**

```bash
git add internal/builtin/settings/content_prs.go internal/builtin/settings/settings.go
git commit -m "Add PR plugin settings pane with ignored repos and PRs lists"
```

---

## Task 7: Final Verification

- [ ] **Step 1: Run full test suite**

Run: `make test`

- [ ] **Step 2: Build**

Run: `make build`

- [ ] **Step 3: Fix any issues**

If tests fail or build breaks, fix and re-run.

- [ ] **Step 4: Verify specs match implementation**

Read `specs/builtin/prs.md` and `specs/core/db.md` — confirm they accurately describe what was built.

- [ ] **Step 5: Commit any corrections**

```bash
git add specs/
git commit -m "Align specs with PR ignore implementation"
```

(Skip if no changes needed.)

---

## Verification

After all tasks complete:

1. `make build` — compiles cleanly
2. `make test` — all tests pass
3. Manual smoke test:
   - Start CCC, navigate to PRs tab
   - Press `i` on a PR — verify it disappears, flash shows "PR ignored: ..."
   - Press `I` on a PR — verify all PRs from that repo disappear, flash shows "{repo} ignored — all PRs hidden"
   - Navigate to Settings > PRs — verify ignored repos and PRs lists show
   - Restart CCC — verify ignored items remain hidden
   - Check DB: `sqlite3 ~/.config/ccc/data/ccc.db "SELECT repo FROM cc_ignored_repos"`
