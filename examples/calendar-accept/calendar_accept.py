#!/usr/bin/env python3
"""Calendar Accept — example CCC automation using the Python SDK.

Demonstrates the automation pattern by reading upcoming calendar events from
the CCC database and identifying ones that need a response. The actual
Google Calendar API call to accept events is stubbed out — this example
focuses on showing how to:

  1. Subclass CCCAutomation
  2. Read settings from config.yaml
  3. Query the CCC SQLite database
  4. Return structured results

To use this for real, you'd add the google-api-python-client dependency
and implement the accept_event() function with actual API calls.
"""

import os
import sqlite3
import sys

# Allow importing the SDK from the sdk/python directory when running from the
# examples directory without installing the package.
SDK_PATH = os.path.join(os.path.dirname(__file__), "..", "..", "sdk", "python")
sys.path.insert(0, SDK_PATH)

from ccc_automation import CCCAutomation


class CalendarAcceptAutomation(CCCAutomation):
    """Auto-accept calendar invites matching configured patterns."""

    def on_init(self, db_path, config, settings):
        self.accept_patterns = self.settings.get("accept_patterns", [])
        self.dry_run = self.settings.get("dry_run", True)
        self.log("info", f"Initialized with {len(self.accept_patterns)} accept patterns")
        if self.dry_run:
            self.log("info", "Running in dry-run mode (no events will be accepted)")

    def on_run(self, trigger, context):
        if not self.accept_patterns:
            return "skipped", "No accept_patterns configured"

        if not self.db_path:
            return "error", "No database path provided"

        # Query upcoming events from the CCC database
        pending = self._find_pending_events()

        if not pending:
            return "success", "No pending invites found"

        matched = []
        for event in pending:
            if self._matches_patterns(event):
                matched.append(event)

        if not matched:
            return "success", f"Checked {len(pending)} pending invites, none matched patterns"

        # Accept matched events (or log what we would do in dry-run mode)
        accepted = []
        for event in matched:
            if self.dry_run:
                self.log("info", f"[DRY RUN] Would accept: {event['title']}")
                accepted.append(event["title"])
            else:
                try:
                    self._accept_event(event)
                    accepted.append(event["title"])
                    self.log("info", f"Accepted: {event['title']}")
                except Exception as e:
                    self.log("error", f"Failed to accept '{event['title']}': {e}")

        prefix = "[DRY RUN] " if self.dry_run else ""
        return "success", f"{prefix}Accepted {len(accepted)} of {len(pending)} pending invites"

    def _find_pending_events(self):
        """Query the CCC database for calendar events needing a response.

        The cc_calendar_events table is populated by ccc-refresh. We look for
        events in the future where response_status indicates we haven't accepted.
        """
        events = []
        try:
            conn = sqlite3.connect(f"file:{self.db_path}?mode=ro", uri=True)
            conn.row_factory = sqlite3.Row
            cursor = conn.cursor()

            # Look for events where we haven't responded yet.
            # The actual column names depend on the CCC schema — adjust as needed.
            cursor.execute("""
                SELECT id, title, start_time, end_time, response_status, organizer
                FROM cc_calendar_events
                WHERE start_time > datetime('now')
                  AND (response_status = 'needsAction' OR response_status = 'tentative')
                ORDER BY start_time
                LIMIT 50
            """)

            for row in cursor.fetchall():
                events.append({
                    "id": row["id"],
                    "title": row["title"],
                    "start_time": row["start_time"],
                    "end_time": row["end_time"],
                    "response_status": row["response_status"],
                    "organizer": row["organizer"],
                })

            conn.close()
        except sqlite3.Error as e:
            self.log("error", f"Database query failed: {e}")

        return events

    def _matches_patterns(self, event):
        """Check if an event matches any of the configured accept patterns.

        Patterns are simple substring matches against the event title or
        organizer. A more sophisticated version could use regex or structured
        rules.
        """
        title = (event.get("title") or "").lower()
        organizer = (event.get("organizer") or "").lower()

        for pattern in self.accept_patterns:
            p = pattern.lower()
            if p in title or p in organizer:
                return True

        return False

    def _accept_event(self, event):
        """Accept a calendar event via the Google Calendar API.

        This is where you'd make the actual API call. Requires:
          - google-api-python-client
          - Valid OAuth credentials (from config scopes)

        Example (not implemented):
            from googleapiclient.discovery import build
            from google.oauth2.credentials import Credentials

            creds = Credentials.from_authorized_user_info(self.config.get("calendar", {}))
            service = build("calendar", "v3", credentials=creds)
            service.events().patch(
                calendarId="primary",
                eventId=event["id"],
                body={"attendees": [{"email": my_email, "responseStatus": "accepted"}]},
            ).execute()
        """
        raise NotImplementedError(
            "Actual Google Calendar API integration not implemented. "
            "Set dry_run: true in settings or implement _accept_event()."
        )


if __name__ == "__main__":
    CalendarAcceptAutomation("calendar-accept").run()
