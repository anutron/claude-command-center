# SPEC: Session Viewer (command center sub-view)

## Purpose

Provide a live, scrollable conversation view for monitoring headless Claude agent sessions. Users can watch events in real time, send messages to the agent via stdin pipe, join the session interactively, or resume a completed session as a new headless agent. This replaces the need to join sessions interactively just to check progress.

## Parent Plugin: `commandcenter`

The session viewer is a sub-view of the command center plugin, accessed from the detail view when a todo has an active or completed agent session.

## Entry Points

| Key | Context | Description |
|-----|---------|-------------|
| `w` | Detail view, todo has active agent session | Open live session viewer |
| `c` | Session viewer (not inputting) | Open message textarea to send input to agent |
| `o` | Session viewer | Join session interactively (launches Claude TUI with `--resume`) |
| `r` | Detail view, todo has SessionID, no active session | Resume agent headless with `--resume` flag |

## State

| Field | Type | Description |
|-------|------|-------------|
| `sessionViewerActive` | `bool` | Whether the session viewer sub-view is displayed |
| `sessionViewerTodoID` | `string` | ID of the todo whose session is being viewed |
| `sessionViewerAutoScroll` | `bool` | Whether viewport follows new events at bottom |
| `sessionViewerDone` | `bool` | Whether the session has ended |
| `sessionViewerInputting` | `bool` | Whether the message textarea is active |
| `sessionViewerInput` | `textarea.Model` | Text input for sending messages to the agent |
| `sessionViewerVP` | `viewport.Model` | Scrollable viewport for event content |
| `sessionViewerListening` | `bool` | Whether the event channel listener cmd is active |

## Event Types

Events are parsed from the Claude CLI `--output-format stream-json` stdout into `sessionEvent` structs:

| Type | Icon | Color | Description |
|------|------|-------|-------------|
| `assistant_text` | `◆` | Cyan | Assistant response text (word-wrapped to viewport width) |
| `tool_use` | `▸` | Yellow | Tool invocation with tool name |
| `tool_result` | `◂` | Green/Red | Tool result (green for success, red for error) |
| `error` | `⚠` | Red | Error or blocked state, labeled "BLOCKED:" |
| `user` | `▷` | Purple | User message (sent via `c` or detected from stream) |
| `system` | `●` | Muted | System messages |

### Event Parsing (`parseSessionEvent`)

Raw stream-JSON events are mapped to `sessionEvent` based on the `type` field:

- `"assistant"` — looks in `message.content` (stream-json nests under `message`), falls back to top-level `content`; iterates array for `text` blocks (becomes `assistant_text`) or `tool_use` blocks
- `"tool_result"` — extracts `tool_use_id`, content (string or array), and `is_error` flag
- `"result"` — maps to `assistant_text` (final output from the agent)
- `"error"` — extracts error message from `error.message` or top-level `message`
- `"user"` — `message.content` can be a string (plain text) or array of content blocks; text blocks become `user` events, `tool_result` blocks become `tool_result` events; pure tool-result-only user messages are shown as tool results (not empty "You" lines)
- `"system"` — extracts text from `message`, falls back to `subtype`, then truncated `session_id`; skipped if no displayable content

## Key Bindings

### Session Viewer (normal mode)

| Key | Description | Promoted |
|-----|-------------|----------|
| `j` / `down` | Scroll down one line, disables auto-scroll | yes |
| `k` / `up` | Scroll up one line, disables auto-scroll | yes |
| `G` | Jump to bottom, re-enable auto-scroll | yes |
| `g` | Jump to top, disable auto-scroll | yes |
| `c` | Open message textarea | yes |
| `o` | Join session interactively (launch with `--resume`) | yes |
| `esc` | Exit viewer, return to detail view | yes |

### Session Viewer (input mode)

| Key | Description |
|-----|-------------|
| `enter` | Send message to agent via stdin pipe, exit input mode |
| `esc` | Cancel input, exit input mode |
| _(other)_ | Passed to textarea for editing |

## Hint Bars

- **Normal (session active):** `j/k scroll · G bottom · g top · c message · o join · esc back`
- **Normal (session ended):** `j/k scroll · G bottom · g top · o join · esc back · session ended`
- **Input mode:** `enter send · esc cancel`

## Bidirectional Communication

### Output: `--output-format stream-json`

The agent process emits one JSON object per line on stdout. A background goroutine in `launchAgent` reads these lines, parses them via `parseSessionEvent`, appends to `sess.Events` (mutex-protected), and sends to `sess.EventsCh` (buffered channel, cap 64, non-blocking send). Every raw line is also teed to a log file for forensic replay (see **Session Logging** below).

### Input: `--input-format stream-json`

The initial prompt and follow-up messages are both sent via stdin as NDJSON:

```json
{"type": "user", "message": {"role": "user", "content": "<text>"}}
```

**Initial prompt delivery:** The prompt is NOT passed as a CLI positional argument (`-- prompt`). With `--input-format stream-json`, the CLI expects the initial prompt via stdin. `launchAgent` sends the enhanced prompt as the first stdin message immediately after `cmd.Start()`.

**Follow-up messages:** `sendUserMessage` writes the same NDJSON format to stdin and clears the blocked status (if `sess.Status == "blocked"`, resets to `"active"`). A local `sessionEvent{Type: "user"}` is appended to `sess.Events` for display in the viewer.

### Event Channel Pattern

The session viewer uses the idiomatic bubbletea async pattern via `listenForAgentEvent`:

1. Returns a `tea.Cmd` that blocks on `sess.EventsCh`
2. When an event arrives, returns `agentEventMsg` — the message handler updates the viewport and re-subscribes
3. When the channel closes (process exited), returns `agentEventsDoneMsg` — sets `sessionViewerDone = true`

## Layout

```
┌──────────────────────────────────────┐
│                                      │
│  SESSION VIEWER — <todo title>       │
│  Status: active ● | Session: abc123  | 2m 30s elapsed
│  ────────────────────────────────────│
│                                      │
│  ◆ Assistant  Analyzing the codebase │
│  ▸ Tool: Read                        │
│  ◂ Result (success)                  │
│  ◆ Assistant  I found the issue...   │
│  ▸ Tool: Edit                        │
│  ◂ Result (success)                  │
│                                      │
│  MESSAGE:                            │  ← only when inputting
│  [textarea]                          │
│                                      │
│  j/k scroll · G bottom · c message · o join · esc back
└──────────────────────────────────────┘
```

### Status Bar

Shows three parts joined by `|`:

- **Status indicator**: `active ●` (cyan), `blocked ●` (red), `completed ●` (green), or `inactive` (muted)
- **Session ID**: first 8 characters of the Claude session UUID (if available)
- **Elapsed time**: `Ns elapsed` or `Nm NNs elapsed`

### Viewport Sizing

- Chrome overhead: header(1) + blank(1) + statusLine(1) + divider(1) + blank(1) + viewport + blank(1) + hints(1) + border(2) = 8 lines
- When inputting, add 4 lines for label(1) + textarea(2) + blank(1)
- `viewHeight` = full content height from the host (no additional subtraction — the host already accounts for tabs/header)
- Viewport height = `viewHeight - 8 - inputChrome`, minimum 3
- Content max width from `ui.ContentMaxWidth`, inner width = maxWidth - 4
- Event text wraps at `viewportWidth - 14` chars (14 = icon + label prefix width)

## Auto-Scroll Behavior

- **Default**: auto-scroll enabled (`sessionViewerAutoScroll = true`)
- **Disabled**: when user scrolls up via `j`/`k`/`up`/`down`
- **Re-enabled**: when user presses `G` (jump to bottom)
- On each new event (`agentEventMsg`), `updateSessionViewerContent` rebuilds the viewport content and calls `GotoBottom()` if auto-scroll is enabled

## Resume Agent (`r` from detail view)

When a todo has a `SessionID` but no active agent session, pressing `r`:

1. Builds a `queuedSession` with `ResumeID` set to the todo's `SessionID`
2. Calls `launchOrQueueAgent` which either starts immediately or queues
3. The `launchAgent` function adds `--resume <ResumeID>` to the CLI args
4. Exits detail view and shows flash message ("Agent resumed for: ..." or "Agent queued for: ...")
5. Uses default permission and budget from `cfg.Agent`

## Session Logging

Headless sessions write raw stream-json output to disk for forensic replay when the in-memory session is lost (TUI restart, process crash, etc.).

### Log Location

`~/.config/ccc/data/session-logs/{timestamp}_{todoID}.jsonl`

- Timestamp format: `2006-01-02T15-04-05` (filesystem-safe)
- Directory created on demand via `os.MkdirAll`

### Log Contents

1. **Header line**: `--- session started at {RFC3339} for todo {todoID} ---`
2. **Raw stdout lines**: every line from the Claude CLI stdout, written verbatim (JSON and non-JSON)
3. **Footer line**: `--- session exited with code {N} at {RFC3339} ---`

### Launch Failure Logging

If the agent fails before the goroutine starts (stdout pipe, stdin pipe, or `cmd.Start()` errors), a log file is created with a single `--- LAUNCH ERROR ---` line describing the failure.

### Design Decisions

- **Best-effort**: if the log file cannot be created, the session proceeds without logging (non-fatal)
- **Raw, not parsed**: logs contain the exact stream-json output, not the parsed `sessionEvent` structs, so nothing is lost in translation
- **No automatic cleanup**: logs accumulate until manually deleted (future: age-based rotation)

## Test Cases

- `w` on a todo with active session opens session viewer (`sessionViewerActive = true`)
- `w` on a todo without active session shows "No active session" flash message
- `j`/`k` scrolls viewport and disables auto-scroll
- `G` jumps to bottom and re-enables auto-scroll
- `g` jumps to top and disables auto-scroll
- `c` opens textarea input mode (`sessionViewerInputting = true`)
- `enter` in input mode sends message via `sendUserMessage` and exits input mode
- `enter` in input mode with empty text cancels without sending
- `esc` in input mode cancels and exits input mode
- `esc` in normal mode exits session viewer (`sessionViewerActive = false`)
- `o` launches interactive session with `resume_id` if todo has SessionID
- `o` is no-op if todo has no SessionID
- `sendUserMessage` writes NDJSON to stdin pipe and clears blocked status
- `sendUserMessage` appends a user event to `sess.Events` for display
- `parseSessionEvent` correctly maps assistant, tool_use, tool_result, result, error, user, system types
- System events with `session_id` (no `message`) display truncated session ID
- System events with no displayable content are skipped (return nil)
- `renderEventLine` renders correct icons and colors for each event type
- Auto-scroll follows bottom on new events when enabled
- Auto-scroll does not follow when disabled (user scrolled up)
- Viewport resizes correctly when dimensions change (re-sets content)
- `agentEventsDoneMsg` sets `sessionViewerDone = true` and shows "session ended" in content
- Empty event list shows "Waiting for events..." or "Session has ended" depending on done state
- `listenForAgentEvent` re-subscribes after each event (returns new cmd)
- `r` from detail view on todo with SessionID and no active session launches headless agent with `--resume`
- `r` from detail view auto-accepts the todo via `launchOrQueueAgent`
- `r` respects concurrency limits (queues if at max)
- Status bar shows correct indicator color for active/blocked/completed/inactive states
- Status bar shows truncated session ID (first 8 chars)
- Status bar shows elapsed time in seconds or minutes+seconds format
