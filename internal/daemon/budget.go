package daemon

import "encoding/json"

// Budget RPC handlers on the Server.

func (s *Server) handleGetBudgetStatus(req *RPCRequest) (interface{}, *RPCError) {
	if s.governed == nil {
		return nil, &RPCError{Code: -32000, Message: "budget governance not configured"}
	}

	status := s.governed.BudgetTracker().Status()

	var agentCount int
	if s.runner != nil {
		agentCount = len(s.runner.Active()) + s.runner.QueueLen()
	}

	return BudgetStatusResult{
		HourlySpent:      status.HourlySpent,
		HourlyLimit:      status.HourlyLimit,
		DailySpent:       status.DailySpent,
		DailyLimit:       status.DailyLimit,
		EmergencyStopped: status.EmergencyStopped,
		WarningLevel:     status.WarningLevel,
		ActiveAgents:     agentCount,
	}, nil
}

func (s *Server) handleStopAllAgents(req *RPCRequest) (interface{}, *RPCError) {
	if s.governed == nil {
		return nil, &RPCError{Code: -32000, Message: "budget governance not configured"}
	}

	// Count active agents before stopping.
	var stopped int
	if s.runner != nil {
		active := s.runner.Active()
		stopped = len(active)
		for _, a := range active {
			s.runner.Kill(a.ID)
		}
	}

	// Activate emergency stop on the budget tracker.
	s.governed.BudgetTracker().EmergencyStop()

	// Broadcast emergency stop event.
	data, _ := json.Marshal(map[string]interface{}{
		"stopped": stopped,
	})
	s.Broadcast(Event{
		Type: "budget.emergency_stop",
		Data: data,
	})

	return StopAllAgentsResult{Stopped: stopped}, nil
}

func (s *Server) handleResumeAgents(req *RPCRequest) (interface{}, *RPCError) {
	if s.governed == nil {
		return nil, &RPCError{Code: -32000, Message: "budget governance not configured"}
	}

	s.governed.BudgetTracker().Resume()

	// Broadcast resumed event.
	data, _ := json.Marshal(map[string]bool{"resumed": true})
	s.Broadcast(Event{
		Type: "budget.resumed",
		Data: data,
	})

	return ResumeAgentsResult{Resumed: true}, nil
}
