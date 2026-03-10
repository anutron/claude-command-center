# SPEC: Plugin Logger

## Purpose

Provides structured logging for plugins. Supports both file-backed logging (production) and memory-only logging (testing). Keeps recent entries in memory for display in the TUI settings/debug views.

## Interface

```go
type Logger interface {
    Info(plugin, msg string, fields ...interface{})
    Warn(plugin, msg string, fields ...interface{})
    Error(plugin, msg string, fields ...interface{})
    Recent(n int) []LogEntry
}

type LogEntry struct {
    Time    time.Time
    Level   string      // "INFO", "WARN", "ERROR"
    Plugin  string
    Message string
    Fields  []interface{}
}
```

## Implementations

### FileLogger

Created via `NewFileLogger(logPath string)`. Writes log lines to a file and keeps recent entries in memory.

- Creates parent directories as needed
- Opens file in append mode
- Thread-safe (protected by `sync.Mutex`)
- Keeps up to 500 entries in memory (oldest entries trimmed when limit exceeded)
- Log line format: `2006-01-02 15:04:05 [LEVEL] plugin: message [fields]`
- `Close()` closes the underlying file

### MemoryLogger

Created via `NewMemoryLogger()`. Same as FileLogger but with no file backing -- entries are only kept in memory. Used for testing.

- Returns a `*FileLogger` with nil file handle
- Same 500-entry memory limit
- `Close()` is a no-op

## Behavior

1. All log methods (`Info`, `Warn`, `Error`) create a `LogEntry` with the current time
2. Entry is appended to the in-memory ring buffer (mutex-protected)
3. If memory entries exceed `maxMem` (500), oldest entries are trimmed
4. If a file is open, the formatted log line is written to it
5. `Recent(n)` returns the last `n` entries (or fewer if less are available), as a copy

## Test Cases

- Info/Warn/Error create entries with correct level
- Recent returns last n entries in order
- Recent with n > total returns all entries
- Memory limit trims oldest entries
- FileLogger writes to file
- MemoryLogger works without file
- Thread-safety under concurrent writes
- Close closes file handle
