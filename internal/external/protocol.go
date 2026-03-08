package external

import "encoding/json"

// HostMsg is sent from host to plugin on stdin (one JSON line).
type HostMsg struct {
	Type string `json:"type"` // init, render, key, navigate, event, refresh, shutdown

	// init fields
	Config json.RawMessage `json:"config,omitempty"`
	DBPath string          `json:"db_path,omitempty"`
	Width  int             `json:"width,omitempty"`
	Height int             `json:"height,omitempty"`

	// render fields (also uses Width, Height)
	Frame int `json:"frame,omitempty"`

	// key fields
	Key string `json:"key,omitempty"`
	Alt bool   `json:"alt,omitempty"`

	// navigate fields
	Route string            `json:"route,omitempty"`
	Args  map[string]string `json:"args,omitempty"`

	// event fields
	Source  string                 `json:"source,omitempty"`
	Topic   string                 `json:"topic,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// PluginMsg is received from plugin on stdout (one JSON line).
type PluginMsg struct {
	Type string `json:"type"` // ready, view, action, event, log

	// ready fields
	Slug        string          `json:"slug,omitempty"`
	TabName     string          `json:"tab_name,omitempty"`
	RefreshMS   int             `json:"refresh_interval_ms,omitempty"`
	Routes      []RouteMsg      `json:"routes,omitempty"`
	KeyBindings []KeyBindingMsg `json:"key_bindings,omitempty"`
	Migrations  []MigrationMsg  `json:"migrations,omitempty"`

	// view fields
	Content string `json:"content,omitempty"`

	// action fields
	Action   string            `json:"action,omitempty"`
	APayload string            `json:"action_payload,omitempty"`
	AArgs    map[string]string `json:"action_args,omitempty"`

	// event fields
	Topic2   string                 `json:"event_topic,omitempty"`
	EPayload map[string]interface{} `json:"event_payload,omitempty"`

	// log fields
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
}

// RouteMsg is the wire format for a route declaration.
type RouteMsg struct {
	Slug        string   `json:"slug"`
	Description string   `json:"description"`
	ArgKeys     []string `json:"arg_keys,omitempty"`
}

// KeyBindingMsg is the wire format for a key binding declaration.
type KeyBindingMsg struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Mode        string `json:"mode,omitempty"`
	Promoted    bool   `json:"promoted,omitempty"`
}

// MigrationMsg is the wire format for a migration declaration.
type MigrationMsg struct {
	Version int    `json:"version"`
	SQL     string `json:"sql"`
}
