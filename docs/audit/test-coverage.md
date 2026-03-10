# Test Coverage Audit

## Summary

The project has solid test coverage for core data structures, plugin infrastructure, and UI components, but significant gaps exist in the refresh pipeline (all 5 external data sources are completely untested), the LLM abstraction layer, and the two CLI entrypoints. Critical business logic like the merge algorithm and DB operations are well-tested, while network-dependent code and the refresh orchestrator have no tests at all.

## Coverage Map

| Package | Has Tests | Files Tested | Files Untested | Coverage Quality |
|---------|-----------|-------------|----------------|-----------------|
| `internal/db` | Yes | `db.go`, `types.go`, `migrate.go` | (none) | **Strong** — thorough CRUD round-trips, migration, save/load, helper functions |
| `internal/config` | Yes | `config.go`, `palettes.go`, `validate.go`, `doctor.go`, `schedule.go` | `setup.go` | **Good** — config load/save, palettes, paths, doctor checks; setup wizard untested (interactive I/O) |
| `internal/plugin` | Yes | `eventbus.go`, `logger.go`, `registry.go`, `migrations.go` | `plugin.go`, `lifecycle.go` | **Good** — all core infra tested; `plugin.go` is just interfaces, `lifecycle.go` is just message types |
| `internal/refresh` | Partial | `lock.go`, `merge.go` | `refresh.go`, `calendar.go`, `github.go`, `gmail.go`, `granola.go`, `slack.go`, `actions.go`, `llm.go`, `auth.go`, `datasource.go`, `types.go` | **Weak** — only lock and merge tested; entire fetch pipeline untested |
| `internal/tui` | Yes | `model.go`, `notify.go`, `styles.go`, `banner.go` | `launch.go`, `effects.go` | **Good** — tab navigation, view rendering, socket notify; launch/effects untested |
| `internal/builtin/commandcenter` | Yes | `commandcenter.go`, `claude_exec.go` | `refresh.go`, `cc_view.go`, `threads_view.go`, `styles.go` | **Good** — keybindings, navigation, undo, modes, messages; view rendering and refresh spawning untested |
| `internal/builtin/sessions` | Yes | `sessions.go` | (none) | **Good** — paths, session loading, key handling, navigation |
| `internal/builtin/settings` | Yes | `settings.go` | (none) | **Good** — toggle logic, sub-views, detail views |
| `internal/external` | Yes | `external.go`, `process.go` | `loader.go`, `protocol.go` | **Good** — handshake, render, key handling, crash detection, async events; loader untested |
| `internal/llm` | No | (none) | `llm.go`, `claude_cli.go`, `noop.go` | **None** — no tests for LLM layer |
| `cmd/ccc` | No | (none) | `main.go` | **None** — CLI entrypoint |
| `cmd/ccc-refresh` | No | (none) | `main.go` | **None** — CLI entrypoint |

## Critical Untested Code

### 1. Refresh Data Sources (HIGH RISK)

All five external data source implementations have zero tests:

- `/Users/aaron/Personal/claude-command-center/internal/refresh/calendar.go` — Google Calendar fetching, event parsing, auto-accept logic, `matchesDomain()` helper
- `/Users/aaron/Personal/claude-command-center/internal/refresh/github.go` — GitHub PR listing via `gh` CLI, PR summarization, JSON parsing
- `/Users/aaron/Personal/claude-command-center/internal/refresh/granola.go` — Granola API calls, meeting listing, transcript fetching, gzip handling
- `/Users/aaron/Personal/claude-command-center/internal/refresh/slack.go` — Slack message search, commitment language detection (`hasCommitmentLanguage`), thread context fetching
- `/Users/aaron/Personal/claude-command-center/internal/refresh/gmail.go` — Gmail message listing, header parsing, URL construction

These are the primary data ingestion paths. Bugs here silently produce wrong data. The `hasCommitmentLanguage()` function in `slack.go` and `matchesDomain()` in `calendar.go` are pure functions that could easily be unit tested.

### 2. Refresh Orchestrator (HIGH RISK)

`/Users/aaron/Personal/claude-command-center/internal/refresh/refresh.go` — The `Run()` function orchestrates the entire pipeline: parallel source fetching, merge, LLM suggestions, pending action execution, and DB save. No tests verify this coordination. The `combineResults()` function in `datasource.go` is a pure function that should be tested.

### 3. LLM Integration (MEDIUM RISK)

- `/Users/aaron/Personal/claude-command-center/internal/refresh/llm.go` — `extractCommitments()` and `generateSuggestions()` parse LLM JSON responses. The `cleanJSON()`, `activeTodos()`, and `activeThreads()` helper functions are pure and easily testable.
- `/Users/aaron/Personal/claude-command-center/internal/llm/claude_cli.go` — `ClaudeCLI.Complete()` shells out to claude; error handling paths untested.
- `/Users/aaron/Personal/claude-command-center/internal/llm/noop.go` — Trivial but still no compile-time test beyond the `var _ LLM` check.

### 4. Auth Loading (MEDIUM RISK)

`/Users/aaron/Personal/claude-command-center/internal/refresh/auth.go` — All auth loading functions (`loadCalendarAuth`, `loadGmailAuth`, `loadGranolaAuth`, `loadSlackToken`, `loadGitHubToken`) are untested. The `loadEnvFile()` function parses `.env` files with edge cases (quoting, comments, existing vars) that should be tested. The `googleTokenFile.toOAuth2Token()` method is a pure transformation with no test.

### 5. Pending Actions Execution (MEDIUM RISK)

`/Users/aaron/Personal/claude-command-center/internal/refresh/actions.go` — `executePendingActions()` creates Google Calendar events and `findFreeSlot()` implements time-slot-finding logic. `findFreeSlot()` contains date arithmetic and boundary conditions that are error-prone and untested.

### 6. Launch/Effects (LOW RISK)

- `/Users/aaron/Personal/claude-command-center/internal/tui/launch.go` — `RunClaude()` does chdir + exec; inherently hard to test but the initial prompt file-writing logic could be tested.
- `/Users/aaron/Personal/claude-command-center/internal/tui/effects.go` — Animation/ticker logic.

## Weak Tests

### 1. Config Doctor Checks (`internal/config/config_test.go`)

`TestDoctorChecks` verifies count and that config check passes, but the assertions on specific check results are loose (commented-out logic, vague conditions). The test doesn't verify specific check behavior like `checkDataFreshness` with stale data.

### 2. Config Validation (`internal/config/validate_test.go`)

Only tests the negative case (missing credentials/CLI). Does not test the positive case where credentials exist. These are thin tests — 3 tests, each just checking that an error is returned when external dependencies are missing.

### 3. External Plugin Crash Detection (`internal/external/external_test.go`)

`TestCrashDetection` uses `time.Sleep(100ms)` twice for timing-dependent assertions. The test could be flaky under load. The restart path modifies internal state (`ep.command`, `ep.errState`) directly rather than through public API.

### 4. Command Center View Tests (`internal/builtin/commandcenter/commandcenter_test.go`)

`TestViewRendersWithoutPanic` and `TestViewWithNilCC` only check that `View()` returns a non-empty string. They don't verify that specific content appears in the rendered output (e.g., todo titles, thread names).

## Recommendations

Priority order for adding tests:

1. **Pure helper functions in refresh pipeline** (low effort, high value)
   - `hasCommitmentLanguage()` in `slack.go`
   - `matchesDomain()` in `calendar.go`
   - `cleanJSON()` in `llm.go`
   - `combineResults()` in `datasource.go`
   - `activeTodos()` and `activeThreads()` in `llm.go`
   - `loadEnvFile()` in `auth.go`
   - `googleTokenFile.toOAuth2Token()` in `auth.go`

2. **`findFreeSlot()` in actions.go** (medium effort, high value) — Date arithmetic with boundary conditions (end of day, next day rollover, back-to-back events). Can be tested with a mock `calendar.Service` or by extracting the slot-finding logic into a pure function.

3. **DataSource interface with mock sources** (medium effort, high value) — Test `Run()` in `refresh.go` with mock `DataSource` implementations that return canned data. Verifies the parallel fetch, merge, and save orchestration.

4. **LLM response parsing** (medium effort, medium value) — Test `extractCommitments()` and `generateSuggestions()` with a mock LLM that returns canned JSON responses. Test malformed JSON handling. Test the `extractSlackCommitments()` prompt/parse path.

5. **Auth loading with temp files** (medium effort, medium value) — Create temp credential files and verify `loadCalendarAuth()`, `loadGmailAuth()`, `loadGranolaAuth()` parse them correctly. Test expired token detection in `loadGranolaAuth()`.

6. **`external.LoadExternalPlugins()`** (low effort, low value) — Test with empty config, disabled plugins, and missing commands.

7. **View content assertions** (low effort, low value) — Strengthen `commandcenter` and `tui` view tests to assert specific content (todo titles, thread counts) appears in rendered output rather than just checking non-empty.
