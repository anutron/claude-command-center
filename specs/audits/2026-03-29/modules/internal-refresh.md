# Spec Audit: internal/refresh/

**Date:** 2026-03-29
**Specs:** `specs/core/refresh.md`, `specs/core/datasource.md`, `specs/core/todo-extraction.md`

## Summary

- **Total exported behavioral paths analyzed:** 98
- **Covered:** 62
- **Uncovered-Behavioral:** 24
- **Uncovered-Implementation:** 8
- **Contradictions:** 4

---

## Core: refresh.go

### `Run(opts Options) error`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 1 | Load env via `auth.LoadEnvFile()` | **[COVERED]** | refresh.md Behavior #1: "Load env vars from `~/.config/ccc/.env`" |
| 2 | Load existing state from DB | **[COVERED]** | refresh.md Behavior #2: "Load existing state from SQLite" |
| 3 | Parallel fetch of enabled sources | **[COVERED]** | refresh.md Behavior #4: "Parallel data fetch" |
| 4 | Disabled sources skipped | **[COVERED]** | datasource.md Test Cases: "Source with `Enabled() == false` is not fetched" |
| 5 | Source fetch error -> warning, not fatal | **[COVERED]** | datasource.md Test Cases: "Source returning error produces a warning" |
| 6 | Per-source sync status recorded via `DBUpsertSourceSync` | **[UNCOVERED-BEHAVIORAL]** | Code records success/failure per source in `cc_source_sync` table. Spec does not mention sync status tracking. **Intent question:** Should the spec document per-source sync status tracking and its use in incremental processing? |
| 7 | `combineResults` into FreshData | **[COVERED]** | refresh.md Behavior #5, datasource.md Behavior: "combineResults()" |
| 8 | Warnings from source results collected after combine | **[COVERED]** | datasource.md: "FreshData does not carry warnings. Source warnings...collected separately in Run()" |
| 9 | Merge fresh with existing | **[COVERED]** | refresh.md Behavior #6 |
| 10 | PostMerger hooks called after merge | **[COVERED]** | datasource.md Behavior: "Execute PostMerger hooks" |
| 11 | PostMerge errors logged, not fatal | **[UNCOVERED-BEHAVIORAL]** | Code logs PostMerge errors but continues. Spec says PostMerger is called but does not specify error handling. **Intent question:** Should PostMerge failures be fatal or best-effort? |
| 12 | `generateSuggestions` when LLM non-nil and todos exist | **[COVERED]** | refresh.md Behavior #8: "Generate suggestions" |
| 13 | `generateProposedPrompts` with RoutingLLM fallback to LLM | **[COVERED]** | refresh.md Behavior #9 and Options field docs |
| 14 | `dedupTodos` pass after routing | **[UNCOVERED-BEHAVIORAL]** | Code runs `dedupTodos()` which synthesizes merged todos via LLM. Not mentioned in any spec. **Intent question:** Should the spec document the dedup/merge-into pass, including veto logic and synthesis? |
| 15 | `FetchContextBestEffort` for todos with source_ref | **[COVERED]** | refresh.md Behavior #10: "Fetch source context" |
| 16 | DryRun -> print JSON to stdout | **[COVERED]** | refresh.md: "print JSON to stdout instead of writing" |
| 17 | Save merged state to DB | **[COVERED]** | refresh.md Behavior #11 |
| 18 | 3-minute context timeout | **[UNCOVERED-BEHAVIORAL]** | Code creates `context.WithTimeout(3*time.Minute)`. No spec mentions the timeout. **Intent question:** Is the 3-minute timeout a behavioral contract or tuning detail? |

### `dedupTodos(ctx, l, database, todos, merges) []db.Todo`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 19 | Group todos by MergeInto target | **[UNCOVERED-BEHAVIORAL]** | Entire dedup pass is unspecced. |
| 20 | Veto check via `WerePreviouslyMergedAndVetoed` | **[UNCOVERED-BEHAVIORAL]** | Veto logic not in spec. |
| 21 | Synthesis via LLM (`Synthesize`) | **[UNCOVERED-BEHAVIORAL]** | See todo_synthesis.go below. |
| 22 | Replace old synthesis target with new synthesized todo | **[UNCOVERED-BEHAVIORAL]** | DB operations (insert, delete merges, delete old) not specified. |
| 23 | `logSourceResult` verbose logging | **[UNCOVERED-IMPLEMENTATION]** | Logging detail, no spec needed. |

---

## Core: merge.go

### `Merge(existing *db.CommandCenter, fresh *FreshData) *db.CommandCenter`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 24 | Nil existing -> default empty | **[COVERED]** | refresh.md Test Cases: "Nil existing state handled gracefully" |
| 25 | Calendar replaced entirely | **[COVERED]** | refresh.md Merge Rules: "Calendar: Replaced entirely" |
| 26 | PullRequests replaced entirely from fresh | **[CONTRADICTS]** | Spec says "Merge-based upsert...Each fresh PR is upserted by ID...agent tracking columns preserved...missing PRs archived." Code does `PullRequests: fresh.PullRequests` (full replace, no upsert, no archival, no agent column preservation). **This is a significant spec/code disagreement.** |
| 27 | PendingActions preserved from existing | **[COVERED]** | refresh.md Merge Rules: "PendingActions: Preserved" |
| 28 | Suggestions preserved from existing | **[UNCOVERED-BEHAVIORAL]** | Code preserves `existing.Suggestions`. Spec does not mention Suggestions merge behavior. **Intent question:** Should the spec document that Suggestions are preserved until overwritten by LLM? |

### `mergeTodos(existing, fresh []db.Todo) []db.Todo`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 29 | Fresh todo with no source_ref -> new with generated ID | **[COVERED]** | refresh.md Merge Rules: "new items get generated IDs" |
| 30 | Fresh matches existing by source_ref, existing is dismissed -> skip (tombstone) | **[COVERED]** | refresh.md: "dismissed = tombstone (never recreated)" |
| 31 | Fresh matches existing, existing is completed -> preserve as-is | **[UNCOVERED-BEHAVIORAL]** | Code preserves completed todos without updating fields. Spec only mentions dismissed tombstones, not completed preservation. **Intent question:** Should completed todos be treated as immutable like dismissed ones? |
| 32 | Fresh matches existing, active -> update title/detail/context, preserve ID/status/created_at | **[COVERED]** | refresh.md: "existing items preserve ID/status/created_at while updating title/detail/context" |
| 33 | Preserve ProposedPrompt if already set | **[UNCOVERED-BEHAVIORAL]** | Code only writes ProposedPrompt if existing is empty. Not in spec. |
| 34 | Fresh matches existing, preserve Due/Effort only if fresh non-empty | **[UNCOVERED-IMPLEMENTATION]** | Defensive detail. |
| 35 | New todo gets status "new" | **[CONTRADICTS]** | Spec says "new items get generated IDs and 'active' status" but code sets `Status: "new"`. **Spec says "active", code says "new".** |
| 36 | Unmatched existing todos preserved | **[COVERED]** | refresh.md: "manual items always preserved" (and general preservation) |

---

## Core: datasource.go

### `combineResults(results []*SourceResult) *FreshData`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 37 | Nil results skipped | **[UNCOVERED-IMPLEMENTATION]** | Defensive nil check. |
| 38 | Calendar uses first non-nil | **[COVERED]** | datasource.md: "Calendar: uses first non-nil" |
| 39 | Todos concatenated | **[COVERED]** | datasource.md: "Todos: concatenates" |
| 40 | PullRequests concatenated | **[CONTRADICTS]** | datasource.md says "Threads: concatenates" but code concatenates PullRequests. The SourceResult struct has PullRequests, not Threads. Spec references Threads which no longer exist in SourceResult. |
| 41 | ANSI sanitization on calendar titles | **[COVERED]** | datasource.md Data Safety: "ANSI Sanitization" |
| 42 | ANSI sanitization on todo fields | **[COVERED]** | datasource.md Data Safety: "ANSI Sanitization" |
| 43 | ANSI sanitization on PR titles | **[COVERED]** | By extension of sanitization spec. |

---

## Core: context.go

### `NewContextRegistry() *ContextRegistry`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 44 | Creates empty registry | **[COVERED]** | refresh.md Source Context: "ContextRegistry maps source names" |

### `Register(source, fetcher)` / `Get(source)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 45 | Register and retrieve fetchers | **[COVERED]** | refresh.md: "Registered at startup in ai-cron" |

### `shouldRefresh(todo, fetcher) bool`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 46 | Empty SourceContext -> true | **[UNCOVERED-IMPLEMENTATION]** | Internal cache freshness logic. |
| 47 | TTL=0 (immutable) -> false after first fetch | **[COVERED]** | refresh.md: "Granola: 0 (immutable)" implies no re-fetch. |
| 48 | TTL expired -> true | **[UNCOVERED-IMPLEMENTATION]** | Standard TTL cache behavior. |

### `FetchAndSave(ctx, db, registry, todo) (string, error)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 49 | No fetcher for source -> return empty, nil | **[UNCOVERED-BEHAVIORAL]** | Code silently returns nothing for unregistered sources. **Intent question:** Should missing fetchers be logged or flagged? |
| 50 | Context still fresh -> return cached | **[COVERED]** | Implied by TTL spec. |
| 51 | Fetch and persist to DB | **[COVERED]** | refresh.md: "stored in source_context and source_context_at columns" |

### `FetchContextBestEffort`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 52 | Errors logged, not returned | **[COVERED]** | refresh.md: "FetchContextBestEffort is called for each todo" (best-effort by name) |

---

## Core: llm.go

### `generateSuggestions(ctx, llm, cc) (*db.Suggestions, error)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 53 | Serialize active todos + calendar, call LLM | **[COVERED]** | refresh.md Behavior #8 |
| 54 | Parse LLM response into Suggestions | **[UNCOVERED-IMPLEMENTATION]** | JSON parse detail. |
| 55 | `CleanJSON` strips markdown fences and extracts JSON | **[COVERED]** | datasource.md: "CleanJSON() helper is exported" |

### `generateProposedPrompts(ctx, llm, db, todos) []db.Todo`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 56 | Filter eligible: active, has source, not manual, no prompt yet | **[COVERED]** | refresh.md Behavior #9: "Route eligible todos (active, has source, no prompt yet)" |
| 57 | Load path context from DB | **[UNCOVERED-BEHAVIORAL]** | Code loads paths, routing rules, skills from DB for routing. Spec does not describe path context, skills, or routing rules. **Intent question:** Should the spec document the path/skill/routing-rule context used in prompt generation? |
| 58 | No paths -> legacy batch prompt | **[UNCOVERED-BEHAVIORAL]** | Fallback to `generateProposedPromptsLegacy` when no learned paths exist. Not in spec. |
| 59 | REJECT -> auto-dismiss todo | **[COVERED]** | refresh.md Behavior #9: "If the LLM returns `project_dir: "REJECT"`, the todo is auto-dismissed" |
| 60 | Assign ProjectDir and ProposedPrompt | **[COVERED]** | refresh.md Behavior #9: "assigns a project directory and generates an actionable prompt" |
| 61 | MergeInto field populated by routing | **[UNCOVERED-BEHAVIORAL]** | Routing LLM can suggest merging todos. Not in spec. |
| 62 | `activeTodos` helper | **[UNCOVERED-IMPLEMENTATION]** | Utility filter. |

---

## Core: todo_agent.go

### `GenerateTodoPrompt(ctx, llm, todo, paths, activeTodos) (*TodoPromptResult, error)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 63 | Build routing prompt with task, source context, available projects, active todos | **[UNCOVERED-BEHAVIORAL]** | The routing prompt structure (project descriptions, skills, routing rules, dedup hints, user instructions) is unspecced. **Intent question:** Should the spec document what context is provided to the routing LLM? |
| 64 | Source context included in `<source_context>` tags | **[COVERED]** | refresh.md: "The routing prompt includes source context in `<source_context>` tags" |
| 65 | Ownership validation rules in prompt | **[COVERED]** | refresh.md Behavior #9 and todo-extraction.md Stage 3 |
| 66 | Active todos list capped at 50 for dedup | **[UNCOVERED-IMPLEMENTATION]** | Performance guard. |
| 67 | `loadTodoInstructions()` from file | **[UNCOVERED-BEHAVIORAL]** | Reads `todo_instructions.md` from CWD or `~/.config/ccc/`. Not in spec. **Intent question:** Should user-provided routing instructions be documented? |

---

## Core: todo_synthesis.go

### `Synthesize(ctx, llm, originals) (*SynthesisResult, error)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 68 | LLM combines multiple todos into one | **[UNCOVERED-BEHAVIORAL]** | Entire synthesis feature unspecced. |

### `BuildSynthesisTodo(result, originals, mergeTarget) db.Todo`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 69 | Status from mergeTarget, non-LLM fields from newest original | **[UNCOVERED-BEHAVIORAL]** | Unspecced. |
| 70 | Source set to "merge" | **[UNCOVERED-BEHAVIORAL]** | Unspecced. |

---

## Sources: calendar/

### `CalendarSource.Fetch(ctx)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 71 | Migrate credentials before fetch | **[COVERED]** | refresh.md Behavior #3: "Migrate calendar credentials if needed" |
| 72 | Load auth from credentials.json/token.json chain | **[COVERED]** | refresh.md Data Sources table |
| 73 | Default to "primary" calendar if no IDs configured | **[UNCOVERED-BEHAVIORAL]** | Code defaults `calendarIDs = ["primary"]`. Not in spec. |
| 74 | Fetch today/tomorrow events | **[COVERED]** | refresh.md Data Sources: "Today/tomorrow events" |
| 75 | Auto-accept events from configured domains | **[COVERED]** | refresh.md: "Auto-accept is configurable via CalendarSource.AutoAcceptDomains" |
| 76 | Skip workingLocation events | **[UNCOVERED-BEHAVIORAL]** | `listEvents` filters out `item.EventType == "workingLocation"`. Not in spec. |
| 77 | Parse all-day vs timed events | **[UNCOVERED-IMPLEMENTATION]** | Parsing detail. |
| 78 | Mark declined events | **[UNCOVERED-BEHAVIORAL]** | Code sets `ev.Declined = true` for self-declined events. Not in spec. |

### `CalendarSource.PostMerge(ctx, db, cc, verbose)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 79 | Execute pending booking actions | **[COVERED]** | refresh.md Behavior #7: "Execute pending actions" |

### `ListAvailableCalendars() ([]CalendarInfo, error)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 80 | List all calendars visible to user | **[UNCOVERED-BEHAVIORAL]** | Settings UI function. Not in refresh spec (belongs to settings behavior). |

### `RunCalendarAuth() error`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 81 | OAuth2 flow with PKCE and random state | **[COVERED]** | refresh.md Security: "PKCE (S256)", "Random state parameter", "Loopback binding" |

### `MigrateCalendarCredentials() error`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 82 | Skip if credentials.json exists | **[COVERED]** | datasource.md: "Calendar credential migration happens in CalendarSource.Fetch()" |
| 83 | Copy token.json fields + client creds to credentials.json | **[COVERED]** | Implied by migration behavior. |

### `ValidateCalendarResult() plugin.ValidationResult`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 84 | Structural credential check for doctor | **[UNCOVERED-BEHAVIORAL]** | Doctor/settings validation. Belongs in a settings/doctor spec. |

---

## Sources: github/

### `GitHubSource.Fetch(ctx)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 85 | Fetch username from `gh api /user` if not configured | **[COVERED]** | Implied by datasource.md: "Auth loading happens inside Fetch()" |
| 86 | Fetch authored PRs + review-requested PRs | **[COVERED]** | refresh.md Data Sources: "Open PRs authored by user" |
| 87 | Fetch detail per PR (reviews, CI, headRefOid) | **[COVERED]** | datasource.md: "gh pr view --json includes headRefOid" |
| 88 | PR detail failure -> warning, not error | **[COVERED]** | datasource.md Test Cases |
| 89 | Build PullRequests with category computation | **[UNCOVERED-BEHAVIORAL]** | `computeCategory` assigns review/respond/stale/waiting categories. Not in refresh spec. **Intent question:** Should PR categorization rules be specced? |
| 90 | `computeRole` (author/reviewer/both) | **[UNCOVERED-BEHAVIORAL]** | Role computation logic not specced. |
| 91 | `computeCIStatus` (success/failure/pending) | **[UNCOVERED-BEHAVIORAL]** | CI status derivation not specced. |
| 92 | 14-day stale threshold | **[UNCOVERED-BEHAVIORAL]** | `staleDays = 14` is a behavioral constant. Not in spec. |

---

## Sources: gmail/

### `GmailSource.Fetch(ctx)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 93 | Load auth with scope selection (readonly vs modify+compose) | **[COVERED]** | datasource.md: "Read-only mode / Advanced mode" |
| 94 | Fetch label-based todos | **[COVERED]** | datasource.md: "Gmail label-based todos" |
| 95 | LLM title generation for labeled emails | **[COVERED]** | todo-extraction.md: "Gmail: Label-Based Title Generation" |
| 96 | LLM commitment detection on sent emails (advanced only) | **[COVERED]** | datasource.md: "LLM commitment detection: analyzes sent emails and auto-labels" |
| 97 | Fallback to subject when LLM nil | **[COVERED]** | todo-extraction.md: "No LLM available -> falls back to email subject" |

### `GmailSource.PostMerge(ctx, db, cc, verbose)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| 98 | Remove label from completed gmail todos | **[COVERED]** | datasource.md: "Label removal on completion: PostMerge removes the todo label" |

### `SafeGmailClient`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| -- | No Send/Delete/Trash methods | **[COVERED]** | CLAUDE.md + datasource.md: "Safety: Gmail API access is wrapped in SafeGmailClient" |
| -- | ModifyLabels blocked in non-advanced mode | **[COVERED]** | datasource.md: "Read-only mode...No write-back" |

---

## Sources: granola/

### `GranolaSource.Fetch(ctx)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| -- | Load auth from Electron app cache | **[COVERED]** | refresh.md Data Sources: "Granola stored auth" |
| -- | Fetch meetings from this week | **[COVERED]** | refresh.md Data Sources: "This week's meetings with transcripts" |
| -- | Skip already-processed meetings (via source sync) | **[UNCOVERED-BEHAVIORAL]** | Code checks `lastSuccess` from `cc_source_sync` to skip old meetings. Not in spec. Related to item #6. |
| -- | Token expiry check | **[UNCOVERED-BEHAVIORAL]** | Code checks `savedAt + expiresIn` and returns error if expired. Not specced. |
| -- | Speaker labels: microphone=[Aaron], system=[Other] | **[COVERED]** | refresh.md Source Context: "Speaker Attribution (Granola)" |
| -- | 10MB response limit | **[COVERED]** | refresh.md Security: "API Response Size Limits" |

---

## Sources: slack/

### `SlackSource.Fetch(ctx)`

| # | Path | Classification | Notes |
|---|------|---------------|-------|
| -- | Conversations API with search.messages fallback | **[COVERED]** | todo-extraction.md: "Search Queries (Slack search.messages fallback)" |
| -- | Pre-filter by commitment phrases | **[COVERED]** | todo-extraction.md: "Stage 1: Pre-Filter" |
| -- | Conversation context (15 preceding same-day messages) | **[COVERED]** | todo-extraction.md: "Stage 1b: Conversation Context" |
| -- | Thread replies fetched | **[COVERED]** | todo-extraction.md: "Thread replies" |
| -- | Skip already-processed messages (via source sync with 2min overlap) | **[UNCOVERED-BEHAVIORAL]** | 2-minute overlap window to avoid missing messages during sync. Not in spec. |
| -- | Rate limit retry (3 attempts, Retry-After header) | **[UNCOVERED-BEHAVIORAL]** | `slackAPIGet` retries on 429 with Retry-After. Not in spec. |
| -- | 10MB response limit | **[COVERED]** | refresh.md Security: "API Response Size Limits" |
| -- | Calendar-day boundary for conversation context | **[COVERED]** | todo-extraction.md: "Stops at the calendar day boundary (Pacific time)" |

---

## Contradictions

### 1. PR Merge Rules: Spec vs Code

- **Spec (refresh.md Merge Rules):** "PullRequests: Merge-based upsert...Each fresh PR is upserted by ID...agent tracking columns preserved...PRs missing from fresh batch are archived...Archived PRs reappearing are reactivated."
- **Code (merge.go:25):** `PullRequests: fresh.PullRequests` -- full replacement, no upsert logic, no archival, no agent column preservation.
- **Impact:** High. Agent tracking data (`agent_session_id`, `agent_status`, etc.) is destroyed every refresh.

### 2. New Todo Default Status

- **Spec (refresh.md):** "new items get generated IDs and 'active' status"
- **Code (merge.go:91):** `ft.Status = "new"`
- **Impact:** Medium. Status filtering may behave differently than expected.

### 3. SourceResult.Threads vs SourceResult.PullRequests

- **Spec (datasource.md):** SourceResult has `Threads []db.Thread`, combineResults "Threads: concatenates"
- **Code (datasource.go):** SourceResult has `PullRequests []db.PullRequest`, no Threads field. combineResults concatenates PullRequests.
- **Impact:** Spec references a removed field. Thread merge logic described in refresh.md ("Threads: Matched by URL...") has no corresponding code in merge.go.

### 4. Thread Merge Rules Orphaned

- **Spec (refresh.md Merge Rules):** "Threads: Matched by URL; completed/dismissed never recreated; paused state preserved; summary updated from fresh data"
- **Code:** No thread merge logic exists. Threads are not in FreshData or SourceResult.
- **Impact:** Spec documents behavior for a removed feature.

---

## Spec -> Code Gaps (spec describes behavior not found in code)

1. **Thread handling entirely** -- spec describes Thread merge, Thread in SourceResult, but code has PullRequests instead
2. **PR upsert with agent column preservation** -- specced but not implemented (simple replacement instead)
3. **PR archival for missing PRs** -- specced but not implemented
4. **PR reactivation from archived state** -- specced but not implemented

## Code -> Spec Gaps (code has behavior not in spec)

1. **Per-source sync tracking** (`DBUpsertSourceSync`) -- incremental processing optimization
2. **Todo dedup/synthesis** (entire `dedupTodos` + `todo_synthesis.go`) -- LLM-driven duplicate merging
3. **MergeInto field on routing** -- routing LLM can suggest todo merges
4. **Path context/skills/routing rules in prompt generation** -- rich context for routing
5. **`loadTodoInstructions()`** -- user-provided routing instruction file
6. **3-minute context timeout** on refresh run
7. **Legacy prompt generation fallback** when no paths available
8. **Granola token expiry check** before fetch
9. **Slack rate limit retry logic** (3 attempts with Retry-After)
10. **Slack 2-minute overlap window** for incremental processing
11. **Calendar workingLocation event filtering**
12. **Calendar declined event marking**
13. **Calendar default to "primary" when no IDs configured**
14. **GitHub PR categorization** (review/respond/stale/waiting)
15. **GitHub PR role computation** (author/reviewer/both)
16. **GitHub CI status derivation**
17. **GitHub 14-day stale threshold**
18. **Completed todo preservation** (not just dismissed tombstones)
19. **Suggestions preserved from existing state during merge**
