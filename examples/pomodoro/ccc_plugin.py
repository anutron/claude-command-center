"""CCC External Plugin SDK for Python.

Provides a base class for building CCC plugins. Subclass CCCPlugin and
override the on_* methods to handle host messages.
"""

import json
import sys
import time


class CCCPlugin:
    """Base class for CCC external plugins."""

    # Declare which top-level config sections this plugin needs.
    # Override in subclasses, e.g. config_scopes = ["github", "slack"]
    config_scopes = []

    def __init__(self, slug, tab_name, routes=None, key_bindings=None,
                 refresh_interval_ms=0):
        self.slug = slug
        self.tab_name = tab_name
        self.routes = routes or []
        self.key_bindings = key_bindings or []
        self.refresh_interval_ms = refresh_interval_ms
        self.config = {}
        self.db_path = ""
        self.width = 80
        self.height = 24

    # --- Override these in subclasses ---

    def on_init(self, db_path, width, height):
        """Called when the host sends init (before config is available)."""
        pass

    def on_config(self, config):
        """Called when the host sends scoped config after the ready handshake."""
        pass

    def on_render(self, width, height, frame):
        """Return a string of rendered content (can include ANSI escape codes)."""
        return ""

    def on_key(self, key, alt):
        """Handle a key press. Return an action dict or None for noop.

        Action dict: {"action": "flash", "action_payload": "Hello!", "action_args": {}}
        """
        return None

    def on_navigate(self, route, args):
        """Handle navigation to a route."""
        pass

    def on_refresh(self):
        """Handle a refresh request."""
        pass

    def on_event(self, source, topic, payload):
        """Handle an event from the bus."""
        pass

    def on_shutdown(self):
        """Called before the plugin process exits."""
        pass

    # --- SDK methods for plugins to call ---

    def emit_event(self, topic, payload):
        """Publish an event to the host's event bus."""
        self._send({"type": "event", "event_topic": topic, "event_payload": payload})

    def log(self, level, message):
        """Send a log message to the host."""
        self._send({"type": "log", "level": level, "message": message})

    # --- Internal ---

    def _send(self, msg):
        sys.stdout.write(json.dumps(msg) + "\n")
        sys.stdout.flush()

    def _send_ready(self):
        self._send({
            "type": "ready",
            "slug": self.slug,
            "tab_name": self.tab_name,
            "refresh_interval_ms": self.refresh_interval_ms,
            "routes": self.routes,
            "key_bindings": self.key_bindings,
            "migrations": [],
            "config_scopes": self.config_scopes,
        })

    def _handle(self, msg):
        msg_type = msg.get("type")

        if msg_type == "init":
            self.db_path = msg.get("db_path", "")
            self.width = msg.get("width", 80)
            self.height = msg.get("height", 24)
            self.on_init(self.db_path, self.width, self.height)
            self._send_ready()

        elif msg_type == "config":
            self.config = msg.get("config", {})
            self.on_config(self.config)

        elif msg_type == "render":
            w = msg.get("width", self.width)
            h = msg.get("height", self.height)
            frame = msg.get("frame", 0)
            content = self.on_render(w, h, frame)
            self._send({"type": "view", "content": content or ""})

        elif msg_type == "key":
            result = self.on_key(msg.get("key", ""), msg.get("alt", False))
            if result:
                self._send({"type": "action", **result})
            else:
                self._send({"type": "action", "action": "noop"})

        elif msg_type == "navigate":
            self.on_navigate(msg.get("route", ""), msg.get("args", {}))

        elif msg_type == "refresh":
            self.on_refresh()

        elif msg_type == "event":
            self.on_event(msg.get("source", ""), msg.get("topic", ""),
                         msg.get("payload", {}))

        elif msg_type == "shutdown":
            self.on_shutdown()
            sys.exit(0)

    def run(self):
        """Main loop. Call this from your plugin's __main__."""
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            try:
                msg = json.loads(line)
                self._handle(msg)
            except json.JSONDecodeError:
                self.log("error", f"Invalid JSON: {line}")
            except Exception as e:
                self.log("error", f"Plugin error: {e}")
