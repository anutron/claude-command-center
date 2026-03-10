# Spec-Implementation Alignment Audit

Generated: 2026-03-09

## Summary

The project has 17 specs covering core infrastructure, plugin system, and built-in plugins. Overall alignment is strong -- most specs accurately reflect the implemented code. Key issues: the db spec references several legacy JSON functions and `EnforceDismissed` that no longer exist in the codebase; the host spec describes direct plugin references (`sessionsPlugin`, `commandCenterPlugin`) that have been removed; and the LLM spec has no tests. Several specs have minor key binding discrepancies with the actual implementation.

## Spec Inventory

| Spec | Status | Implementation Match | Tests Match |
|------|--------|---------------------|-------------|
| `specs/core/config.md` | Implemented | Match | Match (6/6 test cases covered) |
| `specs/core/db.md` | Implemented | Partial -- stale legacy refs | Partial -- 20+ of ~27 cases covered; missing EnforceDismissed tests |
| `specs/core/host.md` | Implemented | Partial -- stale direct refs | Match (9/9 test cases covered) |
| `specs/core/cli.md` | Implemented | Match | Partial -- doctor/schedule tests exist; setup not tested |
| `specs/core/llm.md` | Implemented | Match | Missing -- no test file in `internal/llm/` |
| `specs/core/refresh.md` | Implemented | Match | Match (9/9 merge tests + 4 lock tests) |
| `specs/core/datasource.md` | Implemented | Match | Partial -- no dedicated datasource tests |
| `specs/plugin/interface.md` | Implemented | Match | Implicit via plugin tests |
| `specs/plugin/registry.md` | Implemented | Match (+ `IndexOf` not in spec) | Match (5/5 + IndexOf test) |
| `specs/plugin/protocol.md` | Implemented | Match | Partial -- 6/8 in external_test.go |
| `specs/plugin/external-adapter.md` | Implemented | Match | Match (7/7 test cases covered) |
| `specs/plugin/lifecycle.md` | Implemented | Match | Partial -- host test covers tab nav; no dedicated lifecycle msg tests |
| `specs/plugin/event-bus.md` | Implemented | Match | Match (4/5 -- missing explicit "handler receives correct fields" test, though tested implicitly) |
| `specs/builtin/sessions.md` | Implemented | Match | Partial -- Init/HandleKey tests exist but sessions_test.go is minimal |
| `specs/builtin/pomodoro.md` | Example only | N/A (external example plugin) | N/A |
| `specs/builtin/settings.md` | Implemented | Match | Match (13+ test cases covering all sub-views) |
| `specs/builtin/command-center.md` | Implemented | Partial -- key binding discrepancies | Match (20+ test cases) |

## Detailed Findings

### specs/core/db.md -- Stale Legacy References

The db spec documents several functions that no longer exist in the codebase:

- **`EnforceDismissed`**: Spec says it auto-dismisses active todos matching dismissed source_refs and runs on every `LoadCommandCenter` call. No function by this name exists in the code. The merge logic in `internal/refresh/merge.go` handles dismissed-todo filtering instead.
- **`LoadCommandCenter(path)`**: Legacy JSON loading function no longer exists. Replaced by `LoadCommandCenterFromDB(db)`.
- **`SaveCommandCenter(path, cc)`**: Legacy JSON saving no longer exists. Replaced by `DBSaveRefreshResult`.
- **`MutateSave(path, fallback, fn)`**: Legacy reload-modify-save pattern no longer exists.
- **`LoadWinddownSessions`, `LoadAllSessions`, `ParseSessionFile`, `LoadBookmarks`, `RemoveBookmark`**: These JSON/file-based functions still exist in `internal/db/types.go` and have tests, but are legacy. The spec should clarify their status.

The spec's "JSON File Operations (Legacy)" section is accurate in calling these legacy, but they appear to still be in the codebase and tested. The spec should note which are still used (e.g., by the sessions plugin for winddown files) vs. fully deprecated.

### specs/core/host.md -- Stale Direct Plugin References

The spec's Model struct shows:
```go
sessionsPlugin      *sessions.Plugin
commandCenterPlugin *commandcenter.Plugin
```

These fields do not exist in the actual `Model` struct at `/Users/aaron/Personal/claude-command-center/internal/tui/model.go`. The "Cross-Plugin Communication" section describing direct references is outdated -- the event bus is now the sole communication mechanism.

The actual Model struct has `allPlugins []plugin.Plugin` and `returnedFromLaunch bool` which are not in the spec's struct definition.

### specs/builtin/command-center.md -- Key Binding Discrepancies

The spec lists these key bindings that differ from the implementation:

- Spec says `d` = "Dismiss todo", but the test shows `X` (capital) dismisses and `d` defers. Looking at `commandcenter_test.go`: `TestDeferTodo` uses `keyMsg("d")` and `TestDismissTodo` uses `keyMsg("X")`.
- Spec says `D` = "Defer todo to bottom", but implementation uses `d` (lowercase) for defer.
- Spec says `z` = "Undo last completion", but test uses `u` for undo (`TestUndoCompletion` sends `keyMsg("u")`).
- Spec says `B` = "Enter booking mode", but test uses `s` (`TestBookingMode` sends `keyMsg("s")`).

### specs/core/llm.md -- No Tests

The spec lists three test cases:
- `NoopLLM.Complete()` returns `""`, `nil`
- `Available()` returns `true` when `claude` is on PATH, `false` otherwise
- `ClaudeCLI` implements the `LLM` interface (compile-time check)

The compile-time check exists in `internal/llm/claude_cli.go` (`var _ LLM = ClaudeCLI{}`), but there is no `_test.go` file in `internal/llm/`. The first two test cases have no test implementations.

### specs/core/datasource.md -- No Dedicated Tests

The spec lists 6 test cases for the DataSource interface. While the merge tests cover some of this behavior implicitly, there are no tests specifically for:
- Source with `Enabled() == false` is not fetched
- Source returning error produces a warning
- `combineResults` combining multiple sources

### specs/plugin/lifecycle.md -- Incomplete Test Coverage

The spec lists 11 test cases. The host test (`model_test.go`) covers tab navigation and tab entry mapping, which implicitly tests `TabViewMsg`/`TabLeaveMsg` via `activateTab()`. However, there are no explicit tests for:
- `LaunchMsg` broadcast before quit on launch action
- `ReturnMsg` broadcast on Init when `returnedFromLaunch` is true
- Command center staleness-based reload on `TabViewMsg`
- Removal of 60s tick-based reload

### specs/plugin/protocol.md -- Missing Edge Case Tests

The spec lists 8 test cases. The `external_test.go` covers 6 (init, render, key, crash, restart, shutdown, async events). Missing:
- Malformed JSON from plugin is logged and ignored
- Plugin that never flushes stdout is handled gracefully (loading state shown)

## Unspecced Features

The following implemented features have no corresponding spec:

1. **Plugin Logger** (`internal/plugin/logger.go`): `Logger` interface, `FileLogger`, `MemoryLogger` -- fully implemented and tested (`logger_test.go`) but no spec. Used by settings log view and plugin error reporting.

2. **Plugin Migrations** (`internal/plugin/migrations.go`): `RunMigrations` function -- fully implemented and tested (`migrations_test.go`) but only mentioned in the plugin interface spec as a method signature. No dedicated spec for the migration tracking system (ccc_plugin_migrations table, version tracking, idempotency).

3. **Notification System** (`internal/tui/notify.go`): Unix socket notification (`StartNotifyListener`, `SendNotify`, `SocketPath`) -- fully implemented and tested (`notify_test.go`). Described in the host spec's "Cross-Instance Notification" section and the CLI spec's `ccc notify` section, but not as a standalone spec.

4. **TUI Effects/Animation** (`internal/tui/effects.go`, `internal/tui/banner.go`): Gradient animation, tick system, fade-in -- described in the host spec but no detailed spec for the animation system itself.

5. **Config Validation** (`internal/config/validate.go`): `ValidateCalendar`, `ValidateGitHub`, `ValidateGranola` -- implemented and tested but not specced. Used by settings plugin for credential checking.

6. **Refresh Auth** (`internal/refresh/auth.go`): OAuth token loading, calendar credential migration -- implemented but not specced beyond brief mentions in the refresh spec.

7. **Registry.IndexOf()** (`internal/plugin/registry.go`): Method exists in implementation and has tests but is not in the registry spec.

## Unimplemented Specs

1. **`specs/core/db.md` -- EnforceDismissed**: Spec describes an `EnforceDismissed` function that auto-dismisses todos with matching source_refs. This function does not exist. The behavior is handled differently -- the merge logic in `internal/refresh/merge.go` prevents dismissed items from being recreated.

2. **`specs/core/db.md` -- Legacy JSON functions**: `LoadCommandCenter`, `SaveCommandCenter`, `MutateSave` are documented as current functions but `LoadCommandCenter` and `SaveCommandCenter` and `MutateSave` no longer exist. The file-based `LoadWinddownSessions`, `LoadAllSessions`, `ParseSessionFile` still exist.

3. **`specs/core/host.md` -- Direct plugin references for cross-plugin communication**: The spec says the host holds `sessionsPlugin` and `commandCenterPlugin` references. The implementation uses only the event bus pattern; these direct references have been removed.

## Stale Specs

| Spec | Stale Element | Details |
|------|--------------|---------|
| `specs/core/db.md` | `EnforceDismissed` | Function does not exist; behavior handled by merge logic |
| `specs/core/db.md` | `LoadCommandCenter(path)` | Replaced by `LoadCommandCenterFromDB(db)` |
| `specs/core/db.md` | `SaveCommandCenter(path, cc)` | Replaced by `DBSaveRefreshResult` |
| `specs/core/db.md` | `MutateSave(path, fallback, fn)` | Removed; DB operations are direct now |
| `specs/core/host.md` | `sessionsPlugin`, `commandCenterPlugin` fields | Removed from Model struct; event bus used instead |
| `specs/core/host.md` | Model struct definition | Missing `allPlugins`, `returnedFromLaunch`; includes removed fields |
| `specs/builtin/command-center.md` | Key bindings table | `d`/`D`/`z`/`B` mappings don't match actual implementation (`d`=defer, `X`=dismiss, `u`=undo, `s`=booking) |
| `specs/plugin/registry.md` | API surface | Missing `IndexOf(slug string) int` method |

## Spec Quality

**Strengths:**
- Specs are well-structured with clear Purpose, Interface, Behavior, and Test Cases sections
- The DataSource spec and refresh spec form a coherent pair
- Plugin interface spec is precise and matches the Go interface exactly
- Event bus catalog is comprehensive and useful as documentation
- Test case sections are specific and actionable

**Weaknesses:**
- The db spec is the largest and most stale -- it carries significant legacy baggage from the JSON-file era that hasn't been cleaned up
- Key binding specs in command-center.md are wrong in 4+ places -- these are high-frequency reference items that should be accurate
- Some specs describe internal implementation details (struct field names) that drift as the code evolves -- prefer behavioral descriptions
- No spec for the Logger system, which is used across the entire codebase
- The pomodoro spec exists but it's an example plugin, not a core feature -- could be moved to `examples/`

## Recommendations

1. **Update `specs/core/db.md`**: Remove or clearly mark as deleted: `EnforceDismissed`, `LoadCommandCenter`, `SaveCommandCenter`, `MutateSave`. Document that dismissed-todo filtering now lives in the merge layer. Keep file-based session functions but mark them as "used by sessions plugin for winddown files only."

2. **Update `specs/core/host.md`**: Remove `sessionsPlugin`/`commandCenterPlugin` from the Model struct. Add `allPlugins` and `returnedFromLaunch`. Update the "Cross-Plugin Communication" section to reflect event-bus-only communication.

3. **Fix `specs/builtin/command-center.md` key bindings**: Correct the key binding table to match the actual implementation: `d`=defer, `X`=dismiss, `u`=undo, `s`=booking. Verify all other bindings against `commandcenter.go`.

4. **Add tests for `internal/llm/`**: Create `llm_test.go` with the 2 testable cases from the spec (NoopLLM and Available). The compile-time check already exists.

5. **Add spec for Logger**: Create `specs/plugin/logger.md` covering the `Logger` interface, `FileLogger`, `MemoryLogger`, log entry format, and memory limit behavior.

6. **Add DataSource integration tests**: Test the `combineResults` function and the enabled-source filtering in `Run()`.

7. **Update `specs/plugin/registry.md`**: Add `IndexOf(slug string) int` method to the interface section.

8. **Move `specs/builtin/pomodoro.md`**: Move to `examples/pomodoro/` since it's an example external plugin, not a built-in.

9. **Add lifecycle message tests**: Create dedicated tests for `LaunchMsg` and `ReturnMsg` broadcasting, and for the command center's staleness-based reload behavior on `TabViewMsg`.
