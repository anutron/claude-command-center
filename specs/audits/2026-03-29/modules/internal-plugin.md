# Spec Coverage Audit: internal/plugin + internal/external

**Date:** 2026-03-29
**Modules:** `internal/plugin/`, `internal/external/`
**Specs:** `specs/plugin/event-bus.md`, `specs/plugin/interface.md`, `specs/plugin/lifecycle.md`, `specs/plugin/logger.md`, `specs/plugin/migrations.md`, `specs/plugin/registry.md`, `specs/plugin/external-adapter.md`, `specs/plugin/protocol.md`

---

## Summary

- **Total exported behavioral branches:** 78
- **Covered:** 62
- **Uncovered-behavioral:** 10
- **Uncovered-implementation:** 4
- **Contradictions:** 2

---

## internal/plugin/eventbus.go

**Spec:** `specs/plugin/event-bus.md`

### NewBus()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns initialized Bus with empty handlers map | **[COVERED]** | Spec Behavior: "Publish delivering event to all handlers subscribed to the event's Topic" implies working bus construction |

### Bus.Subscribe(topic, handler)

| Branch | Status | Notes |
|--------|--------|-------|
| Registers handler for a new topic | **[COVERED]** | Spec Behavior: "Subscribe registers a handler for a topic" |
| Appends handler for existing topic (multiple handlers) | **[COVERED]** | Spec Behavior: "Multiple handlers per topic allowed" |
| Mutex-protected write | **[UNCOVERED-IMPLEMENTATION]** | Concurrency detail, no spec needed |

### Bus.Publish(event)

| Branch | Status | Notes |
|--------|--------|-------|
| Delivers event to all subscribed handlers | **[COVERED]** | Spec Behavior: "Publish delivers the event to all handlers subscribed to the event's Topic" |
| No subscribers for topic: no-op | **[COVERED]** | Spec Behavior: "Publishing to a topic with no subscribers is a no-op (no error)" |
| Handlers called synchronously in registration order | **[COVERED]** | Spec Behavior: "Events are delivered synchronously in the order handlers were registered" |
| RLock for concurrent reads | **[UNCOVERED-IMPLEMENTATION]** | Concurrency detail |

### Event.Payload type

| Branch | Status | Notes |
|--------|--------|-------|
| Code uses `any` for Payload | **[CONTRADICTS]** | Spec Interface declares `Payload map[string]interface{}` but code uses `Payload any`. The code is more permissive. External adapter passes `map[string]interface{}`, built-in plugins may pass other types. Spec should be updated to `any`. |

---

## internal/plugin/plugin.go

**Spec:** `specs/plugin/interface.md`

### Types: Plugin, Context, Action, Route, Migration, KeyBinding

| Branch | Status | Notes |
|--------|--------|-------|
| Plugin interface methods match spec | **[COVERED]** | Spec Interface section lists all 14 methods; code matches |
| Context struct fields | **[COVERED]** | Spec Context section. Code adds `AgentRunner agent.Runner` field |
| Action constants (Noop, Consumed, OpenURL, Flash, Launch, Quit, Navigate, Unhandled) | **[COVERED]** | Spec lists all except `ActionConsumed` |
| ActionConsumed constant | **[UNCOVERED-BEHAVIORAL]** | Code defines `ActionConsumed = "consumed"` with semantics "key was handled, no further action needed" -- prevents host default behavior unlike NoopAction. **Intent question: Should spec document the distinction between Noop (host may apply defaults) and Consumed (host must not)?** |

### NoopAction()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns Action{Type: ActionNoop} | **[COVERED]** | Spec Test Cases: "NoopAction() helper returns correct default" |

### ConsumedAction()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns Action{Type: ActionConsumed} | **[UNCOVERED-BEHAVIORAL]** | Not in spec. Prevents host from applying default key handling (e.g., Tab switching). **Intent question: Is ConsumedAction a distinct host-observable behavior or just a variant of Noop?** |

### Starter interface

| Branch | Status | Notes |
|--------|--------|-------|
| StartCmds() returns tea.Cmd for initial commands | **[COVERED]** | Spec Interface section: "Plugins that need to run initial tea.Cmds (e.g., spinner ticks)" |

### SettingsProvider interface

| Branch | Status | Notes |
|--------|--------|-------|
| SettingsView, HandleSettingsKey | **[COVERED]** | Spec Interface section describes both |
| SettingsOpenCmd() returns tea.Cmd for async init | **[UNCOVERED-BEHAVIORAL]** | Code adds `SettingsOpenCmd() tea.Cmd` not in spec. Called when user navigates to settings pane. **Intent question: Should spec document async credential checks on settings open?** |
| HandleSettingsMsg(msg) for async result handling | **[UNCOVERED-BEHAVIORAL]** | Code adds `HandleSettingsMsg(msg tea.Msg) (bool, Action)` not in spec. **Intent question: Should spec document settings async message routing?** |

### Context.AgentRunner field

| Branch | Status | Notes |
|--------|--------|-------|
| AgentRunner field in Context | **[UNCOVERED-BEHAVIORAL]** | Code has `AgentRunner agent.Runner` in Context, spec does not list it. **Intent question: Should agent runner dependency be specced?** |

### NotifyMsg type

| Branch | Status | Notes |
|--------|--------|-------|
| NotifyMsg{Event string} for external process notifications | **[COVERED]** | Spec event-bus.md Behavior: references NotifyMsg for daemon events |

---

## internal/plugin/lifecycle.go

**Spec:** `specs/plugin/lifecycle.md`

### TabViewMsg

| Branch | Status | Notes |
|--------|--------|-------|
| Sent when tab becomes active | **[COVERED]** | Spec Types section: "Sent to a plugin when its tab becomes the active tab" |

### TabLeaveMsg

| Branch | Status | Notes |
|--------|--------|-------|
| Sent when tab deactivated | **[COVERED]** | Spec Types section: "Sent to a plugin when its tab is being deactivated" |

### LaunchMsg

| Branch | Status | Notes |
|--------|--------|-------|
| Broadcast before TUI quits for Claude launch | **[COVERED]** | Spec Types section: "Broadcast to all plugins just before the TUI quits to launch a Claude session" |

### ReturnMsg

| Branch | Status | Notes |
|--------|--------|-------|
| ReturnMsg has TodoID and WasResumeJoin fields | **[CONTRADICTS]** | Spec declares `ReturnMsg struct{}` (empty). Code has `TodoID string` and `WasResumeJoin bool`. Spec needs updating to reflect the added fields. |

---

## internal/plugin/logger.go

**Spec:** `specs/plugin/logger.md`

### NewFileLogger(logPath)

| Branch | Status | Notes |
|--------|--------|-------|
| Creates parent dirs, opens file in append mode | **[COVERED]** | Spec Implementations: "Creates parent directories as needed", "Opens file in append mode" |
| Returns error if MkdirAll fails | **[COVERED]** | Implicit in spec "Creates parent directories as needed" |
| Returns error if OpenFile fails | **[COVERED]** | Spec: "Opens file in append mode" |
| Sets maxMem = 500 | **[COVERED]** | Spec: "Keeps up to 500 entries in memory" |

### NewMemoryLogger()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns FileLogger with nil file | **[COVERED]** | Spec: "Returns a *FileLogger with nil file handle" |
| Same 500-entry limit | **[COVERED]** | Spec: "Same 500-entry memory limit" |

### FileLogger.log(level, plugin, msg, fields...)

| Branch | Status | Notes |
|--------|--------|-------|
| Creates LogEntry with current time | **[COVERED]** | Spec Behavior step 1 |
| Appends to in-memory buffer | **[COVERED]** | Spec Behavior step 2 |
| Trims oldest when > maxMem | **[COVERED]** | Spec Behavior step 3 |
| Writes to file if file != nil | **[COVERED]** | Spec Behavior step 4 |
| Skips file write if file == nil | **[COVERED]** | Spec MemoryLogger: "Close() is a no-op" implies no file |

### FileLogger.Info/Warn/Error

| Branch | Status | Notes |
|--------|--------|-------|
| Delegate to log with correct level string | **[COVERED]** | Spec Test Cases: "Info/Warn/Error create entries with correct level" |

### FileLogger.Recent(n)

| Branch | Status | Notes |
|--------|--------|-------|
| Returns last n entries as copy | **[COVERED]** | Spec Behavior step 5, Test Cases |
| n > total returns all entries | **[COVERED]** | Spec Test Cases: "Recent with n > total returns all entries" |

### FileLogger.Close()

| Branch | Status | Notes |
|--------|--------|-------|
| Closes file if non-nil | **[COVERED]** | Spec: "Close closes the underlying file" |
| Returns nil if file is nil | **[COVERED]** | Spec: "Close() is a no-op" for MemoryLogger |

---

## internal/plugin/migrations.go

**Spec:** `specs/plugin/migrations.md`

### ValidateExternalMigrationSQL(slug, sqlText)

| Branch | Status | Notes |
|--------|--------|-------|
| Strips SQL comments before validation | **[COVERED]** | Spec: "SQL comments are stripped before validation" |
| Splits on semicolons, validates each statement | **[COVERED]** | Spec: "each statement must be namespaced DDL" |
| Empty statements after split are skipped | **[UNCOVERED-IMPLEMENTATION]** | Internal whitespace handling |
| Allowed patterns: CREATE TABLE/INDEX, ALTER TABLE, DROP TABLE/INDEX with slug prefix | **[COVERED]** | Spec External Plugin Migration Security section |
| Returns error for disallowed SQL with truncated preview | **[COVERED]** | Spec: "Any non-matching statement causes the plugin to be rejected" |
| Returns nil when all statements valid | **[COVERED]** | Implicit |

### RunMigrations(db, slug, migrations)

| Branch | Status | Notes |
|--------|--------|-------|
| No-op when db is nil | **[COVERED]** | Spec Behavior step 1 |
| No-op when migrations is empty | **[COVERED]** | Spec Behavior step 1 |
| Creates tracking table if not exists | **[COVERED]** | Spec Behavior step 2 |
| Queries max applied version | **[COVERED]** | Spec Behavior step 3 |
| Skips migrations with Version <= maxVersion | **[COVERED]** | Spec: "Skips already-applied migrations" |
| Applies pending migrations in transaction | **[COVERED]** | Spec Behavior step 4 |
| Records migration in tracking table | **[COVERED]** | Spec Behavior step 4 |
| Rolls back on SQL execution error | **[COVERED]** | Spec Behavior step 5, Key Design: "Transactional" |
| Rolls back on tracking insert error | **[COVERED]** | Spec Behavior step 5 |
| Returns error on commit failure | **[COVERED]** | Implicit in "If any step fails" |
| Returns error on begin tx failure | **[UNCOVERED-IMPLEMENTATION]** | Error path detail |

---

## internal/plugin/registry.go

**Spec:** `specs/plugin/registry.md`

### NewRegistry()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns empty registry | **[COVERED]** | Implicit in spec interface |

### Registry.Register(p)

| Branch | Status | Notes |
|--------|--------|-------|
| Adds new plugin in registration order | **[COVERED]** | Spec: "Register adds a plugin to the registry in order of registration" |
| Replaces existing plugin with same slug at same index | **[COVERED]** | Spec: "Duplicate slugs: last registration wins (overwrites previous at the same index)" |

### Registry.All()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns copy in registration order | **[COVERED]** | Spec: "All() returns a copy of plugins in registration order" |

### Registry.BySlug(slug)

| Branch | Status | Notes |
|--------|--------|-------|
| Found: returns (plugin, true) | **[COVERED]** | Spec: "BySlug looks up a plugin by its Slug() value" |
| Not found: returns (nil, false) | **[COVERED]** | Spec: "returns false if not found" |

### Registry.Count()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns number of registered plugins | **[COVERED]** | Spec: "Count returns the number of registered plugins" |

### Registry.IndexOf(slug)

| Branch | Status | Notes |
|--------|--------|-------|
| Found: returns index | **[COVERED]** | Spec: "IndexOf returns the index for a plugin slug" |
| Not found: returns -1 | **[COVERED]** | Spec: "or -1 if not found" |

---

## internal/plugin/scope.go

**No dedicated spec file.** Covered partially in `specs/plugin/external-adapter.md`.

### ScopeConfig(cfg, scopes)

| Branch | Status | Notes |
|--------|--------|-------|
| Empty scopes returns empty map | **[COVERED]** | External-adapter spec: "If config_scopes is empty, use an empty map (secure by default)" |
| Nil cfg returns empty map | **[UNCOVERED-BEHAVIORAL]** | Code handles `cfg == nil` but no spec mentions this guard. **Intent question: Is nil-config a real scenario or defensive code?** |
| Matches yaml struct tags to scope names (case-insensitive) | **[COVERED]** | External-adapter spec: "matched against YAML struct tags" |
| Skips fields with empty or "-" yaml tags | **[UNCOVERED-BEHAVIORAL]** | Not documented anywhere. **Intent question: Should spec note that unexported/untagged config fields are never scoped?** |
| Returns only matching fields | **[COVERED]** | External-adapter spec: "Scope the host config to only the sections listed in config_scopes" |

---

## internal/plugin/doctor.go

**No dedicated spec file.** Not referenced in any existing spec.

### Types: ValidationResult, DoctorCheck, DoctorOpts, DoctorProvider

| Branch | Status | Notes |
|--------|--------|-------|
| ValidationResult with Status/Message/Hint | **[UNCOVERED-BEHAVIORAL]** | No spec covers the doctor/health-check system. **Intent question: Should there be a `specs/plugin/doctor.md` spec for the credential/config health check system?** |
| DoctorCheck with Name/Result/Inconclusive | **[UNCOVERED-BEHAVIORAL]** | Same as above |
| DoctorOpts with Live flag (network vs offline checks) | **[UNCOVERED-BEHAVIORAL]** | Same as above |
| DoctorProvider interface: DoctorChecks(opts) []DoctorCheck | **[UNCOVERED-BEHAVIORAL]** | Same as above. This is a plugin-facing optional interface, same level as SettingsProvider. |

---

## internal/external/protocol.go

**Spec:** `specs/plugin/protocol.md`

### HostMsg struct

| Branch | Status | Notes |
|--------|--------|-------|
| Type field with all message types (init, config, render, key, navigate, event, refresh, shutdown) | **[COVERED]** | Spec Host -> Plugin Messages table |
| Init fields: Config (as RawMessage), DBPath, Width, Height | **[COVERED]** | Spec protocol examples |
| Render fields: Width, Height, Frame | **[COVERED]** | Spec render example |
| Key fields: Key, Alt | **[COVERED]** | Spec key example |
| Navigate fields: Route, Args | **[COVERED]** | Spec navigate example |
| Event fields: Source, Topic, Payload | **[COVERED]** | Spec event example |

### PluginMsg struct

| Branch | Status | Notes |
|--------|--------|-------|
| Ready fields: Slug, TabName, RefreshMS, Routes, KeyBindings, Migrations, ConfigScopes | **[COVERED]** | Spec Plugin -> Host Messages table |
| View fields: Content | **[COVERED]** | Spec view example |
| Action fields: Action, APayload, AArgs | **[COVERED]** | Spec action example |
| Event fields: Topic2, EPayload | **[COVERED]** | Spec event example |
| Log fields: Level, Message | **[COVERED]** | Spec log example |

### RouteMsg, KeyBindingMsg, MigrationMsg

| Branch | Status | Notes |
|--------|--------|-------|
| Wire format structs for ready message sub-objects | **[COVERED]** | Spec ready example JSON |

---

## internal/external/process.go

**Spec:** `specs/plugin/external-adapter.md`

### Process.Start(command, logger)

| Branch | Status | Notes |
|--------|--------|-------|
| Launches via `sh -c <command>` | **[COVERED]** | Spec Process: "Launches subprocess via sh -c <command>" |
| Sets PYTHONUNBUFFERED=1 | **[COVERED]** | Spec Process: "with PYTHONUNBUFFERED=1" |
| Creates stdin/stdout/stderr pipes | **[COVERED]** | Implicit in process management |
| Starts stdout and stderr reader goroutines | **[COVERED]** | Spec Process: "Stdout reader goroutine routes messages" |

### Process.Send(msg)

| Branch | Status | Notes |
|--------|--------|-------|
| Checks process liveness before writing | **[COVERED]** | Spec Process: "checks process liveness before writing" |
| Returns error if process exited | **[COVERED]** | Spec Process: "checks process liveness before writing" |
| Marshals JSON + newline to stdin | **[COVERED]** | Spec Transport: "single line of JSON followed by a newline" |
| Mutex-protected write | **[COVERED]** | Spec Process: "Send is mutex-protected" |

### Process.Receive(timeout)

| Branch | Status | Notes |
|--------|--------|-------|
| Returns message from syncResp channel | **[COVERED]** | Spec Process: "Receive blocks on syncResp with configurable timeout" |
| Returns error on process exit | **[COVERED]** | Implicit |
| Returns error on timeout | **[COVERED]** | Spec Process: "with configurable timeout" |

### Process.DrainAsync()

| Branch | Status | Notes |
|--------|--------|-------|
| Non-blocking drain of all pending async messages | **[COVERED]** | Spec Process: "DrainAsync non-blocking drain of all pending async messages" |
| Returns empty slice when no messages | **[COVERED]** | Implicit |

### Process.Alive()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns true if running | **[COVERED]** | Implicit |
| Returns false if done channel closed | **[COVERED]** | Implicit |

### Process.Kill()

| Branch | Status | Notes |
|--------|--------|-------|
| Kills process if cmd and Process non-nil | **[COVERED]** | Spec ExternalPlugin Shutdown: "kill" |
| No-op if cmd or Process is nil | **[COVERED]** | Defensive |

### readStdout (unexported goroutine)

| Branch | Status | Notes |
|--------|--------|-------|
| Routes view/action/ready to syncResp | **[COVERED]** | Spec Process: "view/action/ready -> syncResp, everything else -> asyncCh" |
| Routes other types to asyncCh | **[COVERED]** | Same |
| Drops message if syncResp full (non-blocking send) | **[COVERED]** | Spec Key Decision 2: "Sync/async split" |
| Drops message if asyncCh full | **[COVERED]** | Implicit in channel capacity |
| Invalid JSON logged and skipped | **[COVERED]** | Spec protocol.md Edge Cases: "not valid JSON, it logs the error and ignores the line" |
| 1MB scanner buffer | **[UNCOVERED-IMPLEMENTATION]** | Implementation detail |

### readStderr (unexported goroutine)

| Branch | Status | Notes |
|--------|--------|-------|
| Logs each stderr line as warning | **[COVERED]** | Spec Process: "Stderr reader goroutine logs each line as a warning" |

---

## internal/external/external.go

**Spec:** `specs/plugin/external-adapter.md`

### ExternalPlugin.Init(ctx)

| Branch | Status | Notes |
|--------|--------|-------|
| Saves context, calls startProcess | **[COVERED]** | Spec ExternalPlugin table: Init row |

### ExternalPlugin.startProcess()

| Branch | Status | Notes |
|--------|--------|-------|
| Checks command exists via LookPath | **[COVERED]** | Spec Error State / errorView: "not found on PATH" |
| Sets errState if command not found | **[COVERED]** | Spec Error State |
| Starts process, sends init with db_path/width/height | **[COVERED]** | Spec Init Handshake steps 1-2 |
| Waits 5s for ready response | **[COVERED]** | Spec Init Handshake step 3 |
| Kills process if not ready | **[COVERED]** | Spec: implied by "reject" |
| Kills process if ready type wrong | **[COVERED]** | Spec Init Handshake step 3 |
| Sends scoped config after ready | **[COVERED]** | Spec Init Handshake steps 5-6 |
| Validates migration SQL | **[COVERED]** | Spec Init Handshake step 7 |
| Runs migrations | **[COVERED]** | Spec Init Handshake step 8 |
| Clears errState on success | **[COVERED]** | Spec Init Handshake step 9 |
| Parses routes, key_bindings, migrations from ready | **[COVERED]** | Spec Init Handshake step 4 |

### ExternalPlugin.Shutdown()

| Branch | Status | Notes |
|--------|--------|-------|
| No-op if proc is nil | **[COVERED]** | Defensive, implied |
| Sends shutdown message | **[COVERED]** | Spec ExternalPlugin table: "Send shutdown" |
| Waits 2s then kills | **[COVERED]** | Spec ExternalPlugin table: "wait 2s, kill" |
| Returns immediately if process exits within 2s | **[COVERED]** | Spec: "process exits cleanly" |

### ExternalPlugin.View(width, height, frame)

| Branch | Status | Notes |
|--------|--------|-------|
| Returns errorView if in error state | **[COVERED]** | Spec Error State: "View() returns an error panel" |
| Sends render, receives view (50ms timeout) | **[COVERED]** | Spec ExternalPlugin table: "Send render, receive view (50ms timeout)" |
| Returns cached view on timeout | **[COVERED]** | Spec Key Decision 3: "On timeout, return cached view (no flicker)" |
| Sets error state on process death during render | **[COVERED]** | Spec Key Decision 3: "On process death, set error state" |
| Updates cachedView when view received | **[COVERED]** | Implied by "cached fallback" |
| Sets error state if send fails | **[COVERED]** | Implicit |

### ExternalPlugin.HandleKey(msg)

| Branch | Status | Notes |
|--------|--------|-------|
| In error state + "r" key: restart process | **[COVERED]** | Spec Key Decision 4: "Pressing r calls startProcess()" |
| In error state + other key: NoopAction | **[COVERED]** | Spec Error State: "HandleKey() only responds to r" |
| Normal: sends key, receives action (50ms) | **[COVERED]** | Spec ExternalPlugin table |
| Send error: NoopAction | **[COVERED]** | Implicit |
| Receive timeout: NoopAction | **[COVERED]** | Implicit |
| Response type "action": converts to plugin.Action | **[COVERED]** | Spec ExternalPlugin table |

### ExternalPlugin.HandleMessage(msg)

| Branch | Status | Notes |
|--------|--------|-------|
| WindowSizeMsg: updates dimensions, returns unhandled | **[COVERED]** | Spec ExternalPlugin table: "Update dimensions on WindowSizeMsg" |
| proc nil or error state: skip drain | **[COVERED]** | Spec Error State: "HandleMessage() skips async drain" |
| Drains async: events published to bus with slug prefix | **[COVERED]** | Spec ExternalPlugin table and Key Decision 7 |
| Drains async: logs routed to logger by level | **[COVERED]** | Spec ExternalPlugin table |

### ExternalPlugin.Refresh()

| Branch | Status | Notes |
|--------|--------|-------|
| Returns tea.Cmd that sends refresh | **[COVERED]** | Spec ExternalPlugin table: "Return tea.Cmd that sends refresh (fire-and-forget)" |
| No-op if proc nil or error state | **[COVERED]** | Defensive |

### ExternalPlugin.NavigateTo(route, args)

| Branch | Status | Notes |
|--------|--------|-------|
| Sends navigate message | **[COVERED]** | Spec ExternalPlugin table: "Send navigate message" |
| No-op if proc nil or error state | **[COVERED]** | Defensive |

### ExternalPlugin.errorView()

| Branch | Status | Notes |
|--------|--------|-------|
| "not found on PATH" shows "not installed" variant | **[COVERED]** | Spec Loader: "For exit status 127, a hint is shown that the command was not found on PATH" |
| Other errors show "crashed" variant | **[COVERED]** | Spec Error State |
| Falls back through slug -> tabName -> command for display name | **[UNCOVERED-BEHAVIORAL]** | Not documented. **Intent question: Should spec mention the name fallback chain in error views?** |

---

## internal/external/loader.go

**Spec:** `specs/plugin/external-adapter.md`

### LoadExternalPlugins(cfg, ctx)

| Branch | Status | Notes |
|--------|--------|-------|
| Skips disabled entries | **[COVERED]** | Spec Loader: "skips disabled/empty entries" |
| Skips entries with empty command | **[COVERED]** | Spec Loader: "skips disabled/empty entries" |
| Init failure: logs warning, keeps plugin in list with error state | **[COVERED]** | Spec Loader: "Failures are logged but the plugin is kept in the list with its error state set" |
| Uses entry.Name as slug fallback when init fails | **[COVERED]** | Spec Loader: "If the slug wasn't set during init, the plugin's configured name is used as the slug" |
| Rejects reserved slugs (sessions, commandcenter, settings) | **[COVERED]** | Spec Loader: "Reserved slug check" |
| Rejects duplicate slugs | **[COVERED]** | Spec Loader: "Uniqueness check" |
| Rejected plugins are shut down and excluded | **[COVERED]** | Spec Loader: "shut down and excluded from the list entirely" |
| resolveCommand returns command as-is | **[COVERED]** | Spec implies full binary name required |

---

## Spec-to-Code Direction (Spec claims not in code)

### specs/plugin/protocol.md

| Spec Claim | Status | Notes |
|------------|--------|-------|
| "Unknown message types: Both host and plugin MUST ignore message types they do not recognize" | **OK** | Code's readStdout default case routes unknown types to asyncCh (ignored by HandleMessage unless "event" or "log"). Not explicitly ignored but effectively no-op. |
| "Slow plugins: If plugin does not respond to render within 2 seconds, host displays loading... placeholder" | **NOT IN CODE** | Code uses 50ms timeout and returns cached view, not a "loading..." placeholder. 2-second timeout is not implemented. Spec says "2 seconds" but code uses 50ms. This is an **outdated spec claim** -- the 50ms timeout with cached-view fallback is the actual design. |

### specs/plugin/event-bus.md

| Spec Claim | Status | Notes |
|------------|--------|-------|
| Starter interface documented in event-bus spec | **OK** | Code has Starter in plugin.go; spec documents it in event-bus.md (arguably wrong location but covered) |

---

## Contradictions

1. **Event.Payload type mismatch** -- Spec `event-bus.md` declares `Payload map[string]interface{}`, code uses `Payload any`. Code is intentionally more flexible. **Recommendation:** Update spec to `any`.

2. **ReturnMsg fields** -- Spec `lifecycle.md` declares `ReturnMsg struct{}` (empty). Code has `TodoID string` and `WasResumeJoin bool`. **Recommendation:** Update spec to include both fields with documentation of when they are set.

---

## Behavioral Gaps (no spec coverage)

1. **ActionConsumed** -- Distinct from ActionNoop: tells host not to apply default key handling. Used by plugins to "swallow" keys.

2. **ConsumedAction() helper** -- Constructor for ActionConsumed, parallels NoopAction().

3. **SettingsProvider.SettingsOpenCmd()** -- Async initialization when settings pane opens (e.g., live credential checks).

4. **SettingsProvider.HandleSettingsMsg()** -- Routes async tea.Msg results back to the settings provider.

5. **Context.AgentRunner** -- Agent runner dependency injected into plugins.

6. **DoctorProvider interface + types** -- Entire health-check/doctor system (ValidationResult, DoctorCheck, DoctorOpts, DoctorProvider) has no spec. Used by settings for credential diagnostics.

7. **ScopeConfig nil-cfg guard** -- Defensive nil check, possibly worth documenting.

8. **ScopeConfig skips untagged fields** -- Fields without yaml tags are never exposed to external plugins.

9. **errorView name fallback chain** -- slug -> tabName -> command when rendering error views.

10. **Render timeout is 50ms not 2s** -- Spec protocol.md claims 2-second render timeout; code uses 50ms with cached-view fallback.
