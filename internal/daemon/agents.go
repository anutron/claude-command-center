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
	Automation string  `json:"automation,omitempty"`
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
	if s.paused.Load() {
		return nil, &RPCError{Code: -32000, Message: "daemon is paused — resume before launching agents"}
	}
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
		Automation: params.Automation,
	}

	queued, cmd := s.runner.LaunchOrQueue(agentReq)

	// If queued=true and cmd is non-nil, the GovernedRunner denied the launch
	// (budget or rate limit). Execute the cmd to get the denial reason.
	if queued && cmd != nil {
		msg := cmd()
		if denied, ok := msg.(agent.LaunchDeniedMsg); ok {
			return nil, &RPCError{Code: -32000, Message: denied.Reason}
		}
	}

	// If not queued, execute the tea.Cmd in a goroutine to actually start the process.
	if !queued && cmd != nil {
		go func() {
			msg := cmd()
			if started, ok := msg.(agent.SessionStartedMsg); ok {
				// Broadcast the start event.
				data, _ := json.Marshal(AgentStatusResult{
					ID:        started.ID,
					Status:    "processing",
					StartedAt: time.Now().Format(time.RFC3339),
				})
				s.Broadcast(Event{
					Type: "agent.started",
					Data: data,
				})

				// Watch for session ID capture and completion.
				go s.watchAgentSessionID(started.ID, started.Session)
				go s.watchAgentDone(started.ID, started.Session)
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

// watchAgentSessionID polls a session's SessionID field and broadcasts an
// agent.session_id event once the UUID is captured. This runs as a goroutine
// alongside watchAgentDone, since the session ID is parsed from stream-JSON
// output asynchronously after the process starts.
func (s *Server) watchAgentSessionID(id string, sess *agent.Session) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sess.Done():
			// Final check — session may have captured the ID just before exiting.
			sess.Mu.Lock()
			sid := sess.SessionID
			sess.Mu.Unlock()
			if sid != "" {
				data, _ := json.Marshal(map[string]interface{}{
					"id":         id,
					"session_id": sid,
				})
				s.Broadcast(Event{Type: "agent.session_id", Data: data})
			}
			return
		case <-ticker.C:
			sess.Mu.Lock()
			sid := sess.SessionID
			sess.Mu.Unlock()
			if sid != "" {
				data, _ := json.Marshal(map[string]interface{}{
					"id":         id,
					"session_id": sid,
				})
				s.Broadcast(Event{Type: "agent.session_id", Data: data})
				return
			}
		}
	}
}

// watchAgentDone waits for a session to finish and performs cleanup:
// calls CleanupFinished to remove it from activeSessions and finalize cost rows,
// then broadcasts an agent.finished event.
func (s *Server) watchAgentDone(id string, sess *agent.Session) {
	<-sess.Done()

	s.runner.CleanupFinished(id)

	exitCode := sess.ExitCode()
	data, _ := json.Marshal(map[string]interface{}{
		"id":        id,
		"exit_code": exitCode,
	})
	s.Broadcast(Event{
		Type: "agent.finished",
		Data: data,
	})

	// Drain queue: start the next queued agent now that capacity is free.
	s.drainNextAgent()
}

// drainNextAgent pops the next queued request (if any) and launches it.
// If the GovernedRunner denies the launch (budget/rate limit), tries the next
// queued item up to 3 times to avoid stalling the queue.
func (s *Server) drainNextAgent() {
	if s.runner == nil {
		return
	}
	for attempts := 0; attempts < 3; attempts++ {
		next, ok := s.runner.DrainQueue()
		if !ok {
			return
		}
		queued, cmd := s.runner.LaunchOrQueue(next)
		if queued && cmd != nil {
			// GovernedRunner denied the launch (budget/rate limit).
			// Execute cmd to consume the denial, then try the next queued item.
			cmd()
			continue
		}
		if !queued && cmd != nil {
			go func() {
				msg := cmd()
				if started, ok := msg.(agent.SessionStartedMsg); ok {
					data, _ := json.Marshal(AgentStatusResult{
						ID:        started.ID,
						Status:    "processing",
						StartedAt: time.Now().Format(time.RFC3339),
					})
					s.Broadcast(Event{
						Type: "agent.started",
						Data: data,
					})
					go s.watchAgentSessionID(started.ID, started.Session)
					go s.watchAgentDone(started.ID, started.Session)
				}
			}()
		}
		return
	}
}
