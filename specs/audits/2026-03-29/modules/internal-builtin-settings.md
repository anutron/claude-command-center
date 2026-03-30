# Spec Audit: internal/builtin/settings

**Date:** 2026-03-29
**Spec:** `specs/builtin/settings.md`
**Code files:** `settings.go`, `nav.go`, `content.go`, `auth_form.go`, `content_agent.go`, `content_appearance.go`, `content_automations.go`, `content_budget.go`, `content_daemon.go`, `content_datasources.go`, `content_logs.go`, `content_plugins.go`, `content_prs.go`, `content_system.go`, `styles.go`

## Coverage Summary

- **Total exported behaviors analyzed:** 58
- **COVERED:** 46
- **UNCOVERED-BEHAVIORAL:** 8
- **UNCOVERED-IMPLEMENTATION:** 3
- **CONTRADICTS:** 1

## Exported Functions/Methods

### Plugin Core (settings.go)

#### New(registry) / Slug() / TabName()

- **[COVERED]** — Spec Constructor: `settings.New(registry *plugin.Registry) *Plugin`. Slug and tab name match.

#### Plugin.Init(ctx)

- **[COVERED]** — Spec Dependencies section lists all dependencies initialized here.
- **[COVERED]** — Code registers calendar, github, granola providers. Spec Data Sources section documents provider interactivity.
- **[UNCOVERED-BEHAVIORAL]** — Code subscribes to todo events (`todo.completed`, `todo.created`, `todo.dismissed`, `todo.deferred`, `todo.promoted`, `todo.edited`) for logging. Spec does not mention any event bus subscriptions. **Intent question:** Should the spec document that settings subscribes to todo events for logging purposes?

#### Plugin.SetDaemonClientFunc(fn)

- **[COVERED]** — Spec Dependencies: "daemon — daemon process lifecycle (start) and client RPC."

#### Plugin.RegisterProvider(slug, sp)

- **[UNCOVERED-BEHAVIORAL]** — Exported method for external registration of SettingsProviders. Not mentioned in spec. **Intent question:** Should the spec document the RegisterProvider API for external provider registration?

#### Plugin.Routes()

- **[COVERED]** — Code returns single route `{Slug: "settings"}`. Spec does not list routes explicitly but the single-route nature is implicit.

#### Plugin.NavigateTo(route, args)

- **[COVERED]** — Spec: "External navigation just activates the settings tab." Code is a no-op, matching.

#### Plugin.KeyBindings()

- **[COVERED]** — Spec Key Bindings table matches all promoted bindings.

#### Plugin.HandleKey(msg)

- **[COVERED]** — Spec Focus Zones and Key Bindings sections document routing by zone.

#### Plugin.HandleMessage(msg)

- **[COVERED]** — Spec mentions async message routing for providers, system action results, auth flow results.
- **[COVERED]** — Code handles `TabLeaveMsg` with auto-save and auth cancellation.
- **[COVERED]** — Code clears flash after 10 seconds.
- **[UNCOVERED-BEHAVIORAL]** — Flash message timeout is hardcoded to 10 seconds. Spec does not specify flash duration. **Intent question:** Is the 10-second flash timeout worth documenting?

#### Plugin.View(width, height, frame)

- **[COVERED]** — Spec Layout section: "Sidebar (left) + content pane (right)."
- **[COVERED]** — Spec describes help line per focus zone.

### Sidebar Navigation (nav.go)

#### rebuildNav()

- **[COVERED]** — Spec Sidebar Categories section lists all 5 categories and their items. Code matches.
- **[CONTRADICTS]** — Spec Sidebar Categories lists PLUGINS as "Sessions, Command Center, Threads, Pomodoro, external plugins." Code shows the plugin registry contents which would include whatever is registered. The spec lists "Threads" which does not appear in code — code uses `commandcenter` and `prs` slugs. **Recommendation:** Update spec PLUGINS category to match actual registry slugs, not hardcoded names.

#### navItemCount() / selectedNavItem()

- **[UNCOVERED-IMPLEMENTATION]** — Internal navigation helpers.

#### navCursorUp() / navCursorDown()

- **[COVERED]** — Spec Key Bindings: "up/down: Navigate sidebar."

#### ensureCursorVisible(visibleHeight)

- **[COVERED]** — Spec Sidebar Scrolling section.

#### viewSidebar(width, height, focus)

- **[COVERED]** — Spec describes toggle indicators, validation indicators, cursor styling, category headers, scroll windowing.

#### handleNavKey(msg)

- **[COVERED]** — Spec Key Bindings FocusNav row and Navigation Between Zones section.
- **[COVERED]** — Spec: "enter/right on Logs item" routes to FocusLogs.
- **[COVERED]** — Spec: "For Google datasources, fire an async live credential check on pane open."

#### applyNavToggle(item)

- **[COVERED]** — Spec Sidebar Toggle Behavior section documents all toggle behaviors.
- **[COVERED]** — Spec: "Data sources: validates credentials first; reverts on failure."
- **[COVERED]** — Code publishes `datasource.toggled` event on data source toggle.
- **[UNCOVERED-BEHAVIORAL]** — Code publishes `datasource.toggled` event with `{name, enabled}` payload. Spec Sidebar Toggle Behavior does not mention this event. **Intent question:** Should the spec document the `datasource.toggled` event?

### Content Pane (content.go)

#### viewContent(width, height)

- **[COVERED]** — Spec Content Pane Header section.
- **[COVERED]** — Spec Content Pane Preview section.

#### viewActiveFormContent(width, height)

- **[COVERED]** — Spec Data Sources: "Provider views are rendered above the form."

#### viewPreviewContent(item, width, height)

- **[COVERED]** — Spec Content Pane Preview: "builds a transient form via `buildFormForSlug`, calls `Init()` on it."

#### renderPaneHeader(title, description)

- **[UNCOVERED-IMPLEMENTATION]** — Rendering helper.

### Auth Forms (auth_form.go)

#### newClientCredForm(theme) / newSlackTokenForm(theme)

- **[COVERED]** — Spec Data Sources actions: "Authenticate (enter client credentials + OAuth)" and "Enter Slack token."
- **[COVERED]** — Spec Slack Token section: "The token form says 'Slack User Token' with 'Starts with xoxp-'."

### Appearance (content_appearance.go)

#### buildBannerForm()

- **[COVERED]** — Spec Banner pane: "Name — text input, 20 char limit" etc. All 4 fields match.

#### saveBannerValues()

- **[COVERED]** — Spec Auto-Save Behavior and Banner pane: "Saves to config on field transition and pane exit."

#### handleBannerFormCompletion()

- **[COVERED]** — Spec Banner pane: "Publishes `config.saved` event."

#### buildPaletteForm()

- **[COVERED]** — Spec Palette pane: "Color Palette — select with palette names, '(active)' suffix on current." "Preview — note with live color swatches via `DescriptionFunc`."

#### savePaletteValues()

- **[COVERED]** — Spec Palette: "On completion: applies palette, rebuilds all styles (settings, shared, gradient), publishes `palette.changed` event."

#### handlePaletteFormCompletion()

- **[COVERED]** — Same as above.

#### renderSwatches(pal)

- **[UNCOVERED-IMPLEMENTATION]** — Rendering helper for palette preview.

### Agent Sandbox (content_agent.go)

#### buildAgentSandboxForm()

- **[UNCOVERED-BEHAVIORAL]** — The spec's AGENT category lists "Daemon, Budget, Sandbox" but the Pane Details section only documents Daemon and Budget. The Sandbox pane (read-only form showing write paths, extra write paths, and autonomous allowed domains) has no spec coverage. **Intent question:** Should the spec document the Sandbox pane content (write learned paths toggle, additional write paths, autonomous allowed domains)?

### Automations (content_automations.go)

#### viewAutomationsContent(width, height)

- **[UNCOVERED-BEHAVIORAL]** — The spec lists "Automations" in the SYSTEM sidebar category but the Pane Details section does not describe the automations pane. Code renders a table of automations with name, schedule, status, last run, and message columns. **Intent question:** Should the spec document the Automations pane layout and behavior?

#### queryLastAutomationRun(name) / statusStyle(status) / relativeTime(t)

- **[UNCOVERED-IMPLEMENTATION]** — Internal helpers for automations rendering.

### Budget (content_budget.go)

#### buildAgentBudgetForm()

- **[COVERED]** — Spec Budget pane details all three groups precisely. Fields, validation ranges, and daemon query fallback messages all match.

#### saveBudgetValues()

- **[COVERED]** — Spec: "Saves to `config.Agent.*` on field transition (auto-save via `saveBudgetValues()`)."

#### handleBudgetFormCompletion()

- **[COVERED]** — Spec: "Publishes `config.saved` event with source `'agent-budget'`."

### Daemon (content_daemon.go)

#### getDaemonState()

- **[COVERED]** — Spec Daemon pane: "Queries the daemon via `daemonClientFunc` on each form build."

#### buildDaemonForm()

- **[COVERED]** — Spec Daemon pane documents status display, contextual actions, and live info group.

#### saveDaemonValues()

- **[COVERED]** — Spec: "Auto-save on field transition calls `saveDaemonValues()` which applies the action immediately."

#### handleDaemonFormCompletion()

- **[COVERED]** — Spec: "waits 300ms for the daemon to transition, then rebuilds the form."

#### applyDaemonAction(action)

- **[COVERED]** — Spec Daemon pane documents all action outcomes.

### Data Sources (content_datasources.go)

#### buildDatasourceForm(item)

- **[COVERED]** — Spec Data Sources action options.

#### handleDatasourceFormCompletion(slug)

- **[COVERED]** — Spec Data Sources actions: recheck, auth, console.

#### viewValidationStatus(item)

- **[COVERED]** — Spec Data Sources Credential verification section. Tiered status rendering matches.

#### isGoogleDatasource(slug)

- **[COVERED]** — Spec distinguishes Google sources (calendar, gmail) from others.

#### startAuthFlowForDatasource()

- **[COVERED]** — Spec Data Sources: "Authenticate (enter client credentials + OAuth)."

#### loadExistingGoogleCreds(slug)

- **[COVERED]** — Code tries to reuse existing credentials from token file. Spec mentions "Authenticate" action but not the reuse optimization.
- **[UNCOVERED-BEHAVIORAL]** — Code silently reuses existing Google credentials from the token file when re-authenticating, skipping the credential form. Spec doesn't document this optimization. **Intent question:** Should the spec document the credential reuse behavior (auto-detect existing creds, skip form, go straight to OAuth)?

### Logs (content_logs.go)

#### filteredLogEntries()

- **[COVERED]** — Spec Logs section mentions `/ filter`.

#### viewLogsContent(width, height)

- **[COVERED]** — Spec Logs section documents all key bindings and filter mode.

#### handleLogsContentKey(msg)

- **[COVERED]** — Spec Key Bindings FocusLogs row and Logs section.

### Plugins (content_plugins.go)

#### buildPluginForm(item)

- **[COVERED]** — Spec Plugins section: "Plugin panes show info via a huh Note."

#### handlePluginFormCompletion(slug)

- **[COVERED]** — Form rebuilds on completion.

### PRs (content_prs.go)

#### buildPRSettingsForm()

- **[UNCOVERED-BEHAVIORAL]** — The PR settings form shows ignored repos and ignored PRs. The spec does not mention a PR-specific settings pane. The spec's PLUGINS category would include "prs" but the Plugins pane details don't describe PR-specific content. **Intent question:** Should the spec document the PR settings pane (ignored repos list, ignored PRs list)?

### System Panes (content_system.go)

#### buildScheduleForm() / handleScheduleFormCompletion()

- **[COVERED]** — Spec System Panes: "Status note — current install/build status" and "ACTIONS select — Install/Uninstall."

#### buildMCPForm() / handleMCPFormCompletion()

- **[COVERED]** — Spec System Panes: "Build & Configure for MCP."

#### buildSkillsForm() / handleSkillsFormCompletion()

- **[COVERED]** — Spec System Panes documents install/uninstall actions.

#### buildShellForm() / handleShellFormCompletion()

- **[COVERED]** — Spec System Panes documents install/uninstall actions.

#### handleSystemActionResult(msg)

- **[COVERED]** — Spec: "The form rebuilds after the async action completes to show updated status."

### Form Routing (settings.go)

#### buildFormForSlug(item)

- **[COVERED]** — Spec Form-Based Content Panes section.

#### saveFormValues(slug)

- **[COVERED]** — Spec Auto-Save Behavior: "banner, palette, agent-budget" auto-save on field transition.
- **[UNCOVERED-BEHAVIORAL]** — Code also auto-saves daemon values on field transition (`saveDaemonValues`). Spec Auto-Save section says "The daemon form auto-saves by executing the selected action on field transition." This is documented but `saveFormValues` routing for daemon is not explicit. Minor gap.

#### handleFormCompletion(slug)

- **[COVERED]** — Spec Form Lifecycle #5.

#### shouldAutoSaveOnExit(slug)

- **[COVERED]** — Spec Auto-Save Behavior: "Forms with editable settings (banner, palette) auto-save on pane exit."
- **[CONTRADICTS-MINOR]** — Spec says "Forms with editable settings (banner, palette, agent-budget) auto-save on" exit/transition. Code `shouldAutoSaveOnExit` only returns true for banner and palette, NOT agent-budget. Budget auto-saves on field transition (via `saveFormValues`) but NOT on pane exit. This is a minor spec imprecision — the spec groups them together but they have different exit behaviors.

### Validation (settings.go)

#### validateDataSourceResult(slug, live)

- **[COVERED]** — Spec Data Sources Credential verification section.

#### applyRecheckResult(msg)

- **[COVERED]** — Spec: "Results respect the `Live` flag — a live 'ok' skips the sync-aware downgrade."

#### validateSlackResult() / liveSlackTokenCheck()

- **[COVERED]** — Spec Slack Token section and Credential verification.

### Styles (styles.go)

#### huhThemeFromPalette(pal)

- **[COVERED]** — Spec Huh Theme section documents the mapping.

#### newSettingsStyles(p)

- **[COVERED]** — Spec mentions palette-driven styling.

## Spec-to-Code Direction (Spec claims not found in code)

- **[OK]** — Spec Quit Behavior (double-escape): This is implemented in the TUI host, not in the settings plugin. The plugin returns `ActionUnhandled` on esc from FocusNav, letting the host handle quit logic.
- **[OK]** — Spec "Enabled states sync from live config at the start of each `View()` call": Code has `syncNavFromConfig()` called at the top of `View()`.
- **[OK]** — Spec Flash message after toggle: Code sets `flashMessage` in `applyNavToggle`.

## Summary

The settings spec is thorough and covers the vast majority of the implementation. The main gaps are:

1. **Sandbox pane** (UNCOVERED) — listed in sidebar categories but no pane detail section
2. **Automations pane** (UNCOVERED) — listed in sidebar categories but no pane detail section
3. **PR settings pane** (UNCOVERED) — ignored repos/PRs form exists but not documented
4. **PLUGINS category naming** (CONTRADICTS) — spec lists "Threads" but code uses registry slugs
5. **RegisterProvider API** (UNCOVERED) — exported method for external provider registration
6. **Credential reuse optimization** (UNCOVERED) — re-auth skips form when existing creds found in token file
7. **Event subscriptions** (UNCOVERED) — todo event logging and `datasource.toggled` event not documented
8. **Budget auto-save on exit** (MINOR CONTRADICTS) — spec groups budget with banner/palette for auto-save on exit, but code only auto-saves budget on field transition, not on pane exit
