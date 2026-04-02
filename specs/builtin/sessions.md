# SPEC: Sessions Plugin (built-in)

## Purpose

Manage sessions, project launching, and worktrees as a plugin. Users can browse active/saved/archived sessions, launch new Claude sessions from project paths, and manage git worktrees.

## Slug: `sessions`

## Routes

- `sessions/active` — live sessions only (Active tab, default)
- `sessions/resume` — saved/bookmarked sessions only (Resume tab)
- `sessions/new` — new session list
- `sessions/worktrees` — worktrees sub-tab
- `sessions/sessions` — legacy alias for `active`

## State

- unified *unifiedView (manages live, saved, archived sessions)
- newList (bubbles/list.Model)
- paths []string
- confirming, confirmYes bool
- confirmItem
- sub-tab: "sessions" (active), "resume", "new", or "worktrees"
- worktreeItems []worktreeItem
- worktreeCursor int
- worktreeWarning string (non-empty = show warning overlay)
- worktreeConfirmAction string ("delete" or "prune")
- worktreeConfirmTarget string
- flashMessage string, flashMessageAt time.Time

## Sessions Tab: Three-Tier Model

The sessions tab displays sessions in three tiers:

| Tier | Source | Visibility | Description |
|------|--------|------------|-------------|
| **Live** | Daemon RPC | Always (main mode) | Running, active, or blocked sessions. Blocked sessions render with a yellow dot indicator and "Blocked" text. |
| **Saved** | `cc_bookmarks` table | Always (main mode) | User-bookmarked sessions |
| **Archived** | `cc_archived_sessions` table | Archive mode only | Auto-saved ended sessions |

### Session Lifecycle

1. Session registers with daemon → appears in **Live**
2. User presses `b` on a live session → also saved to `cc_bookmarks`
3. Session ends in daemon:
   - If bookmarked → moves to **Saved**
   - If not bookmarked → auto-persisted to **Archived**
4. `d` on archived → permanently deletes from DB
5. `d` on saved → removes bookmark

### Deduplication

A session that is both live in the daemon AND bookmarked appears in **Live** only, with a ★ indicator. When it ends, it moves to **Saved**.

### View Modes

The Active and Resume tabs each have their own view filter. The `A` key (shift-a) toggles archive mode within the current tab. The `a` key (lowercase) archives the selected session.

| Tab / Mode | Contents | Default |
|------------|----------|---------|
| **Active** (main) | Live sessions only | Yes |
| **Active** (archive) | Archived sessions only | No |
| **Resume** (main) | Saved/bookmarked sessions only | Yes |
| **Resume** (archive) | Archived sessions only | No |

The Active tab MUST NOT show saved sessions. Saved sessions appear exclusively in the Resume tab.

### Auto-Archiving

When `Refresh()` polls the daemon, it compares the current session list against the previous snapshot. Sessions that were previously running but are now ended (and not bookmarked) are auto-archived to `cc_archived_sessions`. If the daemon is disconnected, no archiving occurs.

**Concurrency model:** `Refresh()` returns a `tea.Cmd` that fetches data (daemon RPC, DB reads, auto-archiving) in a background goroutine and returns it as a `sessionsRefreshMsg`. State is only mutated in `HandleMessage` on the main bubbletea loop, never from background goroutines. This prevents data races between tea.Cmd goroutines and `View()`. Exception: `unifiedView.Refresh()` (the direct method) is called once from `SetDaemonClientFunc()` before the bubbletea loop starts — this is safe because no concurrent access exists at that point.

## Key Bindings

### Global (available from any sub-tab, when not in overlay)

| Key | Description | Promoted |
|-----|-------------|----------|
| s | Switch to Sessions sub-tab | yes |
| n | Switch to New sub-tab | yes |
| t | Switch to Worktrees sub-tab | yes |
| esc | Quit (or back to new from sessions/worktrees) | yes |

### Sessions sub-tab (main mode)

| Key | Description | Promoted |
|-----|-------------|----------|
| enter | Resume selected session | yes |
| b | Bookmark live session → Saved | yes |
| d | Dismiss/remove (tier-dependent) | yes |
| a | Archive selected session (verb action) | yes |
| A | View archive list (toggle archive mode) | yes |
| j/k or up/down | Navigate list | yes |

### Sessions sub-tab (archive mode)

| Key | Description | Promoted |
|-----|-------------|----------|
| enter | Resume archived session | yes |
| b | Promote to Saved (bookmark) | yes |
| d | Permanently delete | yes |
| A | Return to main mode | yes |
| j/k or up/down | Navigate list | yes |

### New sub-tab

| Key | Description | Promoted |
|-----|-------------|----------|
| enter | Launch session in selected path | yes |
| w | Launch in a new worktree (git repos only) | yes |
| shift+up/down | Reorder paths | yes |
| del/backspace | Remove from saved list (with confirmation) | yes |
| up/down | Navigate list | yes |
| type | Filter list | yes |

### Worktrees sub-tab

| Key | Description | Promoted |
|-----|-------------|----------|
| enter | Launch Claude in selected worktree | yes |
| d | Delete selected worktree (with confirmation) | yes |
| p | Prune all worktrees for selected project (with confirmation) | yes |
| up/down/k/j | Navigate worktree list | yes |
| esc | Back to new sub-tab | - |

### Confirmation dialogs (delete path)

| Key | Description |
|-----|-------------|
| y | Confirm delete |
| n/esc | Cancel |
| left/right/tab | Toggle yes/no selection |
| enter | Execute currently highlighted choice |

### Worktree warning overlay (not a git repo)

| Key | Description |
|-----|-------------|
| enter | Launch directly in the directory (without worktree) |
| esc | Cancel, return to list |

### Worktree confirmation overlay (delete/prune)

| Key | Description |
|-----|-------------|
| y | Confirm delete or prune |
| n/esc | Cancel |

## Hint Bar

Each sub-tab displays a hint bar at the bottom:

- **Sessions (main):** `enter resume   b bookmark   d dismiss   j/k navigate   a archive   A view archive   n new   t worktrees`
- **Sessions (archive):** `enter resume   b save   d delete   j/k navigate   A back   n new   t worktrees`
- **New:** `type to filter   enter launch   w worktree   s sessions   n new   t worktrees   shift+up/down reorder   del remove   esc quit`
- **Worktrees:** `enter launch   d delete   p prune   s sessions   n new   esc back`
- **Worktree warning:** `⚠ Not a git repository — worktrees require git.` + `[enter] Launch directly in this directory   [esc] Cancel`
- **Delete confirmation:** `Delete worktree <label>?` + `[y] Yes, delete   [n] Cancel`
- **Prune confirmation:** `Remove all worktrees for <project>? (<count> worktrees)` + `[y] Yes, prune all   [n] Cancel`

## Event Bus

- Publishes: `project.selected` with {path, prompt} when user picks a project
- Publishes: `pending.todo.cancel` when user cancels a pending todo launch
- Subscribes: `pending.todo` to set a pending launch context
- Handles `plugin.NotifyMsg` for `data.refreshed`, `session.registered`, `session.updated`, `session.ended` — dispatches async `Refresh()` cmd
- Subscribes (event bus): `pending.todo` to set a pending launch context

## Storage

### cc_bookmarks (user-curated)

| Column | Description |
|--------|-------------|
| session_id | Claude Code session UUID (primary key) |
| project | Directory where Claude indexes the session |
| repo | Repository display name |
| branch | Branch name at bookmark time |
| label | User-provided label |
| summary | One-line summary of session work |
| worktree_path | Worktree directory path (NULL if not a worktree session) |
| source_repo | Main repo path for worktree sessions (NULL if not a worktree) |

### cc_archived_sessions (auto-saved)

| Column | Description |
|--------|-------------|
| session_id | Session UUID (primary key) |
| topic | Session topic at time of archiving |
| project | Project directory |
| repo | Repository name |
| branch | Branch name |
| worktree_path | Worktree path (if applicable) |
| registered_at | When session was registered (NOT NULL) |
| ended_at | When session ended (NOT NULL) |

### Worktree-aware bookmarks

Claude Code stores session files under `~/.claude/projects/<project-path-encoded>/`. For worktree sessions, Claude maps to the **main repo's** project dir.

When creating a bookmark from a worktree:
- `project` = main repo path (where Claude indexes sessions)
- `worktree_path` = the actual worktree directory

When resuming a worktree bookmark:
- If `worktree_path` is set, `cd` to the worktree path
- If `worktree_path` is empty, `cd` to `project`

### CLI: `ccc add-bookmark`

Flags: `--session-id`, `--project`, `--repo`, `--branch`, `--summary` (required), `--label`, `--worktree-path`, `--source-repo` (optional).

## Behavior

1. On Init, loads paths from DB, bookmarks from DB, archived sessions from DB, creates unified view
2. Sessions sub-tab shows live sessions (from daemon) + saved sessions (bookmarks), with archive toggle
3. New sub-tab shows project paths + Browse option
4. Worktrees sub-tab shows all CCC-managed worktrees grouped by project
5. Enter on a live/saved/archived session resumes it (`--resume <session_id>`). For live sessions, the daemon's CCC-generated session_id differs from Claude CLI's session UUID, so the plugin resolves the real Claude session_id by scanning `~/.claude/projects/<encoded-project>/` for the most recently modified `.jsonl` file. If no file is found, falls back to the daemon session_id.

**Resolving Claude session IDs:** When the user presses `enter` on a live session, CCC resolves the Claude session UUID by finding the most recently modified JSONL file in `~/.claude/projects/<encoded-path>/`. The path is encoded by replacing all path separators (`/`) with `-` (e.g., `/Users/aaron/project` → `-Users-aaron-project`). If no JSONL file is found, the daemon session ID is used for `--resume`.
6. Enter on a project path launches Claude in that directory
7. `b` on a live session bookmarks it; on an archived session promotes it to Saved
8. `d` dismisses live ended sessions, removes bookmarks, or deletes archived sessions (tier-dependent)
9. `a` in sessions sub-tab archives the selected session (writes to `cc_archived_sessions`, removes from current view); `A` toggles between main and archive modes
10. `w` on a path in new sub-tab launches Claude in a new worktree
    - If the path is not a git repo, shows a warning overlay
11. Worktrees sub-tab scans all saved paths for git repos, lists their worktrees grouped by project
12. Delete/backspace on paths shows confirmation dialog
13. Shift+up/down swaps selected path, persisted via `sort_order` column
14. When pendingLaunchTodo is set (via event bus), shows banner "Select project for: <title>"
15. If `config.HomeDir` is set, auto-added to paths list on Init
16. `esc` from sessions/worktrees returns to new sub-tab

### LLM Path Descriptions

When a new path is added (via Browse or `config.HomeDir`), the plugin generates a project description:

1. `LLMDescribePath(llm, dir)` reads the first 200 lines of `README.md` and 100 lines of `CLAUDE.md` from the directory
2. If both files are missing/empty, falls back to `db.AutoDescribePath(dir)` (heuristic based on directory contents)
3. If files exist, builds a prompt asking for a 1-2 sentence summary (what it does, tech stack, domain) and calls `llm.Complete`
4. On LLM error or empty response, falls back to heuristic
5. The description is persisted via `db.DBUpdatePathDescription`

**Invocation paths:**
- **Browse flow (fzf):** On `fzfFinishedMsg`, writes the heuristic description immediately to DB, then fires `backgroundDescribe` in a goroutine to upgrade it via LLM. The goroutine write is fire-and-forget since the TUI is about to quit for launch.
- **pathDescribeCmd:** Returns a `tea.Cmd` wrapping `LLMDescribePath` for async use within the bubbletea loop. On completion (`pathDescribeFinishedMsg`), writes the description to DB.

### Browse Flow (New Sub-Tab)

The "Browse..." item is always the last entry in the New sub-tab list (`isBrowse: true`).

1. User selects "Browse..." and presses `enter`
2. Plugin launches `fzf` via `tea.Exec` (full-screen takeover) with:
   - `--walker=dir` — directory-only results
   - `--walker-root=$HOME` — starts from home directory
   - `--walker-skip=.git,node_modules,.venv,__pycache__,.cache,.Trash,Library`
   - `--scheme=path`, `--exact`, `--layout=reverse`
3. On `fzfFinishedMsg` (user selected a path):
   - Adds path to the in-memory paths list and persists via `db.DBAddPath`
   - Writes heuristic description immediately, fires background LLM description upgrade
   - Returns `ActionLaunch` with the selected path — the session launches immediately after browse
4. On error or empty selection (user pressed `esc` in fzf): no-op

### Daemon Archive RPC on Session Dismiss

When the user presses `d` on an **ended** live session:

1. If the session is still active/running, dismiss is blocked with flash message "Can't dismiss running session"
2. For ended sessions, the plugin calls `client.ArchiveSession(ArchiveSessionParams{SessionID: sel.SessionID})` via the daemon RPC
3. The daemon removes the session from its live list
4. The plugin also calls `unified.RemoveSession(sel.SessionID)` to remove it from the local view immediately
5. Flash message: "Dismissed: <first 8 chars of session ID>"

This is separate from auto-archiving (which writes to `cc_archived_sessions` in the DB). The daemon `ArchiveSession` RPC tells the daemon to stop tracking the session — it does NOT write to the local archive DB. Auto-archiving to DB happens during `Refresh()` when the plugin detects sessions that transitioned from live to ended.

### NavigateTo Args (pending_todo_title)

`NavigateTo(route, args)` accepts an optional `pending_todo_title` key in the `args` map:

- If `args["pending_todo_title"]` is present, sets `pendingLaunchTodo` to a `db.Todo` with that title
- This triggers the pending launch banner in the New sub-tab: "Select project for: <title>"
- When the user selects a path while `pendingLaunchTodo` is set, the launch includes `initial_prompt` with formatted todo context
- Pressing `esc` while a pending todo is active clears it, publishes `pending.todo.cancel` on the event bus, and navigates to the command-center plugin

**Full pending todo fields (via event bus):** The `pending.todo` event bus subscription populates a richer `db.Todo` with `Title`, `Context`, `Detail`, `WhoWaiting`, `Due`, and `Effort`. The `NavigateTo` args path only sets `Title`.

### Session Label Rendering

Session labels follow this fallback order:
1. **Topic** — if set via `/set-topic`, displayed as the label (e.g., "AGENT CONSOLE")
2. **Project basename** — `filepath.Base(project)` (e.g., "claude-command-center")
3. **Branch** — last resort when both topic and project are empty

For **live sessions**, the suffix shows branch in parentheses and session age: `(main)  2h ago`
For **saved sessions**, the suffix shows project basename and branch: `claude-command-center (main)`
For **archived sessions**, the suffix shows how long ago the session ended

### Blocked Session Rendering

Blocked sessions are detected by cross-referencing live sessions with daemon agent statuses:

1. On each `Refresh()`, the unified view calls `client.ListAgents()` to fetch all active `AgentStatusResult` entries
2. `isSessionBlocked(sessionID)` checks if any agent has `Status == "blocked"` and matches either `a.SessionID == sessionID` or `a.ID == sessionID`
3. Blocked sessions that are otherwise active/running render with:
   - **Yellow dot** (`●` in `#f1fa8c`) instead of the green dot for active sessions
   - **"Blocked" text** (yellow, `#f1fa8c`) prepended to the age suffix
4. Non-blocked active/running sessions render with a green dot (`●` in `#50fa7b`)
5. Ended sessions render with a muted hollow dot (`○`) regardless of block state

## Test Cases

- Init loads paths, bookmarks, and archived sessions
- Active tab shows only Live sessions in main mode (no Saved)
- Resume tab shows only Saved sessions in main mode (no Live)
- Sessions tab shows Archived section in archive mode
- Toggle archive mode resets cursor
- Deduplication: bookmarked live session shows ★, not duplicated in Saved
- Empty state shows appropriate message
- Enter on live session returns ActionLaunch with correct dir and resume_id (resolved from Claude session files, not daemon ID)
- Enter on saved session returns ActionLaunch
- Enter on archived session returns ActionLaunch
- Live session label shows project basename when topic is empty (not branch)
- Live session label shows topic when topic is set
- Live session suffix shows branch in parentheses and age
- `b` on live session saves bookmark to DB
- `b` on archived session promotes to Saved, removes from archive
- `d` on running session shows "Can't dismiss" flash
- `d` on saved session removes bookmark
- `d` on archived session deletes from DB
- Auto-archive: ended session (not bookmarked) written to cc_archived_sessions
- Auto-archive: bookmarked ended session NOT archived
- HandleKey "enter" on path sets Launch action
- HandleKey "delete" enters confirming mode
- Sub-tab switching works (s, n, t)
- Shift+up/down reorders paths
- HandleKey "w" on a git repo path sets Launch with worktree=true
- HandleKey "w" on a non-git path shows worktree warning
- Worktree confirmation y executes action, n/esc cancels
- Esc from sessions/worktrees sub-tab returns to new
- LLMDescribePath with README.md returns LLM-generated summary
- LLMDescribePath without README.md or CLAUDE.md falls back to heuristic
- LLMDescribePath on LLM error falls back to heuristic
- Browse (fzf) selection adds path to DB, writes heuristic description, fires background LLM upgrade, launches session
- Browse (fzf) cancellation (esc/error) is a no-op
- `d` on ended live session calls daemon ArchiveSession RPC and removes from view
- `d` on active/running session shows "Can't dismiss" flash
- NavigateTo with `pending_todo_title` arg sets pending launch context and shows banner
- Esc with pending todo clears it, publishes `pending.todo.cancel`, navigates to command-center
- Blocked session (agent status == "blocked") renders yellow dot and "Blocked" text
- Active non-blocked session renders green dot
- Ended session renders muted hollow dot
- NavigateTo("resume") sets subTab to "sessions" (not left unchanged)
- NavigateTo("active") sets subTab to "sessions" (not left unchanged)
- Switching from New Session tab to Resume tab renders sessions content, not project list
- Tab switching does not corrupt other tabs' content (each NavigateTo resets subTab correctly)
- `a` on ended live session archives it to DB and removes from view
- `a` on running/active live session shows "Can't archive running session" flash
- `a` on saved session archives it to DB, removes bookmark, removes from view
- `A` toggles archive mode (view archive list)
- `A` in archive mode returns to main mode
