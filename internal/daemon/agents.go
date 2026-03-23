package daemon

import (
	"encoding/json"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
)

// Agent RPC parameter and result types.

// LaunchAgentParams are the RPC parameters for launching a new agent.
type LaunchAgentParams struct {
	ID         string  `json:"id"`
	Prompt     string  `json:"prompt"`
	Dir        string  `json:"dir,omitempty"`
	Worktree   bool    `json:"worktree,omitempty"`
	Permission string  `json:"permission,omitempty"`
	Budget     float64 `json:"budget,omitempty"`
	ResumeID   string  `json:"resume_id,omitempty"`
}

// StopAgentParams are the RPC parameters for stopping an agent.
type StopAgentParams struct {
	ID string `json:"id"`
}

// AgentStatusParams are the RPC parameters for querying agent status.
type AgentStatusParams struct {
	ID string `json:"id"`
}

// SendAgentInputParams are the RPC parameters for sending input to an agent.
type SendAgentInputParams struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// AgentStatusResult is the RPC result for agent status queries.
type AgentStatusResult struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	Question  string `json:"question,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
}

// Agent dispatch handlers on the Server.

func (s *Server) handleLaunchAgent(req *RPCRequest) (interface{}, *RPCError) {
	if s.runner == nil {
		return nil, &RPCError{Code: -32000, Message: "agent runner not configured"}
	}
	var params LaunchAgentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if params.ID == "" {
		return nil, &RPCError{Code: -32602, Message: "id is required"}
	}

	agentReq := agent.Request{
		ID:         params.ID,
		Prompt:     params.Prompt,
		ProjectDir: params.Dir,
		Worktree:   params.Worktree,
		Permission: params.Permission,
		Budget:     params.Budget,
		ResumeID:   params.ResumeID,
	}

	queued, cmd := s.runner.LaunchOrQueue(agentReq)

	// If not queued, execute the tea.Cmd in a goroutine to actually start the process.
	// The tea.Cmd returns a tea.Msg but we're not in a bubbletea loop, so we
	// fire-and-forget and broadcast status events.
	if !queued && cmd != nil {
		go func() {
			msg := cmd()
			if started, ok := msg.(agent.SessionStartedMsg); ok {
				// Broadcast the start event
				data, _ := json.Marshal(AgentStatusResult{
					ID:        started.ID,
					Status:    "processing",
					StartedAt: time.Now().Format(time.RFC3339),
				})
				s.Broadcast(Event{
					Type: "agent.started",
					Data: data,
				})
			}
		}()
	}

	result := map[string]interface{}{
		"ok":     true,
		"queued": queued,
	}
	return result, nil
}

func (s *Server) handleStopAgent(req *RPCRequest) (interface{}, *RPCError) {
	if s.runner == nil {
		return nil, &RPCError{Code: -32000, Message: "agent runner not configured"}
	}
	var params StopAgentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if params.ID == "" {
		return nil, &RPCError{Code: -32602, Message: "id is required"}
	}

	found := s.runner.Kill(params.ID)
	if !found {
		return nil, &RPCError{Code: -32000, Message: "agent not found: " + params.ID}
	}

	data, _ := json.Marshal(map[string]string{"id": params.ID})
	s.Broadcast(Event{
		Type: "agent.stopped",
		Data: data,
	})

	return map[string]bool{"ok": true}, nil
}

func (s *Server) handleAgentStatus(req *RPCRequest) (interface{}, *RPCError) {
	if s.runner == nil {
		return nil, &RPCError{Code: -32000, Message: "agent runner not configured"}
	}
	var params AgentStatusParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if params.ID == "" {
		return nil, &RPCError{Code: -32602, Message: "id is required"}
	}

	status := s.runner.Status(params.ID)
	if status == nil {
		return nil, &RPCError{Code: -32000, Message: "agent not found: " + params.ID}
	}

	result := AgentStatusResult{
		ID:        status.ID,
		Status:    status.Status,
		SessionID: status.SessionID,
		Question:  status.Question,
	}
	if !status.StartedAt.IsZero() {
		result.StartedAt = status.StartedAt.Format(time.RFC3339)
	}
	return result, nil
}

func (s *Server) handleListAgents(req *RPCRequest) (interface{}, *RPCError) {
	if s.runner == nil {
		return nil, &RPCError{Code: -32000, Message: "agent runner not configured"}
	}

	active := s.runner.Active()
	results := make([]AgentStatusResult, len(active))
	for i, info := range active {
		results[i] = AgentStatusResult{
			ID:        info.ID,
			Status:    info.Status,
			SessionID: info.SessionID,
		}
		if !info.StartedAt.IsZero() {
			results[i].StartedAt = info.StartedAt.Format(time.RFC3339)
		}
	}
	return results, nil
}

func (s *Server) handleSendAgentInput(req *RPCRequest) (interface{}, *RPCError) {
	if s.runner == nil {
		return nil, &RPCError{Code: -32000, Message: "agent runner not configured"}
	}
	var params SendAgentInputParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if params.ID == "" {
		return nil, &RPCError{Code: -32602, Message: "id is required"}
	}

	if err := s.runner.SendMessage(params.ID, params.Message); err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	return map[string]bool{"ok": true}, nil
}
