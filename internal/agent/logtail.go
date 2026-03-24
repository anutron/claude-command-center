package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NativeLogPath computes the path to a Claude Code native log file.
// Claude stores logs at ~/.claude/projects/<encoded-path>/<session-id>.jsonl
// where the project directory path has "/" replaced with "-".
func NativeLogPath(projectDir, sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	// Encode the project path: strip leading slash, replace "/" with "-"
	encoded := strings.TrimPrefix(projectDir, "/")
	encoded = strings.ReplaceAll(encoded, "/", "-")
	return filepath.Join(home, ".claude", "projects", encoded, sessionID+".jsonl")
}

// tailNativeLog polls a native log file for new JSONL lines and sends parsed
// events to the provided channel. It handles the file not yet existing by
// polling every 200ms for up to 30s. The function blocks until ctx is cancelled
// or the timeout waiting for the file is exceeded.
func tailNativeLog(ctx context.Context, logPath string, startOffset int64, eventCh chan<- map[string]interface{}) {
	// Wait for file to appear.
	var f *os.File
	deadline := time.Now().Add(30 * time.Second)
	for {
		var err error
		f, err = os.Open(logPath)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
	defer f.Close()

	// Seek to start offset if provided.
	if startOffset > 0 {
		if _, err := f.Seek(startOffset, 0); err != nil {
			return
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			select {
			case eventCh <- event:
			case <-ctx.Done():
				return
			}
		}
		// No more lines — poll for new data.
		select {
		case <-ctx.Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
		// Re-scan from current position: reset scanner on existing reader.
		scanner = bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	}
}

// extractUsageFromEvent extracts input and output token counts from a native
// log assistant event that has a stop_reason (i.e. a completed message).
// Returns (inputTokens, outputTokens, true) on success or (0, 0, false) if
// the event does not contain usage data.
func extractUsageFromEvent(raw map[string]interface{}) (inputTokens, outputTokens int, ok bool) {
	// Look for message.usage on assistant events with a stop_reason.
	msg, hasMsg := raw["message"].(map[string]interface{})
	if !hasMsg {
		return 0, 0, false
	}

	// Must have a non-nil stop_reason to indicate a completed message.
	stopReason, hasStop := msg["stop_reason"]
	if !hasStop || stopReason == nil {
		return 0, 0, false
	}

	usage, hasUsage := msg["usage"].(map[string]interface{})
	if !hasUsage {
		return 0, 0, false
	}

	inputF, okIn := usage["input_tokens"].(float64)
	outputF, okOut := usage["output_tokens"].(float64)
	if !okIn || !okOut {
		return 0, 0, false
	}

	return int(inputF), int(outputF), true
}

// estimateCost computes the estimated USD cost for a given event based on the
// model name and token counts. Pricing (per million tokens):
//   - Opus:   $15 input / $75 output
//   - Sonnet: $3 input / $15 output
func estimateCost(raw map[string]interface{}, input, output int) float64 {
	model := ""
	if msg, ok := raw["message"].(map[string]interface{}); ok {
		model, _ = msg["model"].(string)
	}

	var inputRate, outputRate float64
	switch {
	case strings.Contains(model, "opus"):
		inputRate = 15.0
		outputRate = 75.0
	case strings.Contains(model, "sonnet"):
		inputRate = 3.0
		outputRate = 15.0
	default:
		// Default to sonnet pricing as a reasonable fallback.
		inputRate = 3.0
		outputRate = 15.0
	}

	cost := (float64(input) * inputRate / 1_000_000) + (float64(output) * outputRate / 1_000_000)
	// Round to 6 decimal places to avoid floating point noise.
	return float64(int(cost*1_000_000+0.5)) / 1_000_000

}

// FormatNativeLogPath is an alias for NativeLogPath for external callers that
// want to inspect the path without importing internal details.
var FormatNativeLogPath = NativeLogPath

// init registers a compile-time check that the functions exist.
func init() {
	_ = NativeLogPath
	_ = fmt.Sprint // ensure fmt is used
}
