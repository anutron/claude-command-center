# SPEC: Pomodoro Plugin (example, external)

## Purpose

A simple pomodoro timer demonstrating the CCC external plugin protocol. Shows how to build an external plugin in Python using the CCC Plugin SDK.

## Slug: `pomodoro`

## Routes

- `pomodoro` — default view (timer display)

## State

- state: idle | working | short_break | long_break
- remaining: seconds left in current phase
- sessions_completed: count of completed work sessions
- last_tick: timestamp of last timer update

## Key Bindings

| Key | Mode | Description | Promoted |
|-----|------|-------------|----------|
| enter | idle | Start timer | yes |
| enter | running | Pause timer | yes |
| r | any | Reset timer | yes |
| s | running | Skip to next phase | yes |

## Event Bus

- Publishes: `pomodoro.completed` with {sessions: int} when a work session finishes

## Migrations

None.

## Behavior

1. Starts in idle state with 25:00 on the clock
2. Enter starts a 25-minute work session
3. When work session completes, transitions to break (5 min short, 15 min long every 4th)
4. Enter during a session pauses (returns to idle, preserving remaining time)
5. `r` resets to idle with full 25 minutes
6. `s` skips to the next phase
7. Timer updates via refresh interval (1 second) and render calls
8. Renders: state label, countdown, progress bar, session count, controls

## Configuration

Add to `~/.config/ccc/config.yaml`:

```yaml
external_plugins:
  - name: Pomodoro
    command: python3 /path/to/examples/pomodoro/pomodoro.py
    enabled: true
```

## Test Cases

- Init responds with ready message containing correct metadata
- Render in idle state shows "Ready" and 25:00
- Enter key starts timer (state transitions to working)
- Enter key during work pauses (state transitions to idle)
- Reset key returns to idle with full duration
- Skip advances to next phase
- Session completion triggers phase transition and event
- Long break triggers after 4 sessions
