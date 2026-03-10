# API & Interface Design Audit

## Summary

The codebase has a clean plugin architecture with well-separated concerns, but suffers from three systemic issues: heavy use of `interface{}`/`any` to work around circular imports, severe parameter bloat in view-rendering functions, and duplicated style/constant definitions across every plugin.

## Interface Analysis

| Interface | Location | Methods | Verdict |
|-----------|----------|---------|---------|
| `Plugin` | `internal/plugin/plugin.go:58` | 13 | **Bloated.** Every plugin must implement 13 methods even if most are no-ops (e.g., `Migrations()`, `Routes()`, `Shutdown()`). Go-idiomatic would be a smaller core interface + optional interfaces (like `Starter` already is). `Migrations`, `Routes`, `Shutdown`, `RefreshInterval`, `Refresh`, and `KeyBindings` are all candidates for optional interfaces. |
| `EventBus` | `internal/plugin/eventbus.go:13` | 2 | **Good.** Minimal, consumed where defined (plugin package). |
| `Logger` | `internal/plugin/logger.go:12` | 4 | **Acceptable.** `Recent(n)` couples it to the settings UI. Consider splitting into `Logger` (3 methods) and `LogReader` (1 method) to let most consumers depend on less. |
| `LLM` | `internal/llm/llm.go:9` | 1 | **Good.** Single-method interface, idiomatic Go. |
| `DataSource` | `internal/refresh/datasource.go:11` | 3 | **Good.** Minimal and focused. Defined where consumed (refresh package). |
| `Starter` | `internal/plugin/plugin.go:89` | 1 | **Good.** Single-method optional interface. |

### Interface Location

All interfaces are defined in the `plugin`, `llm`, or `refresh` packages -- generally close to where they are consumed rather than implemented. This follows Go best practices.

**Exception:** `LLM` is defined in the `llm` package alongside its implementations (`ClaudeCLI`, `NoopLLM`). Since consumers are in `refresh` and `commandcenter`, a Go-idiomatic approach would define `LLM` at the consumer. However, with only one interface method and two implementations, this is low-severity.

## Over-exported API

### `internal/db/types.go` -- Utility functions that belong unexported or in a separate package

| Export | Line | Issue |
|--------|------|-------|
| `GenID()` | 176 | Generic utility, only used within db package and by plugins. Name is too terse for an exported function. |
| `RelativeTime()` | 186 | Time-formatting helper used only by view code. Should be unexported or in a `timeutil` package. |
| `DueUrgency()` | 201 | Used only by view rendering. Same as above. |
| `FormatDueLabel()` | 222 | Used only by view rendering. |
| `LoadPaths()`, `SavePaths()`, `AddPath()`, `RemovePath()` | 440-502 | File-based CRUD for learned paths. These are legacy -- the DB equivalents (`DBLoadPaths`, `DBAddPath`, `DBRemovePath`) now exist. The file-based versions appear unused in the main code path. |
| `ParseSessionFile()`, `LoadWinddownSessions()`, `LoadBookmarks()`, `LoadAllSessions()`, `RemoveBookmark()` | 508-666 | File-based session loading. Like paths, these appear to be legacy now that bookmarks are DB-backed. |

### `internal/db/db.go` -- DB function naming prefix

All exported DB functions use a `DB` prefix (e.g., `DBCompleteTodo`, `DBLoadBookmarks`). Since they're already in the `db` package, callers write `db.DBCompleteTodo(...)` -- the `DB` prefix is redundant. Go convention would be `db.CompleteTodo(...)`.

**Exception:** `LoadCommandCenterFromDB` does NOT use the `DB` prefix, making the naming inconsistent with the rest of the file.

### `internal/builtin/commandcenter/commandcenter.go`

| Export | Line | Issue |
|--------|------|-------|
| `SubView()` | 1323 | Getter only used by tests. Could be unexported. |
| `SetSubView()` | 1328 | Setter only used by tests. Could be unexported. |
| `SetPendingLaunchTodo()` | 382 | Only used from within the plugin (via event bus). Could be unexported. |
| `PendingLaunchTodo()` | 375 | Same pattern -- only internal consumers. |

### `internal/tui/styles.go`

`Styles` and `GradientColors` are exported types but are only consumed within `tui` and passed to plugins via `interface{}` fields. They don't need to be exported since they're never imported by name outside `tui`.

## Parameter Bloat

These functions have excessive parameter counts and should use config/options structs:

| Function | Location | Params | Severity |
|----------|----------|--------|----------|
| `renderCommandCenterView()` | `cc_view.go:31` | **14 params** | Critical. This is the worst offender. |
| `renderExpandedTodoView()` | `cc_view.go:683` | **12 params** | Critical. |
| `renderTodoPanel()` | `cc_view.go:324` | **10 params** | High. |
| `renderExpandedTodoItem()` | `cc_view.go:606` | **8 params** | High. |
| `renderThreadSection()` | `threads_view.go:35` | **8 params** | High. |
| `renderCCFooter()` | `cc_view.go:562` | **6 params** | Moderate. |
| `renderCalendarColumn()` | `cc_view.go:104` | **5 params** | Moderate. |
| `renderCalendarPanelCapped()` | `cc_view.go:167` | **6 params** | Moderate. |
| `renderCalendarPanel()` | `cc_view.go:241` | **5 params** | Moderate. |
| `NewModel()` | `tui/model.go:61` | **5 + variadic** | Moderate. |

**Suggested pattern:** A `renderContext` struct would eliminate most of these:

```go
type renderCtx struct {
    styles    *ccStyles
    grad      *gradientColors
    width     int
    height    int
    frame     int
    refreshing bool
    loadingTodoID string
}
```

## Type Design Issues

### `plugin.Context` uses `interface{}` to avoid circular imports (line 15-20)

```go
type Context struct {
    Styles interface{} // *tui.Styles
    Grad   interface{} // *tui.GradientColors
    LLM    interface{} // llm.LLM
}
```

This forces every consumer to type-assert, losing compile-time safety. Three fields are `interface{}`. The `LLM` field is especially avoidable since `llm.LLM` is already an interface -- the plugin package could import `llm` without circularity (it's a leaf package). `Styles` and `Grad` require a design change: either extract a styles interface, or have plugins receive palette info and construct their own styles (which they already do -- each plugin calls `config.GetPalette()` and builds local styles, making the `Styles`/`Grad` context fields unused in practice).

### `plugin.Event.Payload` is `map[string]interface{}`

`internal/plugin/eventbus.go:9`: Event payloads are untyped maps. This means all event consumers must do defensive type assertions (see `sessions.go:330-343` where every field is individually asserted). A typed event payload per topic, or at minimum a `Payload` struct with known fields, would catch errors at compile time.

### `plugin.Action.Type` is a stringly-typed enum

`internal/plugin/plugin.go:25`: Action types are raw strings (`"noop"`, `"open_url"`, `"flash"`, `"launch"`, `"quit"`, `"navigate"`, `"unhandled"`). These should be constants.

### `commandcenter.Plugin` struct is very large (30+ fields)

`internal/builtin/commandcenter/commandcenter.go:56-125`: The Plugin struct has 30+ fields spanning 6 logical groups (core deps, CC state, thread state, input modes, background processing, UI state). This suggests decomposition into sub-structs:

- `inputState` (addingThread, bookingMode, bookingCursor, textInput, addingTodoRich, todoTextArea, commandConversation)
- `claudeState` (claudeLoading, claudeLoadingMsg, claudeLoadingTodo)
- `refreshState` (ccRefreshing, ccLastRefreshTriggered, lastRefreshAt, lastRefreshError)
- `viewState` (width, height, frame, subView, showHelp, flashMessage, flashMessageAt)

### `SourceResult` is a union struct

`internal/refresh/datasource.go:22-27`: Each DataSource populates only some fields (calendar source sets `Calendar`, GitHub sets `Threads`, etc.), leaving the rest nil. This is fine for now but will scale poorly as more sources are added.

## Naming Inconsistencies

### DB function prefix: `DB` vs. no prefix

Exported functions in `internal/db/db.go` inconsistently use a `DB` prefix:
- **With prefix:** `DBCompleteTodo`, `DBDismissTodo`, `DBRestoreTodo`, `DBDeferTodo`, `DBPromoteTodo`, `DBInsertTodo`, `DBUpdateTodo`, `DBReplaceCalendar`, `DBSaveFocus`, `DBPauseThread`, `DBStartThread`, `DBCloseThread`, `DBInsertThread`, `DBInsertPendingAction`, `DBLoadBookmarks`, `DBInsertBookmark`, `DBRemoveBookmark`, `DBLoadPaths`, `DBAddPath`, `DBRemovePath`, `DBSaveRefreshResult`, `DBSaveSuggestions`, `DBSetMeta`, `DBClearPendingActions`, `DBIsEmpty`
- **Without prefix:** `LoadCommandCenterFromDB`, `OpenDB`, `FormatTime`, `ParseTime`

The `DB` prefix is redundant (package already named `db`) and `LoadCommandCenterFromDB` breaks the convention used by every other function.

### `contentMaxWidth` duplicated in 4 packages

The constant `contentMaxWidth = 120` is independently declared in:
- `internal/tui/styles.go:8`
- `internal/builtin/commandcenter/commandcenter.go:23`
- `internal/builtin/sessions/sessions.go:30`
- `internal/builtin/settings/settings.go:18`

This should be a single exported constant (e.g., in `config` or `plugin`).

### Style struct naming varies by plugin

Each plugin defines its own style struct with different naming conventions:
- `tui.Styles` (exported)
- `commandcenter.ccStyles` (unexported, `cc` prefix)
- `sessions.sessionStyles` (unexported, `session` prefix)
- `settings.settingsStyles` (unexported, `settings` prefix)
- `commandcenter.gradientColors` vs `sessions.gradientColors` (same name, different packages)

While local styles are fine architecturally (each plugin owns its look), the inconsistent naming adds cognitive load.

### Receiver naming

DB functions use inconsistent receiver variable names:
- Most use `db *sql.DB` as the parameter name, shadowing the package name
- `DBSaveRefreshResult` and `DBSaveSuggestions` use `d *sql.DB` instead

### `ccRefreshInterval` is a package-level mutable variable

`internal/builtin/commandcenter/commandcenter.go` uses `ccRefreshInterval` as a mutable package-level var that gets set during `Init()` (line 230). This is a hidden side effect and makes the code harder to reason about.

## Recommendations

1. **Break up `Plugin` interface.** Keep core identity/lifecycle (Slug, TabName, Init, View, HandleKey, HandleMessage) and make everything else optional interfaces. This is the highest-impact change for API cleanliness.

2. **Introduce a `renderCtx` struct** for all `cc_view.go` rendering functions. This immediately fixes 5+ functions with 8-14 parameters.

3. **Replace `interface{}` in `plugin.Context`.** Import `llm.LLM` directly (no circularity). Remove `Styles`/`Grad` fields entirely since all three built-in plugins construct their own local styles from `config.GetPalette()`.

4. **Define Action type constants.** Replace string literals with `const ActionNoop = "noop"` etc.

5. **Extract `contentMaxWidth` to a shared location.** One constant, four consumers.

6. **Drop the `DB` prefix** from all exported `db` package functions. The package name already provides context. Or, introduce a receiver type (`type Store struct { db *sql.DB }`) to make calls like `store.CompleteTodo(id)`.

7. **Type the event payloads.** Even a `map[string]string` would be safer than `map[string]interface{}` for the current usage patterns, or define per-topic payload structs.

8. **Audit legacy file-based functions in `db/types.go`.** `LoadPaths`, `SavePaths`, `LoadWinddownSessions`, `LoadBookmarks`, `LoadAllSessions`, `RemoveBookmark`, `ParseSessionFile` appear to be legacy code superseded by DB-backed equivalents.
