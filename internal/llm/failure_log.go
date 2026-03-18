package llm

import (
	"encoding/json"
	"os"
)

// FailureEntry represents a single failed LLM call for retry/debugging.
type FailureEntry struct {
	Timestamp string `json:"timestamp"`
	Operation string `json:"operation"`
	Prompt    string `json:"prompt"`
	Error     string `json:"error"`
	TodoID    string `json:"todo_id,omitempty"`
}

// LogFailure appends a failure entry as a JSON-lines record to the given file path.
func LogFailure(logPath string, entry FailureEntry) error {
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}
