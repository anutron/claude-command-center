# Error Handling Audit

## Summary

The codebase has generally good error handling in critical paths (database writes, config loading, external plugin lifecycle) but has a pattern of silently swallowing errors in migration code, JSON marshaling, socket operations, and several `os.UserHomeDir()` calls. There are no panics anywhere in the codebase and no TODO/FIXME/HACK comments in Go files.

## Swallowed Errors

| File:Line | Code | Risk |
|---|---|---|
| `internal/db/db.go:133` | `_, _ = db.Exec("ALTER TABLE cc_calendar_cache ADD COLUMN calendar_id ...")` | **Low** — Intentional: column may already exist. But logs nothing on unexpected errors (e.g., disk full). |
| `internal/db/db.go:309` | `_ = json.Unmarshal([]byte(rankedJSON.String), &s.RankedTodoIDs)` | **Medium** — Corrupted JSON in suggestions silently produces empty ranked list. |
| `internal/db/db.go:312` | `_ = json.Unmarshal([]byte(reasonsJSON.String), &s.Reasons)` | **Medium** — Same as above for reasons map. |
| `internal/db/db.go:702` | `rankedJSON, _ := json.Marshal(cc.Suggestions.RankedTodoIDs)` | **Low** — Marshal of `[]string` is unlikely to fail. |
| `internal/db/db.go:703` | `reasonsJSON, _ := json.Marshal(cc.Suggestions.Reasons)` | **Low** — Marshal of `map[string]string` is unlikely to fail. |
| `internal/db/db.go:735` | `rankedJSON, _ := json.Marshal(s.RankedTodoIDs)` | **Low** — Same pattern in `DBSaveSuggestions`. |
| `internal/db/db.go:736` | `reasonsJSON, _ := json.Marshal(s.Reasons)` | **Low** — Same. |
| `internal/db/migrate.go:120-124` | `migrateCachedEvent(tx, "today", ev)` — return value ignored | **Medium** — If calendar event insertion fails during migration, it is silently dropped. `migrateCachedEvent` itself uses `_, _` for the exec result. |
| `internal/db/migrate.go:128-132` | `rankedJSON, _ := json.Marshal(...)` then `_, _ = tx.Exec(...)` | **Medium** — Suggestions migration errors silently swallowed inside a transaction. |
| `internal/db/migrate.go:137-139` | `_, _ = tx.Exec(...)` for pending actions | **Medium** — Same pattern. |
| `internal/db/migrate.go:144-146` | `_, _ = tx.Exec(...)` for generated_at | **Medium** — Same pattern. |
| `internal/db/migrate.go:153-155` | `_, _ = tx.Exec(...)` in `migrateCachedEvent` | **Medium** — Calendar event insertion error swallowed. |
| `internal/tui/notify.go:20-21` | `home, _ := os.UserHomeDir()` in `SocketPath()` | **Medium** — If home dir lookup fails, socket path becomes `/.config/ccc/data/ccc-PID.sock`, likely causing silent failure to create socket. |
| `internal/tui/notify.go:30` | `home, _ := os.UserHomeDir()` in `sockDir()` | **Medium** — Same issue. |
| `internal/tui/notify.go:41` | `_ = os.MkdirAll(filepath.Dir(sockPath), 0o755)` | **Low** — If mkdir fails, the next `net.Listen` will also fail and is handled. |
| `internal/tui/notify.go:44` | `_ = os.Remove(sockPath)` | **Low** — Intentional cleanup of stale socket. |
| `internal/config/config.go:90` | `home, _ := os.UserHomeDir()` in `ConfigDir()` | **High** — Used everywhere for config/data/DB paths. If this fails, all paths are wrong and the app silently misbehaves. |
| `internal/refresh/auth.go:52` | `home, _ := os.UserHomeDir()` in `loadCalendarAuth()` | **Medium** — Bad home dir leads to confusing "file not found" errors. |
| `internal/refresh/auth.go:92` | `home, _ := os.UserHomeDir()` in `loadGmailAuth()` | **Medium** — Same. |
| `internal/refresh/auth.go:140` | `home, _ := os.UserHomeDir()` in `loadGranolaAuth()` | **Medium** — Same. |
| `internal/refresh/auth.go:203` | `home, _ := os.UserHomeDir()` in `RunCalendarAuth()` | **Medium** — Same. |
| `internal/refresh/auth.go:226` | `_ = exec.Command("open", url).Run()` | **Low** — Best-effort browser open on macOS. Acceptable. |
| `internal/refresh/auth.go:264` | `os.MkdirAll(dir, 0o755)` — return value not captured | **Medium** — If directory creation fails, the subsequent `os.WriteFile` will also fail, but the error message will be about the write, not the mkdir. |
| `internal/refresh/auth.go:273` | `data, _ := json.MarshalIndent(creds, "", "  ")` | **Low** — Simple struct marshal unlikely to fail. |
| `internal/refresh/auth.go:279` | `home, _ := os.UserHomeDir()` in `loadEnvFile()` | **Low** — Non-critical helper. |
| `internal/refresh/auth.go:303` | `home, _ := os.UserHomeDir()` in `loadCalendarCredsFromClaudeConfig()` | **Low** — Returns empty strings on any failure. |
| `internal/refresh/auth.go:325` | `home, _ := os.UserHomeDir()` in `migrateCalendarCredentials()` | **Medium** — Same pattern. |
| `internal/refresh/actions.go:100` | `eventStart, _ := time.Parse(time.RFC3339, item.Start.DateTime)` | **High** — If parse fails, `eventStart` is zero time, which silently corrupts slot-finding logic, potentially double-booking. |
| `internal/refresh/actions.go:106` | `eventEnd, _ := time.Parse(time.RFC3339, item.End.DateTime)` | **High** — Same: zero time could cause the candidate to never advance past this event. |
| `internal/refresh/llm.go:90` | `state, _ := json.Marshal(struct{...}{...})` | **Low** — Marshal of known struct unlikely to fail. |
| `internal/refresh/refresh.go:115` | `data, _ := json.MarshalIndent(merged, "", "  ")` | **Low** — Only used in dry-run mode for debug output. |
| `internal/external/external.go:144` | `_ = ep.proc.Send(HostMsg{Type: "shutdown"})` | **Low** — Best-effort shutdown message. Process will be killed on timeout anyway. |
| `internal/external/external.go:274` | `_ = ep.proc.Send(HostMsg{Type: "refresh"})` | **Low** — Non-critical refresh hint. |
| `internal/external/external.go:283` | `_ = ep.proc.Send(HostMsg{Type: "navigate", ...})` | **Low** — Non-critical navigation message. |
| `internal/config/setup.go:201` | `line, _ := reader.ReadString('\n')` | **Low** — Interactive input; EOF is handled by returning empty string. |
| `internal/refresh/gmail.go:58-59` | `if err != nil { continue }` for individual email fetch | **Low** — Intentional: skip individual broken emails, continue with rest. |
| `internal/refresh/granola.go:71-72` | `if err != nil { continue }` for individual transcript fetch | **Low** — Same pattern: skip broken transcripts. |
| `internal/refresh/github.go:65-66` | `if err != nil { continue }` for individual repo PR fetch | **Medium** — Silently skips entire repos that fail. A single auth issue or rate limit could cause all repos to silently return nothing. |

## Inconsistent Patterns

### Error wrapping style varies within the same file

In `internal/refresh/slack.go`, some errors are wrapped with context and some are not:
- Line 32: `fmt.Errorf("slack auth: %w", err)` (wrapped)
- Line 106: `return nil, err` (bare)
- Line 112: `return nil, err` (bare)
- Line 118: `return nil, err` (bare)

In `internal/refresh/calendar.go`:
- Line 38: `fmt.Errorf("calendar auth: %w", err)` (wrapped)
- Line 67: `return nil, err` (bare)
- Line 108: `return nil, err` (bare)

In `internal/refresh/granola.go`:
- Line 38: `fmt.Errorf("granola auth: %w", err)` (wrapped)
- Line 66: `return nil, err` (bare)
- Line 83-96: Multiple `return nil, err` (bare)

Pattern: Top-level `Fetch()` methods wrap errors, but internal helper functions (`fetchSlackCandidates`, `listEvents`, `granolaPost`) often return bare errors. This makes it harder to trace which subsystem failed when errors bubble up.

### `return err` vs `return fmt.Errorf` in db.go

`internal/db/db.go` has ~30 bare `return err` instances in write methods (e.g., `DBCompleteTodo`, `DBDismissTodo`, `DBDeferTodo`) but wraps errors in `DBSaveRefreshResult`. The write methods that users interact with through the TUI return unhelpful bare SQL errors.

### Inconsistent treatment of `json.Marshal` errors

- In `DBSaveRefreshResult` (db.go:702-703): `_, _` for marshal
- In `DBSaveSuggestions` (db.go:735-736): `_, _` for marshal
- In `generateSuggestions` (llm.go:90): `_, _` for marshal
- But in `external.go:59-63`: Marshal error is checked and returned with context

## Missing Error Context

These locations return bare errors without wrapping, making debugging harder:

| File:Line | Function | Bare Return |
|---|---|---|
| `internal/db/db.go:22` | `OpenDB` | `return nil, err` (sql.Open failure) |
| `internal/db/db.go:188` | `dbLoadTodos` | `return nil, err` (Query failure) |
| `internal/db/db.go:203` | `dbLoadTodos` | `return nil, err` (Scan failure) |
| `internal/db/db.go:228` | `dbLoadThreads` | `return nil, err` (Query failure) |
| `internal/db/db.go:242` | `dbLoadThreads` | `return nil, err` (Scan failure) |
| `internal/db/db.go:358` | `DBCompleteTodo` | `return err` |
| `internal/db/db.go:364` | `DBDismissTodo` | `return err` |
| `internal/db/db.go:376` | `DBRestoreTodo` | `return err` |
| `internal/db/db.go:383` | `DBDeferTodo` | `return err` |
| `internal/db/db.go:390` | `DBPromoteTodo` | `return err` |
| `internal/db/db.go:413` | `DBInsertTodo` | `return err` |
| `internal/db/db.go:430` | `DBUpdateTodo` | `return err` |
| `internal/db/db.go:488` | `DBPauseThread` | `return err` |
| `internal/db/db.go:495` | `DBStartThread` | `return err` |
| `internal/db/db.go:502` | `DBCloseThread` | `return err` |
| `internal/db/db.go:517` | `DBInsertThread` | `return err` |
| `internal/db/db.go:567` | `DBInsertBookmark` | `return err` |
| `internal/db/db.go:572` | `DBRemoveBookmark` | `return err` |
| `internal/db/db.go:603` | `DBAddPath` | `return err` |
| `internal/db/db.go:608` | `DBRemovePath` | `return err` |
| `internal/db/migrate.go:51-61` | `migrateCommandCenter` | Multiple bare returns |
| `internal/db/migrate.go:164-193` | `migrateBookmarks`, `migrateLearnedPaths` | Multiple bare returns |
| `internal/refresh/slack.go:105-118` | `fetchSlackCandidates` | Three consecutive bare `return nil, err` |
| `internal/refresh/granola.go:66-96` | `fetchGranolaMeetings`, `granolaPost` | Multiple bare returns |
| `internal/refresh/calendar.go:67,108` | `fetchCalendarEvents`, `listEvents` | Bare returns |
| `internal/refresh/gmail.go:39,48` | `fetchActionableEmails` | Bare returns |
| `internal/config/config.go:140,145` | `Save` | Bare returns |
| `internal/llm/claude_cli.go:27` | `Complete` | `return "", err` (non-ExitError case) |

## Panics

No `panic()` calls found anywhere in the codebase. This is good.

## Goroutine Error Handling

| File:Line | Pattern | Assessment |
|---|---|---|
| `internal/refresh/refresh.go:60` | `go func(s DataSource)` — errors logged and collected as warnings | **Good** — Errors are collected via mutex and stored as warnings. |
| `cmd/ccc/main.go:123` | `go func()` — signal handler | **OK** — No errors to propagate, just cleanup and exit. |
| `internal/tui/notify.go:51` | `go func()` — accept loop | **OK** — Errors from `ln.Accept()` cause the goroutine to return (listener was closed). |
| `internal/refresh/auth.go:245` | `go srv.ListenAndServe()` | **Medium** — `ListenAndServe` error (e.g., port already in use) is silently lost. The select will eventually timeout, but the user gets a misleading "timeout" error instead of "port 3000 in use". |

## TODOs/FIXMEs/HACKs

No TODO, FIXME, or HACK comments found in Go source files. The only "TODO" references are in the application's domain (the todo-management feature itself).

## Recommendations

1. **Fix `os.UserHomeDir()` in `config.ConfigDir()`** (highest impact). This single function is the root of all path computation. If it fails, the entire app silently uses wrong paths. Change to return an error or call it once at startup and propagate.

2. **Check time.Parse in `findFreeSlot`** (`internal/refresh/actions.go:100,106`). Parse failures from the Google Calendar API silently corrupt the slot-finding algorithm. At minimum, `continue` past events with unparseable times.

3. **Wrap bare `return err` in `internal/db/db.go` write methods.** Add function-name context (e.g., `fmt.Errorf("complete todo %s: %w", id, err)`). These are user-facing operations where a bare SQLite error is not helpful.

4. **Check `json.Unmarshal` in `dbLoadSuggestions`** (`internal/db/db.go:309,312`). Corrupted suggestion data should either be logged or cause a reload, not silently produce empty results.

5. **Log errors in migration helpers** (`internal/db/migrate.go:120-155`). The `_, _` pattern in `migrateCachedEvent` and the suggestions/pending-actions migration means data loss during migration goes unnoticed. These should at minimum log warnings.

6. **Capture `ListenAndServe` error** in `RunCalendarAuth` (`internal/refresh/auth.go:245`). Send the error to `errCh` so the user sees "port 3000 in use" instead of waiting 5 minutes for a timeout.

7. **Add error context in refresh helper functions.** Functions like `fetchSlackCandidates`, `granolaPost`, and `fetchCalendarEvents` return bare errors from HTTP calls and JSON parsing. Wrapping with the function/operation name would aid debugging.

8. **Consider logging when GitHub repo fetch silently fails** (`internal/refresh/github.go:65-66`). A `continue` with no logging means auth failures or rate limits cause silent data loss.
