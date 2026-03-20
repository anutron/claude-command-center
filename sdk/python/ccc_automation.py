#!/usr/bin/env python3
"""CCC Automation SDK — reference implementation for Python automations.

Single-file, no dependencies beyond stdlib. Implements the JSON-lines protocol
defined in specs/core/automations.md for writing CCC automations.

Usage:
    from ccc_automation import CCCAutomation

    class MyAutomation(CCCAutomation):
        def on_run(self, trigger, context):
            self.log("info", "Doing my thing")
            return "success", "Did the thing"

    if __name__ == "__main__":
        MyAutomation("my-automation").run()
"""

import json
import sys


class CCCAutomation:
    """Base class for CCC automations.

    Subclass this and override on_run() with your automation logic.
    Optionally override on_init() for setup that needs db_path/config/settings.
    """

    def __init__(self, slug):
        self.slug = slug
        self.db_path = None
        self.config = {}
        self.settings = {}

    def on_init(self, db_path, config, settings):
        """Called after the host sends the init message.

        Override for any setup that needs the database path, scoped config,
        or automation settings. The base implementation does nothing.

        Args:
            db_path: Absolute path to the CCC SQLite database (read-only recommended).
            config: Scoped config dict — only sections listed in config_scopes.
            settings: Arbitrary key-value settings from config.yaml.
        """
        pass

    def on_run(self, trigger, context):
        """Called when the host sends the run message. Override with your logic.

        Must return a (status, message) tuple where:
            status: One of "success", "error", "skipped"
            message: Human-readable description of what happened

        Args:
            trigger: The trigger string (currently empty for scheduled runs).
            context: Additional context dict from the host.

        Returns:
            Tuple of (status, message).
        """
        return "skipped", "not implemented"

    def log(self, level, message):
        """Send a log message to the host.

        Can be called any time between ready and result.

        Args:
            level: One of "debug", "info", "warn", "error".
            message: Log line content.
        """
        self._send({"type": "log", "level": level, "message": message})

    def run(self):
        """Main loop — reads JSON-lines from stdin, dispatches to handlers.

        Call this from your __main__ block. It blocks until the host sends
        shutdown or closes stdin.
        """
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue

            try:
                msg = json.loads(line)
            except json.JSONDecodeError as e:
                self._send({
                    "type": "log",
                    "level": "error",
                    "message": f"invalid JSON from host: {e}",
                })
                continue

            msg_type = msg.get("type")

            if msg_type == "init":
                self.db_path = msg.get("db_path")
                self.config = msg.get("config", {})
                self.settings = msg.get("settings", {})
                self.on_init(self.db_path, self.config, self.settings)
                self._send({"type": "ready", "slug": self.slug})

            elif msg_type == "run":
                try:
                    status, message = self.on_run(
                        msg.get("trigger", ""),
                        msg.get("context", {}),
                    )
                    self._send({
                        "type": "result",
                        "status": status,
                        "message": message,
                    })
                except Exception as e:
                    self._send({
                        "type": "result",
                        "status": "error",
                        "message": str(e),
                    })

            elif msg_type == "shutdown":
                break

    def _send(self, msg):
        """Write a JSON-lines message to stdout."""
        sys.stdout.write(json.dumps(msg) + "\n")
        sys.stdout.flush()
