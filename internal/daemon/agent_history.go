package daemon

import (
	"encoding/json"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	cccdb "github.com/anutron/claude-command-center/internal/db"
)

// ListAgentHistoryParams are the parameters for the ListAgentHistory RPC.
type ListAgentHistoryParams struct {
	WindowHours int `json:"window_hours"`
}

// ListAgentHistoryResult wraps the response for ListAgentHistory.
type ListAgentHistoryResult struct {
	Entries []cccdb.AgentHistoryEntry `json:"entries"`
}

// StreamAgentOutputParams are the parameters for the StreamAgentOutput RPC.
type StreamAgentOutputParams struct {
	AgentID string `json:"agent_id"`
}

// StreamAgentOutputResult is the response for StreamAgentOutput.
type StreamAgentOutputResult struct {
	Events []agent.SessionEvent `json:"events"`
	Done   bool                 `json:"done"`
}

func (s *Server) handleListAgentHistory(req *RPCRequest) (interface{}, *RPCError) {
	var params ListAgentHistoryParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}

	windowHours := params.WindowHours
	if windowHours <= 0 {
		windowHours = 24
	}
	window := time.Duration(windowHours) * time.Hour

	entries, err := cccdb.DBLoadAgentHistory(s.cfg.DB, window)
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: "load agent history: " + err.Error()}
	}

	// Enrich running agents with live status from the runner.
	if s.runner != nil {
		for i := range entries {
			if entries[i].Status == "running" {
				if status := s.runner.Status(entries[i].AgentID); status != nil {
					entries[i].Status = status.Status
				}
			}
		}
	}

	return ListAgentHistoryResult{Entries: entries}, nil
}

func (s *Server) handleStreamAgentOutput(req *RPCRequest) (interface{}, *RPCError) {
	var params StreamAgentOutputParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if params.AgentID == "" {
		return nil, &RPCError{Code: -32602, Message: "agent_id is required"}
	}

	if s.runner == nil {
		// No runner — session is definitely done.
		return StreamAgentOutputResult{Done: true}, nil
	}

	sess := s.runner.Session(params.AgentID)
	if sess == nil {
		// Session not found — already cleaned up.
		return StreamAgentOutputResult{Done: true}, nil
	}

	sess.Mu.Lock()
	events := make([]agent.SessionEvent, len(sess.Events))
	copy(events, sess.Events)
	sess.Mu.Unlock()

	// Check if done via Done() channel (non-blocking).
	done := false
	select {
	case <-sess.Done():
		done = true
	default:
	}

	return StreamAgentOutputResult{Events: events, Done: done}, nil
}
