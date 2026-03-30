# Spec Audit: internal/automation

**Date:** 2026-03-29
**Spec:** specs/core/automations.md

---

## internal/automation/runner.go

### Runner struct

- **[COVERED]** automations.md "Interface": Automations from config, Config, DBPath, Logger
- **[UNCOVERED-BEHAVIORAL]** `LogPath` field for log rotation. Not in spec. **Intent question:** Should log rotation be documented?
- **[UNCOVERED-BEHAVIORAL]** `nowFunc` for test clock injection. Impl detail but the rotation behavior it enables is behavioral.

### RunAll

- **[COVERED]** automations.md "Behavior" steps 2-5: filter enabled, check isDue, run sequentially, record results
- **[COVERED]** automations.md: "disabled → skipped entirely, no run recorded" — code returns `{Status: "skipped", Message: "disabled"}`
- **[COVERED]** automations.md: "not due → skipped, no run recorded" — code returns `{Status: "skipped", Message: "not due"}`
- **[UNCOVERED-BEHAVIORAL]** `rotateLog()` called at start of RunAll — rotates automation log if >7 days old or >5MB. Not in spec.

### runOne

- **[COVERED]** automations.md "Lifecycle" steps 1-7: spawn, init, ready, run, execute, result, shutdown
- **[COVERED]** automations.md: "Total timeout: 30 seconds from spawn to result"
- **[COVERED]** automations.md: "Ready must arrive within 5s or the automation is killed"
- **[COVERED]** automations.md: "Scoped config — only sections listed in config_scopes"
- **[COVERED]** automations.md: "log messages forwarded to the refresh logger"
- **[UNCOVERED-BEHAVIORAL]** Log messages during init phase (between init send and ready receipt) are consumed and forwarded. Spec doesn't explicitly mention log messages during init, only "between ready and result". **Intent question:** Clarify that log messages can arrive during init?
- **[COVERED]** automations.md: "If ready is not received within 5s, kill the process and record status 'error' with message 'init timeout'"
- **[COVERED]** automations.md: "stderr output (truncated to 500 bytes)" — code uses `proc.stderrOutput(500)`

### getLastRun / recordRun

- **[COVERED]** automations.md "Tracking Table" and isDue logic: queries cc_automation_runs, records with RFC3339 timestamps

### rotateLog

- **[UNCOVERED-BEHAVIORAL]** Log rotation: >7 days or >5MB, keeps one previous rotation (.1). Not in spec. **Intent question:** Should log rotation be documented in the automations spec?

### EnsureTable

- **[COVERED]** automations.md "Tracking Table": CREATE TABLE IF NOT EXISTS with exact schema match

### LogDir

- **[UNCOVERED-IMPLEMENTATION]** Returns DataDir/logs — simple path helper

---

## internal/automation/protocol.go

### HostMsg / ResultMsg

- **[COVERED]** automations.md "Protocol Messages": all fields match spec exactly
- Host messages: init (db_path, config, settings), run (trigger), shutdown
- Result messages: ready, result (status, message), log (level, message)

---

## internal/automation/process.go

### startProcess

- **[COVERED]** automations.md: "command" field used with subprocess. Code uses `sh -c <command>`.
- **[UNCOVERED-BEHAVIORAL]** `SysProcAttr{Setpgid: true}` — creates process group. Not in spec but enables group kill.

### send / receive

- **[COVERED]** automations.md "Protocol Messages": JSON-lines over stdin/stdout

### kill

- **[UNCOVERED-BEHAVIORAL]** Kills the process group (`syscall.Kill(-pid, SIGKILL)`) to ensure child processes are terminated. Not explicitly in spec. **Intent question:** Document process group kill?

### stderrOutput

- **[COVERED]** automations.md: "stderr output (truncated to 500 bytes)"

---

## internal/automation/schedule.go

### isDue

- **[COVERED]** automations.md "Schedule Values" and "isDue Logic": every_refresh, hourly, daily, daily_9am, weekly_monday, weekly_friday — all match
- **[COVERED]** automations.md: "Unknown schedule value is skipped" — code returns false for default case
- **[UNCOVERED-BEHAVIORAL]** Unknown schedule returns `false` silently — spec says "skipped with a warning logged" but the code does not log a warning at the isDue level (the caller logs "not due"). Minor discrepancy.

### weeklyDue

- **[COVERED]** automations.md: "weekly_monday/friday — due if today is the target weekday and no run has occurred since the most recent occurrence"

---

## Spec -> Code Direction Gaps

1. **automations.md "Settings Page"** describes a read-only settings view — implemented in settings plugin (not in automation package). No code gap.
2. **automations.md "Security Model"** — scoped config verified in runner.go via `plugin.ScopeConfig`. No gap.
3. **automations.md "Execution Order"** — "run after main refresh data has been saved" — verified in both cmd/ai-cron/main.go and cmd/ccc/daemon_cmd.go.

---

## Summary

- **CONTRADICTS: 0** (minor: unknown schedule warning spec vs silent skip in code — borderline)
- **UNCOVERED-BEHAVIORAL: 4** — Log rotation, LogPath field, log messages during init phase, process group kill
- **COVERED: ~20 behavioral paths** — Well-covered module. Spec and code are closely aligned.
