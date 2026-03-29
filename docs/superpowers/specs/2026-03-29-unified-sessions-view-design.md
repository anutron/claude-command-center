# Design: Unified Sessions View

## Purpose

Combine the "Active Sessions" and "Resume Session" (bookmarks) sub-tabs into a single unified sessions view with three tiers: Live, Saved, and Archived. Reduces tab clutter while keeping all session management in one place. Archived sessions are hidden by default and accessible via a toggle hotkey.

## Context

The sessions plugin currently has four sub-tabs: active (live daemon sessions), new (project launcher), resume (bookmarked sessions), and worktrees. Active and resume serve the same purpose — "find a session and resume it" — but split across two tabs. Users must switch tabs to see live vs saved sessions, and there's no persistence for ended sessions that weren't explicitly bookmarked.

## Design

### Three-Tier Session Model

Sessions exist in one of three tiers, displayed in priority order:

| Tier | Source | Visibility | Description |
|------|--------|------------|-------------|
| **Live** | Daemon RPC (`ListSessions`) | Always visible (main mode) | Running, active, or blocked sessions |
| **Saved** | `cc_bookmarks` table | Always visible (main mode) | User-bookmarked sessions, including ended-but-bookmarked |
| **Archived** | `cc_archived_sessions` table | Hidden (archive mode) | Auto-saved ended sessions that weren't bookmarked |

### Session Lifecycle

1. Session registers with daemon → appears in **Live**
2. User presses `b` on a live session → also saved to `cc_bookmarks`
3. Session ends in daemon:
   - If bookmarked → moves to **Saved** (promoted)
   - If not bookmarked → auto-persisted to **Archived**
4. `d` on archived → permanently deletes
5. `d` on saved → removes bookmark (drops to archived if still in daemon history, otherwise deleted)

### Deduplication

A session that is both live in the daemon AND bookmarked appears in **Live** only, with a bookmark indicator (`★`). When it ends, it moves to **Saved**.

### View Modes

Two modes toggled by `a`:

| Mode | Contents | Default |
|------|----------|---------|
| **Main** | Live section + Saved section | Yes |
| **Archive** | Archived sessions only | No |

### Layout (Main Mode)

```
  LIVE
  ● Fixing the bug              2m ago
  ★ Building UI                10m ago    ← bookmarked indicator

  SAVED
  project-a (main)             Mar 28
  project-b (feature-x)       Mar 25
```

### Layout (Archive Mode)

```
  ARCHIVED
  ○ Feature work                1h ago
  ○ Quick fix                   3h ago
```

### Navigation

Single cursor across all items in current mode. Section headers are visual-only, not selectable.

#### Key Bindings (Main Mode)

| Key | Action |
|-----|--------|
| `j`/`k` | Navigate up/down |
| `enter` | Resume session (launches Claude Code with `--resume` in project dir) |
| `b` | Bookmark live session → persists to Saved |
| `d` | On saved: remove bookmark. On live: no-op |
| `a` | Switch to archive view |
| `/` | Type-to-filter (topic, project, branch across both sections) |

#### Key Bindings (Archive Mode)

| Key | Action |
|-----|--------|
| `j`/`k` | Navigate |
| `enter` | Resume archived session |
| `b` | Promote to Saved (bookmark it) |
| `d` | Permanently delete |
| `a` | Return to main view |
| `/` | Type-to-filter |

### Storage

#### New Table: `cc_archived_sessions`

```sql
CREATE TABLE IF NOT EXISTS cc_archived_sessions (
    session_id TEXT PRIMARY KEY,
    topic TEXT,
    project TEXT,
    repo TEXT,
    branch TEXT,
    worktree_path TEXT,
    registered_at TEXT NOT NULL,
    ended_at TEXT NOT NULL
);
```

Separate from `cc_bookmarks` because archives are automatic and disposable; bookmarks are user-curated and intentional. Different retention policies warrant different tables.

#### Write Path

When `Refresh()` polls the daemon, it compares the current session list against the previous snapshot (already stored in `av.sessions`). Sessions present in the previous snapshot but absent or ended in the current one are "newly ended." For each newly ended session that isn't already in `cc_bookmarks`, insert into `cc_archived_sessions`. If the daemon is disconnected, no archiving occurs (we can't distinguish "ended" from "daemon down").

#### Read Path

- **Main mode:** daemon `ListSessions()` (filter running/active/blocked) + `DBLoadBookmarks()`
- **Archive mode:** `DBLoadArchivedSessions()` (new query, ordered by `ended_at DESC`)

#### Cleanup

No automatic pruning initially. `d` is manual deletion. Future consideration: age-based rotation (e.g., 30 days).

### Route & Sub-Tab Changes

#### Removed Routes

| Old Slug | Old Hotkey | Disposition |
|----------|-----------|-------------|
| `active` | `a` | Merged into `sessions` |
| `resume` | `r` | Merged into `sessions` |

#### New Routes

| Slug | Hotkey | Description |
|------|--------|-------------|
| `sessions` | `s` | Unified view (default tab) |
| `new` | `n` | Unchanged |
| `worktrees` | `t` | Unchanged |

- `a` repurposed as archive toggle within the sessions view
- `r` freed entirely
- `s` is the new sub-tab hotkey for sessions
- Default landing tab: `sessions`

## Test Cases

### Tier Assignment

- Running daemon session appears in Live section
- Bookmarked session with no daemon presence appears in Saved section
- Ended daemon session (not bookmarked) auto-archives to `cc_archived_sessions`
- Ended daemon session (bookmarked) appears in Saved, not Archived
- Live session that is also bookmarked appears in Live with `★`, not duplicated in Saved

### Mode Toggle

- `a` switches from main to archive view
- `a` switches from archive back to main view
- Cursor resets to 0 on mode switch
- Archive mode shows only archived sessions
- Main mode shows only live + saved sessions

### Key Actions

- `enter` on live session launches `--resume` with correct project dir (or worktree path)
- `enter` on saved session launches `--resume` with correct session ID and project
- `enter` on archived session launches `--resume`
- `b` on live session inserts into `cc_bookmarks`, shows `★` indicator
- `b` on archived session promotes to `cc_bookmarks`, removes from `cc_archived_sessions`
- `d` on saved session removes from `cc_bookmarks`
- `d` on live session is no-op (flash: "Can't dismiss running session")
- `d` on archived session deletes from `cc_archived_sessions`
- `/` filters across both sections in main mode
- `/` filters archived sessions in archive mode

### Persistence

- Ended sessions auto-persist to `cc_archived_sessions` on next daemon poll
- Sessions already in `cc_bookmarks` are not duplicated in `cc_archived_sessions`
- Daemon restart doesn't lose ended sessions (they're in the archive table)
- TUI restart reloads both bookmarks and archives from DB

### Route Changes

- `s` hotkey navigates to sessions sub-tab
- `a` hotkey within sessions toggles archive (does not switch sub-tab)
- `n` and `t` hotkeys unchanged
- Old `r` hotkey does not navigate to removed resume tab
- Default landing tab is `sessions`
