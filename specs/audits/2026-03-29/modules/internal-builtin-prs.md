# Spec Audit: `internal/builtin/prs/`

**Date:** 2026-03-29
**Spec:** `specs/builtin/prs.md`
**Code files:** `prs.go`, `keys.go`, `messages.go`, `category.go`, `trigger.go`, `view.go`, `styles.go`

## Summary

- **Behavioral branches analyzed:** 52
- **Covered by spec:** 42
- **Uncovered (behavioral):** 5
- **Uncovered (implementation-only):** 3
- **Contradictions:** 2

---

## Exported Functions / Methods

### `Plugin.Slug()` — returns `"prs"`

- **[COVERED]** — Spec: `## Slug: "prs"`

### `Plugin.TabName()` — returns `"PRs"`

- **[UNCOVERED-IMPLEMENTATION]** — Display label, no behavioral significance.

### `Plugin.RefreshInterval()` — returns 30s

- **[COVERED]** — Spec Data Flow #6: "every 30s or on `r` key"

### `Plugin.Init(ctx)`

- **[COVERED]** — Stores DB, config, logger, bus, agentRunner. Spec State section lists these fields.
- **[UNCOVERED-IMPLEMENTATION]** — Style/gradient fallback to palette when `ctx.Styles == nil`. Internal rendering concern.

### `Plugin.Shutdown()` — no-op

- **[COVERED]** — Spec doesn't claim any cleanup; no-op is consistent.

### `Plugin.SetDaemonClientFunc(fn)`

- **[COVERED]** — Spec Agent Automation: "Prefers the daemon RPC for agent operations; falls back to the local agentRunner"

### `Plugin.Migrations()` — returns nil

- **[COVERED]** — Spec Migrations: "None — `cc_pull_requests` table is created in core `schema.go`."

### `Plugin.Routes()`

- **[COVERED]** — Spec Routes section: waiting, respond, review, stale with matching descriptions.

### `Plugin.NavigateTo(route, args)`

- **[COVERED]** — Spec Routes: `waiting` (default), `respond`, `review`, `stale`. Code maps each to tab index 0-3.
- **[UNCOVERED-BEHAVIORAL]** — Unknown route string: code silently ignores (no default case). Spec doesn't address invalid route handling. **Q: Should an invalid route log a warning or navigate to the default tab?**

### `Plugin.Refresh()`

- **[COVERED]** — Spec Data Flow #6: "Plugin's `Refresh()` method loads from DB via `DBLoadPullRequests`"
- **Path: database is nil** — returns nil cmd. **[COVERED]** implicitly (no DB = no load).
- **Path: DBLoadPullRequests errors** — error is silently discarded (`prs, _ := ...`). **[UNCOVERED-BEHAVIORAL]** — **Q: Should a DB load failure surface an error flash or log entry? Currently silently returns empty data.**

### `Plugin.HandleMessage(msg)`

#### `prsLoadedMsg`

- **[COVERED]** — Spec Data Flow #7: "updates state and clamps cursors"
- **Cursor clamping logic** — **[COVERED]** — Spec Test Cases > View Rendering: "Cursor clamps to list bounds on data refresh"
- **`evaluateAgentTriggers()` call** — **[COVERED]** — Spec Event Handling: "also triggers agent evaluation"

#### `plugin.NotifyMsg` with `data.refreshed`

- **[COVERED]** — Spec Event Handling: "`data.refreshed` — dispatches async `Refresh()` cmd"

#### `agent.SessionStartedMsg`

- **[COVERED]** — Spec Agent Status Lifecycle table: `SessionStartedMsg` → `agent_status = "running"`

#### `agent.SessionIDCapturedMsg`

- **[COVERED]** — Spec Agent Status Lifecycle table: `SessionIDCapturedMsg` → `agent_session_id = <id>`

#### `agent.SessionFinishedMsg`

- **Path: exit code 0** — status = "completed". **[COVERED]** — Spec table row.
- **Path: exit code != 0** — status = "failed". **[COVERED]** — Spec table row.
- **Summary extraction** — **[COVERED]** — Spec OnComplete: "set `agent_summary`"

#### `ui.TickMsg`

- **[COVERED]** — Spec Data Flow #6: "every 30s" auto-refresh.

#### `tea.WindowSizeMsg`

- **[UNCOVERED-IMPLEMENTATION]** — Internal viewport tracking.

### `Plugin.HandleKey(msg)`

#### Sub-tab switching: `1`, `2`, `3`, `4`

- **[COVERED]** — Spec Key Bindings: "1/2/3/4 — Switch to sub-tab by number"

#### Cycling: `right`/`l`, `left`/`h`

- **[COVERED]** — Spec Key Bindings: "left/right, h/l — Cycle sub-tabs"

#### Cursor movement: `down`/`j`, `up`/`k`

- **[COVERED]** — Spec Key Bindings: "up/down, j/k — Navigate PR list (wraps around)"
- **Wrap-around behavior** — Code wraps cursor from bottom to top and top to bottom. **[COVERED]** — "(wraps around)"

#### Open in browser: `o`

- **Path: PR has URL** — returns `ActionOpenURL`. **[COVERED]** — Spec: "Open selected PR in browser (via URL or `gh pr view --web`)"
- **Path: PR has no URL** — falls back to `gh pr view --web`. **[COVERED]** — same cite.
- **Path: empty list** — returns consumed (no-op). **[COVERED]** implicitly.

#### Enter key: context-aware action

- **Path: agent completed/failed + has session ID** — resume via `ActionLaunch` with `resume_id`. **[COVERED]** — Spec Enter Key table: "Resume bookmarked session"
- **Path: agent completed/failed + no session ID** — falls through to manual launch. **[UNCOVERED-BEHAVIORAL]** — Spec says "Resume bookmarked session (`--resume <agent_session_id>`)" but doesn't address the case where `agent_session_id` is empty on a completed/failed agent. **Q: Should this be an explicit error state or flash message?**
- **Path: agent completed/failed + session ID but no local repo** — returns consumed (no-op). **[CONTRADICTS]** — Spec Enter Key table says "Agent failed: Resume session to see what went wrong" but code requires a local repo dir to resume, silently doing nothing if missing.
- **Path: agent running** — navigates to command center. **[CONTRADICTS]** — Spec says "Attach to live session" but code navigates to `"command"` tab. These are different behaviors — "attach" implies streaming output, navigation just switches tabs.
- **Path: agent pending** — returns consumed (no-op). **[COVERED]** partially — Spec says "Flash: 'Agent queued, waiting for slot...'" but code does NOT show a flash message, just silently consumes the key.
- **Path: no agent, review tab** — launches `/pr-review-toolkit:review-pr`. **[COVERED]** — Spec Enter Key table.
- **Path: no agent, respond tab** — launches `/pr-respond <url>`. **[COVERED]** — Spec Enter Key table.
- **Path: no local repo, has URL** — opens in browser. **[COVERED]** partially — Spec says "Flash: 'No local repo found — add a session path first'" but code opens browser instead (no flash).
- **Path: no local repo, no URL** — returns consumed. Reasonable fallback.

#### Watch agent: `w`

- **Path: agent running** — returns watch cmd. **[COVERED]** — Spec: "Watch running agent"
- **Path: agent not running** — returns consumed. **[COVERED]** implicitly.

#### Ignore PR: `i`

- **[COVERED]** — Spec Ignore section: "Toggled via `i` key", "Sets `ignored=1` in DB", flash message.
- **Error path** — shows error flash. **[COVERED]** implicitly.

#### Ignore repo: `I`

- **[COVERED]** — Spec Ignore: "Set via `I` key", "Stored in `cc_ignored_repos` table", flash message matches spec.

#### Force refresh: `r`

- **[COVERED]** — Spec Key Bindings: "r — Force refresh from DB"

### `Plugin.KeyBindings()`

- **[COVERED]** — Spec Key Bindings table. All 9 bindings match.

### `Plugin.View(width, height, frame)`

- **[COVERED]** — Spec View section describes tab bar, PR list, hints.

#### Flash message behavior

- **5-second auto-clear** — **[UNCOVERED-BEHAVIORAL]** — Code clears flash messages after 5 seconds. Spec doesn't mention flash message duration. **Q: Is 5 seconds the intended duration, and should this be documented?**
- **Flash prepended to hints** — **[COVERED]** implicitly by spec mentioning flash messages for ignore actions.

### `renderTabBar`

- **[COVERED]** — Spec View > Tab Bar: "`[1] Waiting (3)  [2] Respond (1)  [3] Review (2)  [4] Stale (0)` with active tab highlighted"

### `renderPRRow` and per-category detail renderers

- **Waiting detail** — reviewers with pending/approved, CI, age. **[COVERED]** — Spec View: "reviewer statuses (pending/approved indicators), CI status, age"
- **Respond detail** — threads, review decision, who requested. **[COVERED]** — Spec View: "unresolved thread count, review decision badge, who requested changes"
- **Review detail** — author, age, draft. **[COVERED]** — Spec View: "PR author, age, draft indicator"
- **Stale detail** — age, draft, CI. **[COVERED]** — Spec View: "age, draft indicator, CI status"

### `renderAgentStatus`

- **All status values** — **[COVERED]** — Spec Agent Status Indicators table matches code exactly.
- **No-agent + no-repo fallback** — shows "no repo" warning. **[COVERED]** — Spec table: "No local repo → `⚠ no repo`"

### `renderCIStatus`

- **[COVERED]** — Spec Test Cases > CI Status Computation lists all cases.

### `renderReviewDecision`

- **[COVERED]** — Respond detail in spec mentions "review decision badge".
- **Specific values (APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, default)** — **[UNCOVERED-IMPLEMENTATION]** — rendering detail.

### `formatAge`

- **[UNCOVERED-IMPLEMENTATION]** — Internal formatting utility.

### Category constants and maps

- **[COVERED]** — Spec Routes and Category Assignment sections.
- **Empty state messages** — **[COVERED]** — Spec Empty States section, all 4 messages match exactly.

### `needsAgent` (pure predicate)

- **Path: category not review/respond** — false. **[COVERED]** — Spec Trigger Condition #1.
- **Path: status running/pending** — false. **[COVERED]** — Spec Trigger Condition #3.
- **Path: agent_head_sha empty** — true. **[COVERED]** — Spec Trigger Condition #2 parenthetical.
- **Path: head_sha != agent_head_sha** — true. **[COVERED]** — Spec Trigger Condition #2.
- **Path: category != agent_category** — true. **[COVERED]** — Spec Trigger Condition #2.
- **Otherwise** — false. **[COVERED]** — all conditions enumerated.

### `evaluateAgentTriggers`

- **DISABLED** — Code has `return nil` at top with comment "Disabled: re-enable after dedup fix is validated in production." **[UNCOVERED-BEHAVIORAL]** — Spec describes this as active behavior. The entire agent automation section assumes triggers fire. **Q: Should the spec note the temporary disable, or is this a code-level toggle that doesn't belong in the spec?**

---

## Spec-to-Code Direction (claims in spec not found in code)

### 1. OnComplete: `DBInsertBookmark`

Spec Data Flow #11 and OnComplete #2: "Insert bookmark via `DBInsertBookmark` for session resumption." Code in `SessionFinishedMsg` handler does NOT call `DBInsertBookmark`. **[CONTRADICTS]** — but this may be handled by the agent runner or daemon externally. Marking as potential gap.

### 2. Enter key "pending" flash message

Spec Enter Key table: `Agent pending → Flash: "Agent queued, waiting for slot..."`. Code returns `plugin.ConsumedAction()` with no flash. Already noted above.

### 3. Enter key "no local repo" flash message

Spec Enter Key table: `No local repo → Flash: "No local repo found — add a session path first"`. Code opens browser instead if URL is available, or silently consumes. Already noted above.

### 4. Enter key "running" → "Attach to live session"

Spec says attach; code navigates to command center tab. Already noted above.

---

## Contradictions Summary

| # | Location | Spec Says | Code Does |
|---|----------|-----------|-----------|
| 1 | Enter key, agent running | "Attach to live session" | Navigates to command center tab (`ActionNavigate "command"`) |
| 2 | Enter key, agent completed/failed + no local repo | Resume session (implied always works) | Silently returns no-op if `resolveRepoDir` returns empty |

## Behavioral Gaps (Intent Questions)

| # | Location | Question |
|---|----------|----------|
| 1 | `NavigateTo` invalid route | Should an invalid route log a warning or navigate to the default tab? |
| 2 | `Refresh` DB error | Should a DB load failure surface an error flash or log entry? Currently silently discards the error. |
| 3 | Enter key, completed agent with empty session ID | Should this show a flash message or fall through to manual launch (current behavior)? |
| 4 | Flash message auto-clear | Is the 5-second auto-clear duration intentional? Should the spec document it? |
| 5 | `evaluateAgentTriggers` disabled | Agent triggers are disabled in code but spec describes them as active. Should spec note the temporary disable? |

## Enter Key Spec vs Code Detail

The enter key has the most spec drift. Three sub-cases in the spec's Enter Key table don't match code:

- **Pending**: Spec promises a flash message; code is silent.
- **No local repo**: Spec promises a flash message; code opens browser or is silent.
- **Running**: Spec says "attach to live session"; code navigates to command center tab.
