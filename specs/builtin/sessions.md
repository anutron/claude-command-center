# SPEC: Sessions Plugin (built-in)

## Purpose

Manage sessions, project launching, and worktrees as a plugin. Users can browse active/saved/archived sessions, launch new Claude sessions from project paths, and manage git worktrees.

## Slug: `sessions`

## Routes

- `sessions/sessions` — unified sessions view (default)
- `sessions/new` — new session list
- `sessions/worktrees` — worktrees sub-tab

## State

- unified *unifiedView (manages live, saved, archived sessions)
- newList (bubbles/list.Model)
- paths []string
- confirming, confirmYes bool
- confirmItem
- sub-tab: "sessions", "new", or "worktrees"
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
| **Live** | Daemon RPC | Always (main mode) | Running, active, or blocked sessions |
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

Two modes toggled by `a`:

| Mode | Contents | Default |
|------|----------|---------|
| **Main** | Live + Saved sections | Yes |
| **Archive** | Archived sessions only | No |

### Auto-Archiving

When `Refresh()` polls the daemon, it compares the current session list against the previous snapshot. Sessions that were previously running but are now ended (and not bookmarked) are auto-archived to `cc_archived_sessions`. If the daemon is disconnected, no archiving occurs.

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
| a | Toggle archive mode | yes |
| j/k or up/down | Navigate list | yes |

### Sessions sub-tab (archive mode)

| Key | Description | Promoted |
|-----|-------------|----------|
| enter | Resume archived session | yes |
| b | Promote to Saved (bookmark) | yes |
| d | Permanently delete | yes |
| a | Return to main mode | yes |
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

- **Sessions (main):** `enter resume   b bookmark   d dismiss   j/k navigate   a archive   s sessions   n new   t worktrees`
- **Sessions (archive):** `enter resume   b save   d delete   j/k navigate   a back   s sessions   n new   t worktrees`
- **New:** `type to filter   enter launch   w worktree   s sessions   n new   t worktrees   shift+up/down reorder   del remove   esc quit`
- **Worktrees:** `enter launch   d delete   p prune   s sessions   n new   esc back`
- **Worktree warning:** `⚠ Not a git repository — worktrees require git.` + `[enter] Launch directly in this directory   [esc] Cancel`
- **Delete confirmation:** `Delete worktree <label>?` + `[y] Yes, delete   [n] Cancel`
- **Prune confirmation:** `Remove all worktrees for <project>? (<count> worktrees)` + `[y] Yes, prune all   [n] Cancel`

## Event Bus

- Publishes: `project.selected` with {path, prompt} when user picks a project
- Publishes: `pending.todo.cancel` when user cancels a pending todo launch
- Subscribes: `pending.todo` to set a pending launch context
- Subscribes: `data.refreshed` to reload bookmarks and archived sessions
- Subscribes: `session.registered`, `session.updated`, `session.ended` to refresh live sessions

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
5. Enter on a live/saved/archived session resumes it (`--resume <session_id>`)
6. Enter on a project path launches Claude in that directory
7. `b` on a live session bookmarks it; on an archived session promotes it to Saved
8. `d` dismisses live ended sessions, removes bookmarks, or deletes archived sessions (tier-dependent)
9. `a` in sessions sub-tab toggles between main and archive modes
10. `w` on a path in new sub-tab launches Claude in a new worktree
    - If the path is not a git repo, shows a warning overlay
11. Worktrees sub-tab scans all saved paths for git repos, lists their worktrees grouped by project
12. Delete/backspace on paths shows confirmation dialog
13. Shift+up/down swaps selected path, persisted via `sort_order` column
14. When pendingLaunchTodo is set (via event bus), shows banner "Select project for: <title>"
15. If `config.HomeDir` is set, auto-added to paths list on Init
16. `esc` from sessions/worktrees returns to new sub-tab

## Test Cases

- Init loads paths, bookmarks, and archived sessions
- Sessions tab shows Live and Saved sections in main mode
- Sessions tab shows Archived section in archive mode
- Toggle archive mode resets cursor
- Deduplication: bookmarked live session shows ★, not duplicated in Saved
- Empty state shows appropriate message
- Enter on live session returns ActionLaunch with correct dir and resume_id
- Enter on saved session returns ActionLaunch
- Enter on archived session returns ActionLaunch
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
