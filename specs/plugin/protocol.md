# SPEC: External Plugin Protocol

## Purpose

Define the JSON-lines protocol for external plugins running as subprocesses. The host communicates with external plugins via stdin/stdout using newline-delimited JSON.

## Protocol

### Host -> Plugin Messages

| Type | Fields | Description |
|------|--------|-------------|
| init | config, db_path, width, height | Initialize plugin |
| refresh | (none) | Trigger data refresh |
| render | width, height, frame | Request view output |
| key | key, selected_index | Key press event |
| navigate | route, args | Navigate to sub-route |
| event | source, topic, payload | Event bus delivery |
| shutdown | (none) | Graceful shutdown |

### Plugin -> Host Messages

| Type | Fields | Description |
|------|--------|-------------|
| ready | slug, tab_name, refresh_interval, routes, key_bindings, migrations | Init response |
| view | content | Rendered ANSI text |
| action | action, payload, args | Action request |
| event | topic, payload | Publish event to bus |
| log | level, message | Log entry |

### Error Recovery

- If process exits unexpectedly, tab shows error + "press r to restart"
- All errors logged to CCC log
- Settings module shows plugin health status

## Test Cases

- Init message produces ready response
- Render message produces view response
- Key message produces action response
- Process crash triggers error state
- Restart after crash works
