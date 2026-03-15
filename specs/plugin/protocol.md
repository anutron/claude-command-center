# SPEC: External Plugin Protocol

## Purpose

Define the JSON-lines protocol for external plugins running as subprocesses. The host communicates with external plugins via stdin/stdout using newline-delimited JSON.

## Transport

- Each message is a single line of JSON followed by a newline (`\n`).
- Plugins MUST flush stdout after writing each message. Buffered output will cause the host to hang waiting for a response.
- The host sets `PYTHONUNBUFFERED=1` in the environment for Python plugins to disable output buffering by default.
- Stderr from the plugin is captured and logged by the host but not parsed as protocol messages.

## Startup Sequence (Two-Phase Init)

1. Host spawns the plugin subprocess.
2. Host sends an `init` message containing `db_path`, `width`, and `height` — but **no config**. This prevents leaking sensitive configuration before the plugin has identified itself.
3. Plugin MUST respond with a `ready` message declaring its metadata, including an optional `config_scopes` array listing the top-level config section names it needs (e.g., `["github", "slack"]`).
4. Host sends a `config` message containing **only** the config sections the plugin requested. If `config_scopes` is empty or omitted, the config payload is an empty object (secure by default).
5. After the `config` message, the host may send `render`, `key`, `navigate`, `event`, `refresh`, or `shutdown` in any order.

## Protocol

### Host -> Plugin Messages

| Type | Fields | Description |
|------|--------|-------------|
| init | db_path, width, height | Initialize plugin (no config) |
| config | config | Scoped configuration (sent after ready) |
| refresh | (none) | Trigger data refresh |
| render | width, height, frame | Request view output |
| key | key, alt | Key press event |
| navigate | route, args | Navigate to sub-route |
| event | source, topic, payload | Event bus delivery |
| shutdown | (none) | Graceful shutdown |

#### JSON Examples

**init** — sent once at startup (no config included):

```json
{"type":"init","db_path":"/home/user/.local/share/ccc/ccc.db","width":120,"height":40}
```

**config** — sent after receiving `ready`, contains only the sections the plugin requested via `config_scopes`:

```json
{"type":"config","config":{"github":{"token":"..."}}}
```

If the plugin declared no `config_scopes`, this is an empty object:

```json
{"type":"config","config":{}}
```

**render** — request the plugin to produce its current view:

```json
{"type":"render","width":120,"height":40,"frame":42}
```

**key** — a key press forwarded to the plugin:

```json
{"type":"key","key":"enter","alt":false}
```

```json
{"type":"key","key":"q","alt":true}
```

**navigate** — switch to a plugin sub-route:

```json
{"type":"navigate","route":"pomodoro/settings","args":{"focus":"duration"}}
```

**event** — an event from the bus delivered to the plugin:

```json
{"type":"event","source":"todos","topic":"todo.completed","payload":{"id":7,"title":"Review PR"}}
```

**refresh** — ask the plugin to refresh its data (e.g., re-read from DB):

```json
{"type":"refresh"}
```

**shutdown** — the host is closing; plugin should clean up and exit:

```json
{"type":"shutdown"}
```

### Plugin -> Host Messages

| Type | Fields | Description |
|------|--------|-------------|
| ready | slug, tab_name, refresh_interval_ms, routes, key_bindings, migrations, config_scopes | Init response |
| view | content | Rendered ANSI text |
| action | action, action_payload, action_args | Action request |
| event | event_topic, event_payload | Publish event to bus |
| log | level, message | Log entry |

#### JSON Examples

**ready** — response to init, declares plugin metadata and config needs:

```json
{"type":"ready","slug":"pomodoro","tab_name":"Pomodoro","refresh_interval_ms":1000,"routes":["pomodoro"],"key_bindings":[{"key":"enter","description":"Start/pause timer","promoted":true}],"migrations":[],"config_scopes":["pomodoro"]}
```

The `config_scopes` field is an array of top-level config section names (matching YAML keys in `config.yaml`) that the plugin needs. The host will only send those sections in the subsequent `config` message. If omitted or empty, no config is sent.

**view** — rendered content (may include ANSI escape codes):

```json
{"type":"view","content":"  POMODORO TIMER\n\n  > WORKING\n\n  24:37\n"}
```

**action** — request the host to perform an action:

```json
{"type":"action","action":"flash","action_payload":"Timer started!","action_args":{}}
```

Valid actions: `noop`, `flash`, `launch`, `quit`, `navigate`.

**event** — publish an event to the host's event bus:

```json
{"type":"event","event_topic":"pomodoro.completed","event_payload":{"sessions":3}}
```

**Note:** The host auto-prefixes the `event_topic` with `<slug>:` before publishing to the bus. For example, if the plugin's slug is `pomodoro` and it publishes topic `completed`, the bus topic becomes `pomodoro:completed`. This prevents external plugins from impersonating built-in event topics.

**log** — send a log message to the host's logger:

```json
{"type":"log","level":"info","message":"Pomodoro plugin initialized"}
```

Valid levels: `info`, `warn`, `error`.

## Security

### Slug Validation

The host validates the slug declared in the `ready` message:

- **Reserved slugs rejected** — Slugs matching built-in plugin names (`sessions`, `commandcenter`, `settings`) are rejected. The plugin is shut down and not loaded.
- **Uniqueness enforced** — If a slug is already in use by another loaded external plugin, the duplicate is rejected and shut down.

### Config Scoping

Plugins only receive the config sections they explicitly request via `config_scopes`. This is secure by default — a plugin that omits `config_scopes` receives an empty config object. The host matches scope names against the YAML tags of the top-level `Config` struct fields.

### Event Topic Prefixing

All event topics published by external plugins are automatically prefixed with `<slug>:` by the host. External plugins cannot publish events with arbitrary topic names, preventing impersonation of built-in event sources.

### Migration SQL Validation

External plugin migrations are validated to ensure they only contain DDL statements (CREATE TABLE, CREATE INDEX, ALTER TABLE, DROP TABLE, DROP INDEX) that are namespaced to the plugin's slug. All table and index names must begin with `<slug>_`. DML statements (INSERT, UPDATE, DELETE, SELECT) and DDL targeting other namespaces are rejected. See the External Adapter spec for details.

## Edge Cases

### Malformed JSON

If the host receives a line that is not valid JSON, it logs the error and ignores the line. The plugin is not terminated. Plugins should similarly handle malformed input from the host by logging an error and continuing.

### Unknown Message Types

Both host and plugin MUST ignore message types they do not recognize. This allows forward-compatible protocol extensions.

### Plugin Crash or Unexpected Exit

If the plugin process exits with a non-zero code or is killed, the host:
1. Logs the exit code and any stderr output.
2. Displays an error message in the plugin's tab: "Plugin crashed. Press r to restart."
3. On `r`, the host respawns the plugin and sends a fresh `init`.

### Slow Plugins

If a plugin does not respond to a `render` message within 2 seconds, the host displays a "loading..." placeholder. The host does not kill the plugin; it waits indefinitely but shows the stale/loading view.

### Buffering

Plugins MUST flush stdout after every message. In Python, the SDK calls `sys.stdout.flush()` after each write. The host also sets `PYTHONUNBUFFERED=1` for Python plugins. For other languages, ensure line buffering or explicit flushing.

## Error Recovery

- If process exits unexpectedly, tab shows error + "press r to restart"
- All errors logged to CCC log
- Settings module shows plugin health status

## Test Cases

- Two-phase init: init message has no config, ready declares config_scopes, config message delivers scoped config
- Plugin with no config_scopes receives empty config object
- Render message produces view response
- Key message produces action response
- Process crash triggers error state
- Restart after crash works
- Malformed JSON from plugin is logged and ignored
- Unknown message types are ignored without error
- Plugin that never flushes stdout is handled gracefully (loading state shown)
- Plugin with reserved slug is rejected and shut down
- Plugin with duplicate slug is rejected and shut down
- Event topics from external plugins are auto-prefixed with slug
- Migration SQL with non-namespaced tables is rejected
