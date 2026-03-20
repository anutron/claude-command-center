# SPEC: Automations Framework

## Purpose

Provides a mechanism for personal headless scripts that run during `ccc-refresh` cycles. Automations are separate from UI plugins — they have no tab, no visual presence, and no bubbletea dependency. They are short-lived subprocesses (seconds, not hours) that perform small tasks like syncing data, sending notifications, or updating external systems based on CCC state.

## Interface

- **Inputs**: `automations:` section from `config.yaml`, scoped config per automation, DB path
- **Outputs**: Result status + message per automation, log entries
- **Dependencies**: `ccc-refresh` (host), SQLite DB, external executables (any language)

## Config Schema

The `automations:` section in `config.yaml` defines available automations:

```yaml
automations:
  - name: "daily-summary"
    command: "/path/to/daily-summary"
    enabled: true
    schedule: "daily_9am"
    config_scopes:
      - "slack"
      - "calendar"
    settings:
      channel: "#general"
      format: "brief"

  - name: "pr-cleanup"
    command: "/path/to/pr-cleanup"
    enabled: false
    schedule: "every_refresh"
    config_scopes:
      - "github"
    settings: {}
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | — | Unique identifier for the automation |
| `command` | string | yes | — | Path to executable (absolute or `$PATH`-resolvable) |
| `enabled` | bool | no | `true` | Whether this automation is active |
| `schedule` | string | no | `"every_refresh"` | When to run (see Scheduling) |
| `config_scopes` | []string | no | `[]` | Which config sections the automation receives |
| `settings` | map[string]any | no | `{}` | Arbitrary key-value settings passed to the automation |

## Protocol Messages

Communication uses JSON-lines over stdin (host→automation) and stdout (automation→host), matching the existing external plugin protocol pattern.

### Host → Automation

#### init

Sent once after the subprocess starts. The automation must respond with `ready` before the run timeout begins.

```json
{
  "type": "init",
  "db_path": "/Users/aaron/.config/ccc/data/ccc.db",
  "config": {
    "slack": { "bot_token": "xoxb-..." },
    "calendar": { "calendar_ids": ["primary"] }
  },
  "settings": {
    "channel": "#general",
    "format": "brief"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"init"` |
| `db_path` | string | Absolute path to the CCC SQLite database (read-only recommended) |
| `config` | object | Scoped config — only sections listed in `config_scopes` |
| `settings` | object | The `settings` map from config.yaml, passed through verbatim |

#### run

Sent after receiving `ready`. Signals the automation to perform its work.

```json
{
  "type": "run"
}
```

#### shutdown

Sent after receiving `result`, or on timeout. The automation should exit promptly.

```json
{
  "type": "shutdown"
}
```

### Automation → Host

#### ready

Acknowledges init. Must be sent before the host sends `run`.

```json
{
  "type": "ready"
}
```

#### result

Reports the outcome of the run. Must be sent exactly once after receiving `run`.

```json
{
  "type": "result",
  "status": "success",
  "message": "Posted daily summary to #general"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"result"` |
| `status` | string | One of: `"success"`, `"error"`, `"skipped"` |
| `message` | string | Human-readable description of what happened |

#### log

Optional. Can be sent at any time between `ready` and `result` for diagnostic output.

```json
{
  "type": "log",
  "level": "info",
  "message": "Fetching calendar events for today"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"log"` |
| `level` | string | One of: `"debug"`, `"info"`, `"warn"`, `"error"` |
| `message` | string | Log line content |

## Lifecycle

Each automation runs through a fixed sequence per invocation:

1. **Spawn** — Host starts the subprocess via `command`
2. **Init** — Host sends `init` with scoped config, DB path, and settings
3. **Ready** — Automation responds with `ready` (must arrive within 5s or the automation is killed)
4. **Run** — Host sends `run`
5. **Execute** — Automation performs its work, optionally sending `log` messages
6. **Result** — Automation sends `result` with status and message
7. **Shutdown** — Host sends `shutdown`, waits up to 2s, then kills the process

**Total timeout**: 30 seconds from spawn to result. If the automation does not produce a `result` within 30s, the host kills the process and records status `"error"` with message `"timeout after 30s"`.

## Scheduling

### Schedule Values

| Value | Description |
|-------|-------------|
| `every_refresh` | Run on every `ccc-refresh` cycle |
| `daily` | Run once per calendar day (first refresh of the day) |
| `daily_9am` | Run once per day, only if current time is 9:00 AM or later |
| `weekly_monday` | Run once per week, only on Monday |
| `weekly_friday` | Run once per week, only on Friday |

### isDue Logic

The runner evaluates whether an automation is due by checking the `cc_automation_runs` table for the most recent successful or skipped run:

- `every_refresh` — always due
- `daily` — due if no run today (comparing dates in local time)
- `daily_9am` — due if no run today AND current local time >= 09:00
- `weekly_monday` — due if no run this calendar week AND today is Monday
- `weekly_friday` — due if no run this calendar week AND today is Friday

An unknown schedule value is skipped with a warning logged (forward-compatible — new schedule types can be added without breaking older runners).

### Tracking Table

```sql
CREATE TABLE IF NOT EXISTS cc_automation_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    started_at  TEXT NOT NULL,  -- RFC3339
    finished_at TEXT NOT NULL,  -- RFC3339
    status      TEXT NOT NULL,  -- "success", "error", "skipped"
    message     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_automation_runs_name_started
    ON cc_automation_runs(name, started_at);
```

## Security Model

- **Scoped config**: Automations receive only the config sections listed in their `config_scopes`, not the full `config.yaml`. An automation with `config_scopes: ["slack"]` cannot see GitHub tokens or calendar credentials.
- **Subprocess isolation**: Automations run as child processes with the same user permissions as `ccc-refresh`. No additional sandboxing is applied.
- **DB access**: The DB path is provided for read access. Automations should not write to CCC tables (the host tracks run results). If an automation needs its own persistent state, it should use its own namespaced table or external storage.
- **No network restrictions**: Automations can make any network calls the user's account permits.

## Behavior

Given a `ccc-refresh` cycle:

1. Load the `automations:` list from config
2. Filter to `enabled: true` automations
3. For each enabled automation, evaluate `isDue` against `cc_automation_runs`
4. Run due automations **sequentially** (not in parallel) to avoid resource contention
5. For each due automation:
   a. Record `started_at`
   b. Spawn the subprocess via `command`
   c. Send `init` with scoped config, DB path, and settings
   d. Wait for `ready` (5s timeout)
   e. Send `run`
   f. Collect `log` messages and forward to the refresh logger
   g. Wait for `result` (30s total timeout from spawn)
   h. Send `shutdown`
   i. Record `finished_at`, `status`, and `message` in `cc_automation_runs`
6. Continue with the rest of the refresh cycle (automations do not block data fetching — they run after data is saved)

### Execution Order

Automations run **after** the main refresh data has been saved to the database. This ensures automations can read fresh data (calendar events, todos, threads) from the DB.

### Error Handling

- If an automation's `command` is not found or not executable, record status `"error"` with message `"command not found: /path/to/cmd"` and continue to the next automation.
- If `ready` is not received within 5s, kill the process and record status `"error"` with message `"init timeout"`.
- If `result` is not received within 30s of spawn, kill the process and record status `"error"` with message `"timeout after 30s"`.
- If the subprocess exits with a non-zero code before sending `result`, record status `"error"` with the stderr output (truncated to 500 bytes).
- Errors in one automation do not affect other automations.

## Test Cases

- **Happy path**: Automation receives init, responds ready, receives run, sends result with success — run recorded in `cc_automation_runs` with correct timestamps and status
- **Error result**: Automation sends `result` with `status: "error"` — recorded as-is, does not affect other automations
- **Timeout**: Automation hangs after `run` — killed after 30s, recorded as error with timeout message
- **Init timeout**: Automation never sends `ready` — killed after 5s, recorded as error
- **Disabled**: Automation with `enabled: false` — skipped entirely, no run recorded
- **Schedule not due**: Daily automation already ran today — skipped, no run recorded
- **Unknown schedule**: Automation with `schedule: "banana"` — skipped with a warning logged
- **Command not found**: `command` points to nonexistent file — recorded as error, next automation still runs
- **Scoped config**: Automation with `config_scopes: ["slack"]` — receives only Slack config, not calendar or GitHub
- **Empty automations list**: No automations configured — runner completes immediately with no errors
- **Sequential execution**: Three automations configured — they run one after another, not concurrently
