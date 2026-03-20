# Calendar Accept Automation

Example CCC automation that auto-accepts calendar invites matching configured patterns.

## Setup

Add to your `~/.config/ccc/config.yaml`:

```yaml
automations:
  - name: "calendar-accept"
    command: "python3 /path/to/examples/calendar-accept/calendar_accept.py"
    enabled: true
    schedule: "every_refresh"
    config_scopes:
      - "calendar"
    settings:
      dry_run: true
      accept_patterns:
        - "standup"
        - "1:1"
        - "team sync"
```

## How It Works

1. On each `ccc-refresh` cycle, the automation receives the CCC database path
2. It queries `cc_calendar_events` for future events with `needsAction` or `tentative` status
3. Events matching any `accept_patterns` substring (in title or organizer) are accepted
4. In `dry_run: true` mode (the default), it only logs what it would do

## Making It Real

To actually accept events via the Google Calendar API:

1. Install `google-api-python-client` and `google-auth`
2. Implement `_accept_event()` in `calendar_accept.py` using your OAuth credentials
3. Set `dry_run: false` in settings

## SDK

This example uses the Python automation SDK at `sdk/python/ccc_automation.py`. The `sys.path` hack at the top of the script allows importing it without installation.
