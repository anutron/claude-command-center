#!/usr/bin/env python3
"""CCC Pomodoro Timer Plugin.

A simple pomodoro timer demonstrating the CCC external plugin protocol.
"""

from ccc_plugin import CCCPlugin
import time


class PomodoroPlugin(CCCPlugin):
    WORK_DURATION = 25 * 60      # 25 minutes
    SHORT_BREAK = 5 * 60         # 5 minutes
    LONG_BREAK = 15 * 60         # 15 minutes
    SESSIONS_BEFORE_LONG = 4

    def __init__(self):
        super().__init__(
            slug="pomodoro",
            tab_name="Pomodoro",
            key_bindings=[
                {"key": "enter", "description": "Start/pause timer", "promoted": True},
                {"key": "r", "description": "Reset timer", "promoted": True},
                {"key": "s", "description": "Skip to next phase", "promoted": True},
            ],
            refresh_interval_ms=1000,  # tick every second
        )
        self.state = "idle"         # idle, working, short_break, long_break
        self.remaining = self.WORK_DURATION
        self.sessions_completed = 0
        self.last_tick = None

    def on_init(self, config, db_path, width, height):
        self.log("info", "Pomodoro plugin initialized")

    def on_render(self, width, height, frame):
        # Update timer based on elapsed time
        self._tick()

        lines = []
        lines.append("")
        lines.append(self._center("POMODORO TIMER", width))
        lines.append("")

        # State indicator
        state_labels = {
            "idle": "Ready",
            "working": "> WORKING",
            "short_break": "SHORT BREAK",
            "long_break": "LONG BREAK",
        }
        state_text = state_labels.get(self.state, self.state)
        lines.append(self._center(state_text, width))
        lines.append("")

        # Timer display
        mins, secs = divmod(self.remaining, 60)
        timer_str = f"  {mins:02d}:{secs:02d}  "

        # Simple ASCII box around timer
        box_width = len(timer_str) + 4
        lines.append(self._center("+" + "-" * (box_width - 2) + "+", width))
        lines.append(self._center("|" + timer_str.center(box_width - 2) + "|", width))
        lines.append(self._center("+" + "-" * (box_width - 2) + "+", width))
        lines.append("")

        # Progress bar
        if self.state != "idle":
            total = self._phase_duration()
            elapsed = total - self.remaining
            bar_width = min(30, width - 10)
            filled = int(bar_width * elapsed / max(total, 1))
            bar = "#" * filled + "." * (bar_width - filled)
            lines.append(self._center(f"[{bar}]", width))
            lines.append("")

        # Session count
        lines.append(self._center(f"Sessions completed: {self.sessions_completed}", width))
        lines.append("")

        # Controls
        if self.state == "idle":
            lines.append(self._center("enter: start  |  r: reset", width))
        else:
            lines.append(self._center("enter: pause  |  s: skip  |  r: reset", width))

        return "\n".join(lines)

    def on_key(self, key, alt):
        if key == "enter":
            if self.state == "idle":
                self.state = "working"
                self.remaining = self.WORK_DURATION
                self.last_tick = time.time()
                return {"action": "flash", "action_payload": "Pomodoro started!", "action_args": {}}
            elif self.state in ("working", "short_break", "long_break"):
                # Pause - go back to idle but keep remaining time
                self.last_tick = None
                old_state = self.state
                self.state = "idle"
                return {"action": "flash", "action_payload": f"Paused ({old_state})", "action_args": {}}

        elif key == "r":
            self.state = "idle"
            self.remaining = self.WORK_DURATION
            self.last_tick = None
            return {"action": "flash", "action_payload": "Timer reset", "action_args": {}}

        elif key == "s" and self.state != "idle":
            self._advance_phase()
            return {"action": "flash", "action_payload": f"Skipped to {self.state}", "action_args": {}}

        return None

    def on_refresh(self):
        self._tick()

    def _tick(self):
        if self.state == "idle" or self.last_tick is None:
            return

        now = time.time()
        elapsed = int(now - self.last_tick)
        if elapsed > 0:
            self.last_tick = now
            self.remaining = max(0, self.remaining - elapsed)

            if self.remaining == 0:
                self._advance_phase()

    def _advance_phase(self):
        if self.state == "working":
            self.sessions_completed += 1
            if self.sessions_completed % self.SESSIONS_BEFORE_LONG == 0:
                self.state = "long_break"
                self.remaining = self.LONG_BREAK
            else:
                self.state = "short_break"
                self.remaining = self.SHORT_BREAK
            self.emit_event("pomodoro.completed", {"sessions": self.sessions_completed})
        else:
            # Break ended, start new work session
            self.state = "working"
            self.remaining = self.WORK_DURATION
        self.last_tick = time.time()

    def _phase_duration(self):
        if self.state == "working":
            return self.WORK_DURATION
        elif self.state == "short_break":
            return self.SHORT_BREAK
        elif self.state == "long_break":
            return self.LONG_BREAK
        return self.WORK_DURATION

    @staticmethod
    def _center(text, width):
        if width <= 0:
            return text
        padding = max(0, (width - len(text)) // 2)
        return " " * padding + text


if __name__ == "__main__":
    PomodoroPlugin().run()
