package daemon

import "encoding/json"

// JSON-RPC wire types — newline-delimited JSON over Unix socket.
type RPCRequest struct {
	Method string          `json:"method"`
	ID     int             `json:"id"`
	Params json.RawMessage `json:"params,omitempty"`
}

type RPCResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Event pushed to subscribers.
type Event struct {
	Type string          `json:"type"` // data.refreshed, session.registered, session.updated, session.ended
	Data json.RawMessage `json:"data,omitempty"`
}

// RPC params/results for session methods.
type RegisterSessionParams struct {
	SessionID    string `json:"session_id"`
	PID          int    `json:"pid"`
	Project      string `json:"project"`
	WorktreePath string `json:"worktree_path,omitempty"`
}

type UpdateSessionParams struct {
	SessionID string `json:"session_id"`
	Topic     string `json:"topic,omitempty"`
}

type SessionInfo struct {
	SessionID    string `json:"session_id"`
	Topic        string `json:"topic"`
	PID          int    `json:"pid"`
	Project      string `json:"project"`
	Repo         string `json:"repo"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktree_path"`
	State        string `json:"state"`
	RegisteredAt string `json:"registered_at"`
	EndedAt      string `json:"ended_at"`
}
