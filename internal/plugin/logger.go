package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger provides structured logging for plugins.
type Logger interface {
	Info(plugin, msg string, fields ...interface{})
	Warn(plugin, msg string, fields ...interface{})
	Error(plugin, msg string, fields ...interface{})
	Recent(n int) []LogEntry
}

// LogEntry is a single log record.
type LogEntry struct {
	Time    time.Time
	Level   string
	Plugin  string
	Message string
	Fields  []interface{}
}

// FileLogger writes logs to a file and keeps recent entries in memory.
type FileLogger struct {
	mu      sync.Mutex
	file    *os.File
	entries []LogEntry
	maxMem  int // max entries kept in memory
}

// NewFileLogger creates a logger that writes to the given path.
func NewFileLogger(logPath string) (*FileLogger, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileLogger{
		file:   f,
		maxMem: 500,
	}, nil
}

// NewMemoryLogger creates a logger that only keeps entries in memory (for testing).
func NewMemoryLogger() *FileLogger {
	return &FileLogger{
		maxMem: 500,
	}
}

func (l *FileLogger) log(level, plugin, msg string, fields ...interface{}) {
	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Plugin:  plugin,
		Message: msg,
		Fields:  fields,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxMem {
		l.entries = l.entries[len(l.entries)-l.maxMem:]
	}

	line := fmt.Sprintf("%s [%s] %s: %s",
		entry.Time.Format("2006-01-02 15:04:05"),
		level,
		plugin,
		msg,
	)
	if len(fields) > 0 {
		line += fmt.Sprintf(" %v", fields)
	}
	line += "\n"

	if l.file != nil {
		_, _ = l.file.WriteString(line)
	}
}

func (l *FileLogger) Info(plugin, msg string, fields ...interface{}) {
	l.log("INFO", plugin, msg, fields...)
}

func (l *FileLogger) Warn(plugin, msg string, fields ...interface{}) {
	l.log("WARN", plugin, msg, fields...)
}

func (l *FileLogger) Error(plugin, msg string, fields ...interface{}) {
	l.log("ERROR", plugin, msg, fields...)
}

// Recent returns the last n log entries.
func (l *FileLogger) Recent(n int) []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	if n > len(l.entries) {
		n = len(l.entries)
	}
	result := make([]LogEntry, n)
	copy(result, l.entries[len(l.entries)-n:])
	return result
}

// Close closes the underlying log file.
func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
