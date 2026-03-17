# CCC Plugin Development Guide

Build external plugins for Claude Command Center (CCC) in any language. Plugins communicate with the host over **JSON-lines on stdin/stdout** and run as subprocesses.

## Architecture Overview

```
┌──────────────────────────────────────┐
│  CCC Host (bubbletea TUI)            │
│                                      │
│  ┌──────────┐  ┌──────────────────┐  │
│  │ Built-in │  │ External Plugin  │  │
│  │ Plugins  │  │ Adapter          │  │
│  │ (Go)     │  │                  │  │
│  └──────────┘  │  stdin ──JSON──▶ │  │   ┌──────────────┐
│                │  stdout ◀─JSON── │──────│ Your Plugin  │
│                │                  │  │   │ (any language)│
│                └──────────────────┘  │   └──────────────┘
└──────────────────────────────────────┘
```

**Key properties:**

- Each plugin is a separate process launched via `sh -c <command>`
- Communication is one-JSON-object-per-line on stdin (host-to-plugin) and stdout (plugin-to-host)
- The host sets `PYTHONUNBUFFERED=1` in the environment automatically
- stderr output is captured and logged as warnings
- Messages are either **synchronous** (request/response with 50ms timeout) or **asynchronous** (events, logs)
- If a plugin crashes, the host shows an error view; the user can press `r` to restart it

## Protocol Reference

### Two-Phase Initialization

Plugins go through a two-phase init to avoid leaking configuration:

1. **Host sends `init`** with the database path and terminal dimensions
2. **Plugin responds with `ready`**, declaring its identity, capabilities, and which config sections it needs
3. **Host sends `config`** with only the requested config sections (empty map if none requested)

This ensures plugins only receive the config they explicitly ask for.

### Message Types: Host to Plugin

#### `init`

Sent once at startup. The plugin must respond with a `ready` message within 5 seconds.

```json
{
  "type": "init",
  "db_path": "/Users/you/.config/ccc/data/ccc.db",
  "width": 120,
  "height": 40
}
```

#### `config`

Sent immediately after the plugin's `ready` response. Contains only the config sections listed in the plugin's `config_scopes`.

```json
{
  "type": "config",
  "config": {
    "github": {
      "token": "ghp_...",
      "repos": ["owner/repo"]
    }
  }
}
```

If the plugin declared no `config_scopes`, `config` will be `{}`.

#### `render`

Sent when the host needs a fresh view. The plugin must respond with a `view` message within 50ms. If the plugin does not respond in time, the host shows the last cached view.

```json
{
  "type": "render",
  "width": 120,
  "height": 35,
  "frame": 42
}
```

`frame` is a monotonically increasing counter useful for animations.

#### `key`

Sent when the user presses a key while the plugin's tab is active. The plugin must respond with an `action` message within 50ms.

```json
{
  "type": "key",
  "key": "enter",
  "alt": false
}
```

Key strings use bubbletea conventions: `"enter"`, `"tab"`, `"up"`, `"down"`, `"left"`, `"right"`, `"backspace"`, `"esc"`, single characters like `"a"`, `"r"`, `"s"`, etc.

#### `navigate`

Sent when the host (or another plugin) navigates to one of this plugin's declared routes.

```json
{
  "type": "navigate",
  "route": "detail",
  "args": {"id": "123"}
}
```

#### `event`

Sent when an event is published on the event bus that this plugin might care about.

```json
{
  "type": "event",
  "source": "sessions",
  "topic": "session.started",
  "payload": {"session_id": "abc"}
}
```

#### `refresh`

Sent periodically based on the plugin's declared `refresh_interval_ms`. No response expected.

```json
{
  "type": "refresh"
}
```

#### `shutdown`

Sent when CCC is quitting. The plugin should clean up and exit. The host will force-kill after 2 seconds.

```json
{
  "type": "shutdown"
}
```

### Message Types: Plugin to Host

#### `ready`

Sent in response to `init`. Declares the plugin's identity and capabilities.

```json
{
  "type": "ready",
  "slug": "pomodoro",
  "tab_name": "Pomodoro",
  "refresh_interval_ms": 1000,
  "routes": [
    {
      "slug": "detail",
      "description": "Show timer detail",
      "arg_keys": ["id"]
    }
  ],
  "key_bindings": [
    {
      "key": "enter",
      "description": "Start/pause timer",
      "mode": "",
      "promoted": true
    },
    {
      "key": "r",
      "description": "Reset timer",
      "promoted": true
    }
  ],
  "migrations": [
    {
      "version": 1,
      "sql": "CREATE TABLE IF NOT EXISTS pomodoro_sessions (id INTEGER PRIMARY KEY, started_at TEXT, duration_secs INTEGER);"
    }
  ],
  "config_scopes": ["github", "slack"]
}
```

**Fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `slug` | Yes | Unique identifier. Must not collide with built-in slugs (`sessions`, `commandcenter`, `settings`) or other loaded plugins. |
| `tab_name` | Yes | Display name shown in the tab bar. |
| `refresh_interval_ms` | No | How often the host sends `refresh` messages. 0 = never. |
| `routes` | No | Sub-routes this plugin handles. Other plugins can navigate to these. |
| `key_bindings` | No | Key bindings to display in help. `promoted: true` shows in the tab footer. |
| `migrations` | No | SQLite migrations to run. See [Database Migrations](#database-migrations). |
| `config_scopes` | No | Top-level config sections to receive. See [Two-Phase Initialization](#two-phase-initialization). |

#### `view`

Sent in response to `render`. Contains the rendered content as a string (ANSI escape codes are supported).

```json
{
  "type": "view",
  "content": "\n  POMODORO TIMER\n\n  > WORKING\n\n  24:59\n"
}
```

#### `action`

Sent in response to `key`. Tells the host what to do.

```json
{
  "type": "action",
  "action": "flash",
  "action_payload": "Timer started!",
  "action_args": {}
}
```

**Action types:**

| Action | Payload | Description |
|--------|---------|-------------|
| `noop` | — | Key was not handled (or no action needed). |
| `consumed` | — | Key was handled; prevents host default behavior (e.g., tab switching). |
| `open_url` | URL string | Open a URL in the default browser. |
| `flash` | Message string | Show a brief flash message in the status bar. |
| `launch` | Command string | Launch an external command. |
| `quit` | — | Quit CCC. |
| `navigate` | Plugin slug | Navigate to another plugin's route. `action_args` carries route args. |

#### `event`

Sent asynchronously to publish events on the host's event bus. Other plugins (built-in or external) can subscribe to these.

```json
{
  "type": "event",
  "event_topic": "pomodoro.completed",
  "event_payload": {"sessions": 4}
}
```

**Note:** The host auto-prefixes event topics with the plugin's slug. If your slug is `pomodoro` and you emit topic `completed`, other plugins see it as `pomodoro:completed`. This prevents external plugins from impersonating built-in event topics.

#### `log`

Sent asynchronously to write to the host's log.

```json
{
  "type": "log",
  "level": "info",
  "message": "Timer started"
}
```

Levels: `"info"`, `"warn"`, `"error"`.

### Message Flow Summary

```
Host                          Plugin
 │                              │
 │──── init ──────────────────▶│
 │                              │
 │◀──── ready ─────────────────│
 │                              │
 │──── config ────────────────▶│
 │                              │
 │──── render ────────────────▶│
 │◀──── view ──────────────────│
 │                              │
 │──── key ───────────────────▶│
 │◀──── action ────────────────│
 │                              │
 │──── refresh ───────────────▶│  (no response expected)
 │                              │
 │◀──── event ─────────────────│  (async, anytime)
 │◀──── log ───────────────────│  (async, anytime)
 │                              │
 │──── navigate ──────────────▶│  (no response expected)
 │──── event ─────────────────▶│  (no response expected)
 │                              │
 │──── shutdown ──────────────▶│
 │                              │  (plugin exits)
```

## Database Migrations

External plugins can declare SQLite migrations that run automatically during init. Migrations are **sandboxed** — every table and index must be prefixed with your plugin's slug.

Allowed SQL patterns (where `{slug}` is your plugin slug):

- `CREATE TABLE IF NOT EXISTS {slug}_...`
- `CREATE INDEX IF NOT EXISTS {slug}_...`
- `CREATE UNIQUE INDEX IF NOT EXISTS {slug}_...`
- `ALTER TABLE {slug}_...`
- `DROP TABLE IF EXISTS {slug}_...`
- `DROP INDEX IF EXISTS {slug}_...`

Any SQL that does not match these patterns is rejected and the plugin will fail to load.

Migrations are versioned. Include all migrations in every `ready` response — the host tracks which versions have already been applied and only runs new ones.

```json
{
  "migrations": [
    {
      "version": 1,
      "sql": "CREATE TABLE IF NOT EXISTS myplugin_items (id INTEGER PRIMARY KEY, name TEXT NOT NULL, created_at TEXT);"
    },
    {
      "version": 2,
      "sql": "ALTER TABLE myplugin_items ADD COLUMN status TEXT DEFAULT 'active';"
    }
  ]
}
```

## Registering Plugins in config.yaml

Add your plugin to the `external_plugins` list in `~/.config/ccc/config.yaml`:

```yaml
external_plugins:
  - name: pomodoro
    command: python3 /path/to/pomodoro.py
    description: Pomodoro timer
    enabled: true

  - name: my-plugin
    command: /usr/local/bin/my-ccc-plugin
    description: My custom plugin
    enabled: true
```

**Fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Display name (used as fallback slug if init fails). |
| `command` | Yes | Shell command to launch the plugin. Executed via `sh -c`. |
| `description` | No | Human-readable description. |
| `enabled` | Yes | Set to `false` to disable without removing the entry. |

The `command` can be any shell command: a Python script, a compiled binary, a Node.js script, etc. The host runs it through `sh -c`, so pipes and environment variables work.

## Tutorial: Build Your First Plugin

This walkthrough builds a "Hello World" plugin in Python using the `ccc_plugin.py` helper module. By the end you will have a working tab in CCC.

### Step 1: Create the project directory

```bash
mkdir ~/ccc-hello && cd ~/ccc-hello
```

### Step 2: Copy the Python helper

Copy `ccc_plugin.py` from the CCC examples directory:

```bash
cp /path/to/claude-command-center/examples/pomodoro/ccc_plugin.py ~/ccc-hello/
```

This ~150-line file provides the `CCCPlugin` base class that handles the JSON-lines protocol for you. You just override callbacks.

### Step 3: Write your plugin

Create `hello.py`:

```python
#!/usr/bin/env python3
"""CCC Hello World Plugin."""

from ccc_plugin import CCCPlugin


class HelloPlugin(CCCPlugin):
    def __init__(self):
        super().__init__(
            slug="hello",
            tab_name="Hello",
            key_bindings=[
                {"key": "enter", "description": "Say hello", "promoted": True},
            ],
            refresh_interval_ms=0,  # no periodic refresh needed
        )
        self.message = "Press enter to say hello!"
        self.count = 0

    def on_init(self, db_path, width, height):
        self.log("info", "Hello plugin initialized")

    def on_render(self, width, height, frame):
        lines = [
            "",
            "  HELLO WORLD",
            "",
            f"  {self.message}",
            "",
            f"  Greetings sent: {self.count}",
        ]
        return "\n".join(lines)

    def on_key(self, key, alt):
        if key == "enter":
            self.count += 1
            self.message = f"Hello #{self.count}!"
            return {
                "action": "flash",
                "action_payload": f"Hello #{self.count}!",
                "action_args": {},
            }
        return None  # unhandled — return None for noop


if __name__ == "__main__":
    HelloPlugin().run()
```

### Step 4: Make it executable

```bash
chmod +x ~/ccc-hello/hello.py
```

### Step 5: Register in config.yaml

Add to `~/.config/ccc/config.yaml`:

```yaml
external_plugins:
  - name: hello
    command: python3 ~/ccc-hello/hello.py
    description: Hello world demo plugin
    enabled: true
```

### Step 6: Run CCC

```bash
ccc
```

Your "Hello" tab should appear. Press Tab to navigate to it, press Enter to send greetings.

## Python Helper Reference (`ccc_plugin.py`)

The helper module provides `CCCPlugin`, a base class that handles the JSON-lines protocol. Your plugin subclasses it and overrides callbacks.

### Constructor

```python
CCCPlugin(
    slug="myplugin",           # unique identifier
    tab_name="My Plugin",      # display name in tab bar
    routes=[],                 # list of route dicts (optional)
    key_bindings=[],           # list of key binding dicts (optional)
    refresh_interval_ms=0,     # refresh interval in ms (0 = disabled)
)
```

### Class attribute

```python
class MyPlugin(CCCPlugin):
    config_scopes = ["github", "slack"]  # top-level config sections to request
```

### Callbacks to Override

| Method | When Called | Return |
|--------|-----------|--------|
| `on_init(db_path, width, height)` | After host sends `init`, before `ready` | None |
| `on_config(config)` | After host sends scoped config | None |
| `on_render(width, height, frame)` | Host needs a view | String (rendered content) |
| `on_key(key, alt)` | User pressed a key | Action dict or `None` |
| `on_navigate(route, args)` | Host navigates to a route | None |
| `on_refresh()` | Periodic refresh tick | None |
| `on_event(source, topic, payload)` | Event from bus | None |
| `on_shutdown()` | CCC is quitting | None |

### SDK Methods

| Method | Description |
|--------|-------------|
| `self.emit_event(topic, payload)` | Publish an event to the host event bus. |
| `self.log(level, message)` | Send a log message (`"info"`, `"warn"`, `"error"`). |

### Action Dict Format

Return from `on_key` to tell the host what to do:

```python
# Flash a message
{"action": "flash", "action_payload": "Done!", "action_args": {}}

# Open a URL
{"action": "open_url", "action_payload": "https://example.com", "action_args": {}}

# Navigate to another plugin
{"action": "navigate", "action_payload": "sessions", "action_args": {"id": "123"}}

# Do nothing (or just return None)
{"action": "noop"}
```

### Instance Variables

After init, these are available on `self`:

- `self.db_path` — path to the shared SQLite database
- `self.config` — dict of scoped config (populated after `on_config`)
- `self.width`, `self.height` — last known terminal dimensions

## Testing and Debugging Tips

### Run your plugin standalone

Since plugins communicate over stdin/stdout, you can test them manually:

```bash
# Start the plugin
python3 my_plugin.py

# Paste this JSON (one line) and press Enter:
{"type":"init","db_path":"/tmp/test.db","width":80,"height":24}

# You should see a ready response. Then paste:
{"type":"config","config":{}}

# Request a render:
{"type":"render","width":80,"height":24,"frame":0}

# Send a key:
{"type":"key","key":"enter","alt":false}

# Shut down:
{"type":"shutdown"}
```

### Check CCC logs

CCC logs plugin stderr and any `log` messages. Check the log output for errors from your plugin.

### Common issues

**Plugin does not appear:** Verify `enabled: true` in config.yaml and that the command path is correct. Run the command manually to check for import errors.

**"command not found on PATH":** The host resolves the command binary before launching. Make sure the binary or interpreter (e.g., `python3`) is on your PATH.

**Timeout on init:** The host waits 5 seconds for the `ready` response. If your plugin does heavy work in `on_init`, move it to a background thread.

**Stale views:** The host gives 50ms for `render` and `key` responses. If your plugin is slow, the host uses the cached view. Keep rendering fast — do heavy work in `on_refresh` or background threads, not `on_render`.

**Migration rejected:** External plugin table names must be prefixed with your slug followed by underscore (e.g., `pomodoro_sessions`). Only DDL statements (`CREATE TABLE`, `ALTER TABLE`, etc.) are allowed.

**Slug collision:** Your slug must not match a built-in plugin (`sessions`, `commandcenter`, `settings`) or another loaded external plugin.

**Buffered stdout:** If using Python, the host sets `PYTHONUNBUFFERED=1`, but if you use another language, make sure stdout is line-buffered or unbuffered so JSON lines are flushed immediately.

### Writing plugins in other languages

The protocol is language-agnostic. Your plugin just needs to:

1. Read JSON lines from stdin
2. Write JSON lines to stdout (flushed immediately)
3. Handle the init/ready/config handshake
4. Respond to `render` and `key` synchronously (within 50ms)
5. Exit on `shutdown`

Example minimal plugin in bash (for illustration only):

```bash
#!/bin/bash
while IFS= read -r line; do
  type=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['type'])")
  case "$type" in
    init)
      echo '{"type":"ready","slug":"bash-demo","tab_name":"Bash","refresh_interval_ms":0,"routes":[],"key_bindings":[],"migrations":[],"config_scopes":[]}'
      ;;
    config) ;;
    render)
      echo '{"type":"view","content":"\n  Hello from bash!\n"}'
      ;;
    key)
      echo '{"type":"action","action":"noop"}'
      ;;
    shutdown)
      exit 0
      ;;
  esac
done
```
