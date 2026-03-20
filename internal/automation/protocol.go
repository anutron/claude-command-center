package automation

// HostMsg is a message sent from the host (ccc-refresh) to an automation subprocess.
type HostMsg struct {
	Type     string                 `json:"type"`               // "init", "run", "shutdown"
	DBPath   string                 `json:"db_path,omitempty"`
	Config   map[string]interface{} `json:"config,omitempty"`
	Settings map[string]interface{} `json:"settings,omitempty"`
	Trigger  string                 `json:"trigger,omitempty"`
	Context  map[string]interface{} `json:"context,omitempty"`
}

// ResultMsg is a message sent from an automation subprocess back to the host.
type ResultMsg struct {
	Type    string `json:"type"`              // "ready", "result", "log"
	Slug    string `json:"slug,omitempty"`
	Status  string `json:"status,omitempty"`  // "success", "error", "skipped"
	Message string `json:"message,omitempty"`
	Level   string `json:"level,omitempty"`   // "info", "warn", "error"
}
