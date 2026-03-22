# CCC Automations Guide

Build headless scripts that run during `ai-cron` cycles. Automations have no UI footprint — no tab, no bubbletea dependency. They are short-lived subprocesses (seconds, not hours) that perform small tasks like syncing data, sending notifications, or updating external systems based on CCC state.

## Automations vs Plugins

| | Automations | External Plugins |
|---|---|---|
| **Runtime** | `ai-cron` (headless) | `ccc` (TUI) |
| **Lifecycle** | Spawned, runs, exits (seconds) | Long-lived subprocess |
| **UI** | None | Own tab in the TUI |
| **Purpose** | Background tasks, side effects | Interactive features |
| **Protocol** | init/run/result (3 messages) | init/render/key/... (ongoing) |
| **Examples** | Auto-accept calendar invites, post Slack summaries, clean up PRs | Pomodoro timer, custom dashboards |

**Rule of thumb:** if it needs a screen, make it a plugin. If it just needs to run and report back, make it an automation.

## Repository Layout

The automations **framework** lives in this repo. Your actual automations live elsewhere.

```
claude-command-center/          # This repo — the framework
  internal/automation/          # Runner, protocol, scheduling (Go)
  sdk/python/ccc_automation.py  # Python SDK (single file, no deps)
  examples/calendar-accept/     # Reference example / template
  specs/core/automations.md     # Spec (source of truth)

your-automations-repo/          # Your repo — the automations themselves
  my-automation/
    main.py
  another-automation/
    main.py
```

The `examples/calendar-accept/` directory is a **reference build** showing how to structure an automation. It is not the production version of anything — use it as a starting template.

## Configuration

Automations are declared in the `automations:` section of `~/.config/ccc/config.yaml`:

```yaml
automations:
  - name: "calendar-accept"
    command: "python3 /path/to/calendar_accept.py"
    enabled: true
    schedule: "every_refresh"
    config_scopes:
      - "calendar"
    settings:
      dry_run: true
      accept_patterns:
        - "standup"
        - "1:1"

  - name: "daily-summary"
    command: "/path/to/daily-summary"
    enabled: false
    schedule: "daily_9am"
    config_scopes:
      - "slack"
      - "calendar"
    settings:
      channel: "#general"
      format: "brief"
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | -- | Unique identifier for the automation. |
| `command` | string | yes | -- | Path to executable. Can be absolute, `$PATH`-resolvable, or a full shell command. Executed via `sh -c`. |
| `enabled` | bool | no | `true` | Set to `false` to disable without removing the entry. |
| `schedule` | string | no | `"every_refresh"` | When to run. See [Scheduling](#scheduling). |
| `config_scopes` | list of strings | no | `[]` | Which top-level config sections to pass to the automation (e.g., `"slack"`, `"calendar"`, `"github"`). The automation receives **only** these sections, not the full config. |
| `settings` | map | no | `{}` | Arbitrary key-value pairs passed through verbatim to the automation. Use this for automation-specific options. |

## Scheduling

The `schedule` field controls when an automation runs. The runner checks the `cc_automation_runs` tracking table to determine if an automation is due.

| Schedule | Description |
|----------|-------------|
| `every_refresh` | Runs on every `ai-cron` cycle. |
| `hourly` | Runs at most once per hour (due if last run was more than 1 hour ago). |
| `daily` | Runs once per calendar day (due if no run since midnight local time). |
| `daily_9am` | Runs once per day, but only if current local time is 9:00 AM or later. |
| `weekly_monday` | Runs once per week, only on Mondays. |
| `weekly_friday` | Runs once per week, only on Fridays. |

**Unknown schedule values** are silently skipped with a warning logged. This is intentional — new schedule types can be added to the runner without breaking older configs.

**Tracking:** Each run is recorded in the `cc_automation_runs` SQLite table. The runner queries this table to determine whether an automation is due. All statuses (success, error, skipped) are tracked.

## Lifecycle

Each automation runs through a fixed sequence per `ai-cron` cycle:

```
ai-cron                        Automation subprocess
    |                                      |
    |-- spawn (sh -c <command>) ---------->|
    |                                      |
    |-- init {db_path, config, settings} ->|
    |                                      |
    |<--------- ready {} -----------------|
    |                                      |
    |-- run {trigger} ------------------->|
    |                                      |
    |<--------- log {} ------------------|  (optional, 0 or more)
    |                                      |
    |<--------- result {status, message} -|
    |                                      |
    |-- shutdown {} ---------------------->|
    |                                      |  (process exits)
```

**Timing:**

- The automation has **5 seconds** to respond with `ready` after receiving `init`.
- The total timeout from spawn to `result` is **30 seconds**. If the automation does not produce a result in time, the host kills the process.
- After receiving `result`, the host sends `shutdown` and waits up to **2 seconds** before force-killing.

**Execution order:** Automations run **after** the main refresh data has been saved to the database. This means automations can read fresh calendar events, todos, threads, etc. from the DB. Automations run **sequentially** (not in parallel) to avoid resource contention.

## Protocol Reference

Communication uses **JSON-lines** over stdin (host to automation) and stdout (automation to host). One JSON object per line, newline-delimited.

### Host to Automation

#### `init`

Sent once after the subprocess starts.

```json
{
  "type": "init",
  "db_path": "/Users/you/.config/ccc/data/ccc.db",
  "config": {
    "calendar": {
      "enabled": true,
      "calendars": [{"id": "primary", "label": "Work"}]
    }
  },
  "settings": {
    "dry_run": true,
    "accept_patterns": ["standup", "1:1"]
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"init"`. |
| `db_path` | string | Absolute path to the CCC SQLite database. Read-only access recommended. |
| `config` | object | Scoped config -- only sections listed in `config_scopes`. Empty object if no scopes declared. |
| `settings` | object | The `settings` map from config.yaml, passed through verbatim. |

#### `run`

Sent after receiving `ready`. Signals the automation to perform its work.

```json
{
  "type": "run",
  "trigger": "refresh"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"run"`. |
| `trigger` | string | What triggered this run. Currently always `"refresh"`. |

#### `shutdown`

Sent after receiving `result`. The automation should exit promptly.

```json
{
  "type": "shutdown"
}
```

### Automation to Host

#### `ready`

Acknowledges `init`. Must be sent before the host sends `run`.

```json
{
  "type": "ready"
}
```

#### `result`

Reports the outcome of the run. Must be sent **exactly once** after receiving `run`.

```json
{
  "type": "result",
  "status": "success",
  "message": "Accepted 3 of 5 pending invites"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"result"`. |
| `status` | string | One of: `"success"`, `"error"`, `"skipped"`. |
| `message` | string | Human-readable description of what happened. |

#### `log`

Optional. Can be sent any time between `ready` and `result` for diagnostic output.

```json
{
  "type": "log",
  "level": "info",
  "message": "Fetching calendar events for today"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `"log"`. |
| `level` | string | One of: `"debug"`, `"info"`, `"warn"`, `"error"`. |
| `message` | string | Log line content. |

## Python SDK

The Python SDK is a single file (`sdk/python/ccc_automation.py`) with no dependencies beyond stdlib. It handles the JSON-lines protocol so you can focus on your logic.

### Basic Usage

```python
from ccc_automation import CCCAutomation

class MyAutomation(CCCAutomation):
    def on_run(self, trigger, context):
        self.log("info", "Doing my thing")
        return "success", "Did the thing"

if __name__ == "__main__":
    MyAutomation("my-automation").run()
```

### API Reference

#### `CCCAutomation(slug)`

Constructor. `slug` is the unique identifier for your automation.

#### `on_init(self, db_path, config, settings)`

Called after the host sends `init`. Override for setup that needs the database path, scoped config, or settings. The base implementation does nothing.

After `on_init`, the following instance variables are available:

- `self.db_path` -- absolute path to the CCC SQLite database
- `self.config` -- dict of scoped config sections
- `self.settings` -- dict of automation-specific settings from config.yaml

#### `on_run(self, trigger, context)`

Called when the host sends `run`. **You must override this.** Return a `(status, message)` tuple.

- `trigger` -- string describing what triggered the run (currently `"refresh"`)
- `context` -- additional context dict from the host (currently empty)
- Return `("success", "description")`, `("error", "what went wrong")`, or `("skipped", "why")`

If `on_run` raises an exception, the SDK catches it and sends `status: "error"` with the exception message.

#### `log(self, level, message)`

Send a log message to the host. Can be called any time between `ready` and `result`.

- `level` -- one of `"debug"`, `"info"`, `"warn"`, `"error"`
- `message` -- log line content

#### `run(self)`

Main loop. Call this from your `__main__` block. Reads JSON-lines from stdin, dispatches to handlers, blocks until shutdown.

### Importing the SDK

If you are developing automations **outside** the CCC repo, you have two options:

**Option A: Copy the file.** It is a single file with no dependencies. Copy `sdk/python/ccc_automation.py` into your project.

**Option B: sys.path hack.** Point at the SDK in your CCC checkout:

```python
import os, sys
sys.path.insert(0, "/path/to/claude-command-center/sdk/python")
from ccc_automation import CCCAutomation
```

## Tutorial: Build Your First Automation

This walkthrough builds a simple automation that checks the CCC database and reports back. We will use the `examples/calendar-accept/` reference example as a guide.

### Step 1: Create your project

```bash
mkdir ~/my-automation && cd ~/my-automation
```

### Step 2: Copy the SDK

```bash
cp /path/to/claude-command-center/sdk/python/ccc_automation.py ~/my-automation/
```

### Step 3: Write your automation

Create `main.py`:

```python
#!/usr/bin/env python3
"""My first CCC automation."""

import sqlite3
from ccc_automation import CCCAutomation


class MyAutomation(CCCAutomation):

    def on_init(self, db_path, config, settings):
        self.threshold = self.settings.get("threshold", 5)
        self.log("info", f"Initialized with threshold={self.threshold}")

    def on_run(self, trigger, context):
        if not self.db_path:
            return "error", "No database path provided"

        # Open the DB read-only
        conn = sqlite3.connect(f"file:{self.db_path}?mode=ro", uri=True)
        conn.row_factory = sqlite3.Row

        cursor = conn.cursor()
        cursor.execute("""
            SELECT COUNT(*) as cnt FROM cc_calendar_events
            WHERE start_time > datetime('now')
              AND start_time < datetime('now', '+1 day')
        """)
        count = cursor.fetchone()["cnt"]
        conn.close()

        if count >= self.threshold:
            self.log("warn", f"Heavy day: {count} events")
            return "success", f"Warning: {count} events today (threshold={self.threshold})"

        return "success", f"{count} events today, under threshold"


if __name__ == "__main__":
    MyAutomation("my-automation").run()
```

### Step 4: Make it executable

```bash
chmod +x ~/my-automation/main.py
```

### Step 5: Register in config.yaml

Add to `~/.config/ccc/config.yaml`:

```yaml
automations:
  - name: "my-automation"
    command: "python3 ~/my-automation/main.py"
    enabled: true
    schedule: "daily_9am"
    config_scopes: []
    settings:
      threshold: 8
```

### Step 6: Test manually

You can test your automation by piping JSON to stdin:

```bash
# Start the automation
python3 ~/my-automation/main.py

# Paste this line (init):
{"type":"init","db_path":"/Users/you/.config/ccc/data/ccc.db","config":{},"settings":{"threshold":8}}

# You should see: {"type": "ready", "slug": "my-automation"}
# Paste this line (run):
{"type":"run","trigger":"refresh"}

# You should see a result message. Then:
{"type":"shutdown"}
```

### Step 7: Run for real

Run `ai-cron` (or wait for the next automatic cycle):

```bash
ai-cron -v
```

Your automation will appear in the output. Check the Settings > Automations panel in the TUI to see run status.

## Monitoring

### Settings Panel

The CCC Settings plugin includes an **Automations** section showing all registered automations with their schedule, last run status (color-coded), relative timestamp, and result message.

### Log File

Automation log output is written to `~/.config/ccc/data/automation.log`. The log file is automatically rotated when it exceeds 5 MB or is older than 7 days (one previous rotation is kept as `.1`).

### Run History

All runs are recorded in the `cc_automation_runs` table in the CCC database:

```sql
SELECT name, started_at, finished_at, status, message
FROM cc_automation_runs
ORDER BY started_at DESC
LIMIT 20;
```

## Common Issues

**"command not found"** -- The `command` field is executed via `sh -c`. Make sure the path is absolute or that the binary/interpreter is on your PATH. Test the command in a terminal first.

**Init timeout** -- The automation has 5 seconds to send `ready` after receiving `init`. If your setup is slow, defer heavy work to `on_run`.

**Run timeout (30s)** -- The total time from subprocess spawn to `result` is 30 seconds. If your automation needs more time, consider breaking it into smaller pieces or running only the fast path in the automation and kicking off longer work asynchronously.

**"not due" / schedule not firing** -- The runner checks the `cc_automation_runs` table. If a `daily` automation already ran today, it will not run again until tomorrow. To force a re-run during development, delete the relevant row from the table:

```sql
DELETE FROM cc_automation_runs WHERE name = 'my-automation';
```

**Automation skipped with no log** -- Disabled automations (`enabled: false`) are silently skipped. An unknown schedule value is also skipped.

**Database is locked** -- Open the database in read-only mode (`?mode=ro` in the URI) to avoid contention with `ai-cron` and other automations. If you need to write, use your own separate database or a namespaced table.

**Buffered stdout** -- The protocol requires JSON lines to be flushed immediately. Python's `ccc_automation.py` handles this with `sys.stdout.flush()`. If writing an automation in another language, make sure stdout is unbuffered or line-buffered.

**stderr output** -- If your automation crashes (non-zero exit before sending `result`), the runner captures up to 500 bytes of stderr and records it as the error message.

## Security Model

- **Scoped config:** Automations only receive config sections listed in `config_scopes`. An automation with `config_scopes: ["slack"]` cannot see GitHub tokens or calendar credentials.
- **Subprocess isolation:** Automations run as child processes with the same user permissions as `ai-cron`. No additional sandboxing.
- **Database access:** The DB path is provided for read access. Do not write to CCC-owned tables. If you need persistent state, use your own namespaced table or external storage.
- **No network restrictions:** Automations can make any network calls your user account permits.

## Writing Automations in Other Languages

The protocol is language-agnostic. Any executable that reads JSON lines from stdin and writes JSON lines to stdout will work. The steps are:

1. Read one JSON line from stdin -- this is the `init` message
2. Parse it, extract `db_path`, `config`, and `settings`
3. Write `{"type":"ready"}` to stdout (flush immediately)
4. Read the next JSON line -- this is the `run` message
5. Do your work, optionally writing `log` messages
6. Write a `result` message with `status` and `message`
7. Read the `shutdown` message and exit

Example in bash (for illustration):

```bash
#!/bin/bash
while IFS= read -r line; do
  type=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['type'])")
  case "$type" in
    init)
      echo '{"type":"ready"}'
      ;;
    run)
      echo '{"type":"result","status":"success","message":"hello from bash"}'
      ;;
    shutdown)
      exit 0
      ;;
  esac
done
```
