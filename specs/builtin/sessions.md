# SPEC: Sessions Plugin (built-in)

## Purpose

Manage "New Session", "Resume Session", and "Worktrees" functionality as a plugin. Users can browse project paths, launch new Claude sessions, resume bookmarked sessions, and manage git worktrees.

## Slug: `sessions`

## Routes

- `sessions/new` — new session list (default)
- `sessions/resume` — resume tab sub-view
- `sessions/worktrees` — worktrees sub-tab

## State

- newList, resumeList (bubbles/list.Model)
- paths []string
- confirming, confirmYes bool
- confirmItem, confirmResume
- loading bool, spinner
- sub-tab: "new", "resume", or "worktrees"
- worktreeItems []worktreeItem
- worktreeCursor int
- worktreeWarning string (non-empty = show warning overlay)
- worktreeConfirmAction string ("delete" or "prune")
- worktreeConfirmTarget string

## Key Bindings

### Global (available from any sub-tab, when not in overlay)

| Key | Description | Promoted |
|-----|-------------|----------|
| n | Switch to New sub-tab | yes |
| r | Switch to Resume sub-tab | yes |
| t | Switch to Worktrees sub-tab | yes |
| esc | Quit (or back to new from worktrees) | yes |

### New sub-tab

| Key | Description | Promoted |
|-----|-------------|----------|
| enter | Launch session in selected path | yes |
| w | Launch in a new worktree (git repos only) | yes |
| shift+up/down | Reorder paths | yes |
| del/backspace | Remove from saved list (with confirmation) | yes |
| up/down | Navigate list | yes |
| / | Filter list | yes |

### Resume sub-tab

| Key | Description | Promoted |
|-----|-------------|----------|
| enter | Resume selected session | yes |
| del/backspace | Remove saved session (with confirmation) | yes |
| up/down | Navigate list | yes |
| / | Filter list | yes |

### Worktrees sub-tab

| Key | Description | Promoted |
|-----|-------------|----------|
| enter | Launch Claude in selected worktree | yes |
| d | Delete selected worktree (with confirmation) | yes |
| p | Prune all worktrees for selected project (with confirmation) | yes |
| up/down/k/j | Navigate worktree list | yes |
| esc | Back to new sub-tab | - |
| n/r | Switch to new/resume sub-tabs | - |

### Confirmation dialogs (delete path/session)

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

- **New:** `enter launch   w worktree   n new   r resume   t worktrees   shift+up/down reorder   del remove   / filter   esc quit`
- **Resume:** `enter resume   n new   r resume   t worktrees   del remove   / filter   esc quit`
- **Worktrees:** `enter launch   d delete   p prune   n new   r resume   esc back`
- **Worktree warning:** `⚠ Not a git repository — worktrees require git.` + `[enter] Launch directly in this directory   [esc] Cancel`
- **Delete confirmation:** `Delete worktree <label>?` + `[y] Yes, delete   [n] Cancel`
- **Prune confirmation:** `Remove all worktrees for <project>? (<count> worktrees)` + `[y] Yes, prune all   [n] Cancel`

## Event Bus

- Publishes: `project.selected` with {path, prompt} when user picks a project
- Publishes: `pending.todo.cancel` when user cancels a pending todo launch
- Subscribes: `pending.todo` to set a pending launch context
- Subscribes: `data.refreshed` to reload bookmarks

## Migrations

None — uses existing cc_bookmarks and cc_learned_paths tables.

## Bookmark Storage (cc_bookmarks)

Bookmarks store a pointer to a Claude Code session for later resume:

| Column | Description |
|--------|-------------|
| session_id | Claude Code session UUID (primary key) |
| project | Directory where Claude indexes the session (main repo path) |
| repo | Repository display name |
| branch | Branch name at bookmark time |
| label | User-provided label |
| summary | One-line summary of session work |
| worktree_path | Worktree directory path (NULL if not a worktree session) |
| source_repo | Main repo path for worktree sessions (NULL if not a worktree) |

### Worktree-aware bookmarks

Claude Code stores session files under `~/.claude/projects/<project-path-encoded>/`. The project path is derived from `pwd` when Claude starts. For worktree sessions, Claude maps to the **main repo's** project dir (not the worktree's), because worktrees share `.git` with the main repo.

When creating a bookmark from a worktree:
- `project` = main repo path (where Claude indexes sessions)
- `worktree_path` = the actual worktree directory
- `source_repo` = main repo path

When resuming a worktree bookmark:
- `cd` to `project` (main repo) so `claude --resume <id>` finds the session
- The resumed conversation already has full worktree context from the previous session

### CLI: `ccc add-bookmark`

Flags: `--session-id`, `--project`, `--repo`, `--branch`, `--summary` (required), `--label`, `--worktree-path`, `--source-repo` (optional).

## Behavior

1. On Init, loads paths from DB and sessions from DB
2. New sub-tab shows project paths + Browse option
3. Resume sub-tab shows bookmarked sessions
4. Worktrees sub-tab shows all CCC-managed worktrees grouped by project
5. Enter on a path launches Claude in that directory
6. Enter on a session resumes that Claude session (uses `project` as the working dir, `--resume <session_id>` flag)
7. `w` on a path in the new sub-tab launches Claude in a new worktree for that project
   - If the path is not a git repo, shows a warning overlay
   - Warning overlay: enter launches directly (no worktree), esc cancels
8. Worktrees sub-tab scans all saved paths for git repos, lists their worktrees grouped by project basename
   - Each worktree shows branch name and age (human-readable time since creation)
   - Cursor navigation (no bubbles/list — manual cursor with up/down/j/k)
9. Delete/backspace on paths or sessions shows confirmation dialog (all path entries are deletable)
10. `d` on a worktree shows delete confirmation (y/n), removes the worktree via `git worktree remove`
11. `p` on a worktree shows prune confirmation (y/n), removes all worktrees for that project
12. Shift+up/down swaps the selected path with its neighbor, persisted via `sort_order` column in `cc_learned_paths`
13. When pendingLaunchTodo is set (via event bus), shows banner "Select project for: <title>"
14. If `config.HomeDir` is set, it is auto-added to the paths list on Init (treated as a regular path, no special styling)
15. `esc` from worktrees sub-tab returns to new sub-tab (not quit)

## Test Cases

- Init loads paths and sessions
- HandleKey "enter" on path sets Launch action
- HandleKey "enter" on session sets Launch with resume args (dir=project, --resume flag)
- HandleKey "enter" on worktree bookmark uses main repo as dir (not worktree path)
- HandleKey "delete" enters confirming mode
- Confirming "y" removes item
- Sub-tab switching works (n, r, t)
- Delete on first path enters confirming mode (all paths deletable)
- Shift+up/down reorders paths in-memory and persists via DB
- HandleKey "w" on a git repo path sets Launch with worktree=true
- HandleKey "w" on a non-git path shows worktree warning
- Worktree warning: enter launches directly, esc cancels
- Worktrees sub-tab lists worktrees grouped by project
- HandleKey "d" in worktrees shows delete confirmation
- HandleKey "p" in worktrees shows prune confirmation
- Worktree confirm "y" executes action, "n"/esc cancels
- Esc from worktrees sub-tab returns to new sub-tab
