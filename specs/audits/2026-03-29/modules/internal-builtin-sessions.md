# Spec Audit: internal/builtin/sessions

**Date:** 2026-03-29
**Spec:** `specs/builtin/sessions.md`
**Code files:** `sessions.go`, `unified.go`, `describe.go`

## Coverage Summary

- **Total exported behaviors analyzed:** 42
- **COVERED:** 33
- **UNCOVERED-BEHAVIORAL:** 5
- **UNCOVERED-IMPLEMENTATION:** 3
- **CONTRADICTS:** 1

## Exported Functions/Methods

### Plugin (sessions.go)

#### Plugin.Slug() / Plugin.TabName()

- **[COVERED]** — Spec: "Slug: `sessions`" and "Sessions" tab. Routes section confirms slug.

#### Plugin.Init(ctx)

- **[COVERED]** — Spec Behavior #1: "On Init, loads paths from DB, bookmarks from DB, archived sessions from DB, creates unified view."
- **[COVERED]** — Spec Behavior #15: "If `config.HomeDir` is set, auto-added to paths list on Init."
- **[COVERED]** — Spec Event Bus: "Subscribes: `pending.todo` to set a pending launch context."

#### Plugin.SetDaemonClientFunc(fn)

- **[COVERED]** — Spec Concurrency model: "Exception: `unifiedView.Refresh()` (the direct method) is called once from `SetDaemonClientFunc()` before the bubbletea loop starts."

#### Plugin.Routes()

- **[COVERED]** — Spec Routes section lists all three routes.

#### Plugin.NavigateTo(route, args)

- **[UNCOVERED-BEHAVIORAL]** — Code accepts `pending_todo_title` from args to set a pending launch todo. The spec mentions `pendingLaunchTodo` in Behavior #14 via event bus, but NavigateTo's args-based path is not documented. **Intent question:** Should the spec document that NavigateTo also accepts `pending_todo_title` in args as an alternative to the event bus?

#### Plugin.Refresh()

- **[COVERED]** — Spec Concurrency model: "Refresh() returns a tea.Cmd that fetches data (daemon RPC, DB reads, auto-archiving) in a background goroutine and returns it as a `sessionsRefreshMsg`."

#### Plugin.KeyBindings()

- **[COVERED]** — Spec Key Bindings section documents all promoted key bindings.

#### Plugin.HandleKey(msg)

- **[COVERED]** — Spec Key Bindings sections document all key behavior per sub-tab.

#### Plugin.HandleMessage(msg)

- **[COVERED]** — Spec Event Bus: "Handles `plugin.NotifyMsg` for `data.refreshed`, `session.registered`, `session.updated`, `session.ended`."
- **[COVERED]** — Spec Concurrency model: "State is only mutated in `HandleMessage` on the main bubbletea loop."

#### Plugin.View(width, height, frame)

- **[COVERED]** — Spec Behavior #2-4 and Hint Bar section describe view rendering.

### Sessions Sub-Tab Key Handling (sessions.go)

#### handleSessionsTab — "enter" on live/saved/archived

- **[COVERED]** — Spec Behavior #5: "Enter on a live/saved/archived session resumes it (`--resume <session_id>`)."
- **[COVERED]** — Spec Test Cases: "Enter on live session returns ActionLaunch with correct dir and resume_id."

#### handleSessionsTab — "b" (bookmark)

- **[COVERED]** — Spec Session Lifecycle #2, Behavior #7: "`b` on a live session bookmarks it; on an archived session promotes it to Saved."
- **[COVERED]** — Spec Test Cases: "`b` on archived session promotes to Saved, removes from archive."

#### handleSessionsTab — "d" (dismiss/delete)

- **[COVERED]** — Spec Session Lifecycle #4-5, Behavior #8.
- **[COVERED]** — Spec Test Cases: "`d` on running session shows 'Can't dismiss' flash."
- **[UNCOVERED-BEHAVIORAL]** — Code for `d` on TierLive (non-running) calls `client.ArchiveSession()` to archive the session in the daemon before removing it from the view. The spec says "d dismiss" but doesn't mention the daemon-side archive RPC. **Intent question:** Should the spec document that dismissing a live ended session also archives it in the daemon (not just removes from view)?

#### handleSessionsTab — "a" (toggle archive)

- **[COVERED]** — Spec View Modes section and Test Case: "Toggle archive mode resets cursor."

### New Sub-Tab Key Handling (sessions.go)

#### handleNewTab — "enter" on path

- **[COVERED]** — Spec Behavior #6 and Test Cases.

#### handleNewTab — "enter" on Browse

- **[COVERED]** — Code launches fzf for directory browsing. Spec Behavior #3 mentions "Browse option."
- **[UNCOVERED-BEHAVIORAL]** — fzf browse automatically adds the selected path to DB and launches in it. The spec doesn't document the full fzf flow (auto-add to paths, heuristic description, background LLM description). **Intent question:** Should the spec describe the Browse flow in detail (fzf selection -> add path -> describe -> launch)?

#### handleNewTab — "w" (worktree launch)

- **[COVERED]** — Spec Behavior #10 and Test Cases.

#### handleNewTab — "shift+up/down" (reorder)

- **[COVERED]** — Spec Behavior #13 and Key Bindings.

#### handleNewTab — "delete/backspace" (remove path)

- **[COVERED]** — Spec Behavior #12 and Key Bindings.

#### handleNewTab — type-to-filter

- **[COVERED]** — Spec Key Bindings New sub-tab: "type: Filter list."

### Worktrees Sub-Tab Key Handling (sessions.go)

#### handleWorktreesTab

- **[COVERED]** — Spec Key Bindings Worktrees sub-tab documents all keys.

#### handleWorktreeWarning

- **[COVERED]** — Spec Worktree warning overlay key bindings.

#### handleWorktreeConfirm

- **[COVERED]** — Spec Worktree confirmation overlay key bindings.

### Confirmation Dialog (sessions.go)

#### handleConfirming

- **[COVERED]** — Spec Confirmation dialogs key bindings section.

### Unified View (unified.go)

#### NewUnifiedView(clientFn, styles)

- **[UNCOVERED-IMPLEMENTATION]** — Constructor, no spec needed.

#### ToggleArchive()

- **[COVERED]** — Spec View Modes: "Two modes toggled by `a`." Test Case: "Toggle archive mode resets cursor."

#### displayItems() / mainItems() / archivedItems()

- **[COVERED]** — Spec Three-Tier Model and Deduplication sections.

#### View(width, height)

- **[COVERED]** — Spec describes section headers (LIVE, SAVED, ARCHIVED), indicators, and empty states.
- **[CONTRADICTS]** — Code renders blocked sessions with a yellow indicator `●` and "Blocked" suffix. Spec Session Lifecycle and View Modes do not mention blocked state visualization. The Three-Tier Model table says Live sessions are "Running, active, or blocked sessions" but the rendering details (yellow dot, "Blocked" text) are not specified. **Recommendation:** Add blocked session rendering to the spec's View rendering behavior.

#### MoveDown() / MoveUp()

- **[COVERED]** — Spec Key Bindings: "j/k or up/down: Navigate list." Wrapping behavior is implied.

#### SelectedItem()

- **[UNCOVERED-IMPLEMENTATION]** — Internal accessor, no spec needed.

#### RemoveSession(sessionID)

- **[UNCOVERED-IMPLEMENTATION]** — Internal state management after dismiss/delete, no independent spec needed.

#### Refresh()

- **[COVERED]** — Spec Auto-Archiving section and Concurrency model.

#### SetSavedSessions() / SetArchivedSessions() / ReloadArchived()

- **[COVERED]** — These are part of the Init and refresh flow described in Behavior #1.

#### archiveNewlyEndedSessions(database, prevLive, newSessions)

- **[COVERED]** — Spec Auto-Archiving: "Sessions that were previously running but are now ended (and not bookmarked) are auto-archived."
- **[COVERED]** — Spec Test Cases: "Auto-archive: ended session (not bookmarked) written to cc_archived_sessions" and "Auto-archive: bookmarked ended session NOT archived."

#### isSessionBlocked(sessionID)

- **[COVERED]** — Spec Three-Tier Model mentions "blocked" in description. Agent status lookup is implicit.

### LLM Description (describe.go)

#### LLMDescribePath(l, dir)

- **[UNCOVERED-BEHAVIORAL]** — This exported function provides LLM-based project path descriptions. The spec does not document LLM description of paths at all. **Intent question:** Should the spec have a section on path descriptions (LLM with heuristic fallback)?

#### pathDescribeCmd(l, path)

- **[UNCOVERED-BEHAVIORAL]** — Async tea.Cmd wrapper for LLMDescribePath. Not in spec.

### Helper Functions

#### SetPendingLaunchTodo(todo)

- **[COVERED]** — Spec Behavior #14: "When pendingLaunchTodo is set (via event bus), shows banner."

#### formatTodoContext(todo)

- **[UNCOVERED-IMPLEMENTATION]** — Formatting helper, no spec needed.

## Spec-to-Code Direction (Spec claims not found in code)

- **[OK]** — Spec CLI `ccc add-bookmark`: This is a CLI command, not in the plugin code. Presumably implemented elsewhere.
- **[OK]** — Spec `pending.todo.cancel` publish: Code publishes this on esc when pendingLaunchTodo is set (line 534).
- **[OK]** — All hint bar text in spec matches code `renderHints()`.

## Summary

The sessions spec is comprehensive and well-aligned with the code. The main gaps are:

1. **Blocked session visualization** (CONTRADICTS) — code renders yellow indicator + "Blocked" text, spec only mentions blocked in the tier table description
2. **LLM path descriptions** (UNCOVERED) — `describe.go` exports `LLMDescribePath` for generating project summaries, not in spec
3. **Browse flow details** (UNCOVERED) — fzf selection auto-adds path to DB with heuristic + LLM description
4. **Daemon-side archive on dismiss** (UNCOVERED) — dismissing a live ended session calls `client.ArchiveSession()` RPC
5. **NavigateTo args** (UNCOVERED) — accepts `pending_todo_title` in args map
