package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// topicWatcher polls ~/.claude/session-topics/ for topic file changes and
// updates session records in the registry. Claude Code writes topic text to
// {session-uuid}.txt files; the watcher maps those UUIDs back to CCC session
// IDs via pid-{PID}.map files.
type topicWatcher struct {
	dir      string
	registry *sessionRegistry
	interval time.Duration
	stop     chan struct{}
	onUpdate func() // called when any topic is updated
	// Track file contents to detect changes.
	lastSeen map[string]string // filename -> content
}

func newTopicWatcher(registry *sessionRegistry, interval time.Duration) *topicWatcher {
	home, _ := os.UserHomeDir()
	return &topicWatcher{
		dir:      filepath.Join(home, ".claude", "session-topics"),
		registry: registry,
		interval: interval,
		stop:     make(chan struct{}),
		lastSeen: make(map[string]string),
	}
}

func (tw *topicWatcher) start() {
	go tw.loop()
}

func (tw *topicWatcher) shutdown() {
	close(tw.stop)
}

func (tw *topicWatcher) loop() {
	// Do an initial scan immediately.
	if updated := tw.scan(); len(updated) > 0 && tw.onUpdate != nil {
		tw.onUpdate()
	}

	ticker := time.NewTicker(tw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-tw.stop:
			return
		case <-ticker.C:
			if updated := tw.scan(); len(updated) > 0 && tw.onUpdate != nil {
				tw.onUpdate()
			}
		}
	}
}

// scan reads the session-topics directory and updates sessions whose topic
// files have changed.
func (tw *topicWatcher) scan() []string {
	entries, err := os.ReadDir(tw.dir)
	if err != nil {
		return nil
	}

	// Build PID → Claude session UUID map from pid-*.map files.
	pidToClaudeID := make(map[int]string)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "pid-") || !strings.HasSuffix(name, ".map") {
			continue
		}
		pidStr := strings.TrimSuffix(strings.TrimPrefix(name, "pid-"), ".map")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		content, err := os.ReadFile(filepath.Join(tw.dir, name))
		if err != nil {
			continue
		}
		claudeID := strings.TrimSpace(string(content))
		if claudeID != "" {
			pidToClaudeID[pid] = claudeID
		}
	}

	// Invert: Claude UUID → PID.
	claudeIDToPID := make(map[string]int, len(pidToClaudeID))
	for pid, cid := range pidToClaudeID {
		claudeIDToPID[cid] = pid
	}

	// Check each .txt topic file for changes.
	var updated []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		claudeID := strings.TrimSuffix(name, ".txt")

		content, err := os.ReadFile(filepath.Join(tw.dir, name))
		if err != nil {
			continue
		}
		topic := strings.TrimSpace(string(content))
		if topic == "" {
			continue
		}

		// Skip if unchanged since last scan.
		if tw.lastSeen[name] == topic {
			continue
		}
		tw.lastSeen[name] = topic

		// Find the PID for this Claude session.
		pid, ok := claudeIDToPID[claudeID]
		if !ok {
			continue
		}

		// Find the CCC session with this PID and update its topic.
		if sessionID := tw.registry.sessionIDForPID(pid); sessionID != "" {
			if err := tw.registry.update(UpdateSessionParams{
				SessionID: sessionID,
				Topic:     topic,
			}); err == nil {
				updated = append(updated, fmt.Sprintf("%s→%s", sessionID[:8], topic))
			}
		}
	}
	return updated
}
