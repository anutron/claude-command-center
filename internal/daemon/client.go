package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	cccdb "github.com/anutron/claude-command-center/internal/db"
)

// Client connects to the daemon over a Unix socket.
type Client struct {
	conn    net.Conn
	reader  *bufio.Reader
	mu      sync.Mutex
	nextID  atomic.Int64
}

// NewClient dials the daemon Unix socket at the given path.
func NewClient(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("daemon dial: %w", err)
	}
	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	return c.conn.Close()
}

// call sends an RPC request and waits for the response.
func (c *Client) call(method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := int(c.nextID.Add(1))

	req := RPCRequest{
		Method: method,
		ID:     id,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		req.Params = raw
	}

	if err := WriteMessage(c.conn, req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Read response line.
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp RPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// Ping checks that the daemon is alive.
func (c *Client) Ping() error {
	_, err := c.call("Ping", nil)
	return err
}

// Refresh triggers an on-demand data refresh on the daemon.
func (c *Client) Refresh() error {
	_, err := c.call("Refresh", nil)
	return err
}

// RegisterSession registers a new session with the daemon.
func (c *Client) RegisterSession(params RegisterSessionParams) error {
	_, err := c.call("RegisterSession", params)
	return err
}

// UpdateSession updates mutable fields on a session.
func (c *Client) UpdateSession(params UpdateSessionParams) error {
	_, err := c.call("UpdateSession", params)
	return err
}

// EndSession marks an active session as ended.
func (c *Client) EndSession(params EndSessionParams) error {
	_, err := c.call("EndSession", params)
	return err
}

// ArchiveSession marks an ended session as archived, removing it from the list.
func (c *Client) ArchiveSession(params ArchiveSessionParams) error {
	_, err := c.call("ArchiveSession", params)
	return err
}

// ListSessions returns all non-archived sessions from the daemon.
func (c *Client) ListSessions() ([]SessionInfo, error) {
	result, err := c.call("ListSessions", nil)
	if err != nil {
		return nil, err
	}
	var sessions []SessionInfo
	if err := json.Unmarshal(result, &sessions); err != nil {
		return nil, fmt.Errorf("unmarshal sessions: %w", err)
	}
	return sessions, nil
}

// LaunchAgent tells the daemon to launch or queue an agent session.
func (c *Client) LaunchAgent(params LaunchAgentParams) error {
	_, err := c.call("LaunchAgent", params)
	return err
}

// StopAgent tells the daemon to kill a running agent session.
func (c *Client) StopAgent(id string) error {
	_, err := c.call("StopAgent", StopAgentParams{ID: id})
	return err
}

// AgentStatus queries the status of a specific agent session.
func (c *Client) AgentStatus(id string) (AgentStatusResult, error) {
	result, err := c.call("AgentStatus", AgentStatusParams{ID: id})
	if err != nil {
		return AgentStatusResult{}, err
	}
	var status AgentStatusResult
	if err := json.Unmarshal(result, &status); err != nil {
		return AgentStatusResult{}, fmt.Errorf("unmarshal agent status: %w", err)
	}
	return status, nil
}

// ListAgents returns all currently active agent sessions.
func (c *Client) ListAgents() ([]AgentStatusResult, error) {
	result, err := c.call("ListAgents", nil)
	if err != nil {
		return nil, err
	}
	var agents []AgentStatusResult
	if err := json.Unmarshal(result, &agents); err != nil {
		return nil, fmt.Errorf("unmarshal agents: %w", err)
	}
	return agents, nil
}

// SendAgentInput sends a message to a running agent's stdin.
func (c *Client) SendAgentInput(id string, message string) error {
	_, err := c.call("SendAgentInput", SendAgentInputParams{ID: id, Message: message})
	return err
}

// GetBudgetStatus returns the current budget status from the daemon.
func (c *Client) GetBudgetStatus() (BudgetStatusResult, error) {
	result, err := c.call("GetBudgetStatus", nil)
	if err != nil {
		return BudgetStatusResult{}, err
	}
	var status BudgetStatusResult
	if err := json.Unmarshal(result, &status); err != nil {
		return BudgetStatusResult{}, fmt.Errorf("unmarshal budget status: %w", err)
	}
	return status, nil
}

// StopAllAgents triggers an emergency stop: kills all active agents and blocks new launches.
func (c *Client) StopAllAgents() (StopAllAgentsResult, error) {
	result, err := c.call("StopAllAgents", nil)
	if err != nil {
		return StopAllAgentsResult{}, err
	}
	var res StopAllAgentsResult
	if err := json.Unmarshal(result, &res); err != nil {
		return StopAllAgentsResult{}, fmt.Errorf("unmarshal stop result: %w", err)
	}
	return res, nil
}

// ResumeAgents clears the emergency stop, allowing new agent launches.
func (c *Client) ResumeAgents() (ResumeAgentsResult, error) {
	result, err := c.call("ResumeAgents", nil)
	if err != nil {
		return ResumeAgentsResult{}, err
	}
	var res ResumeAgentsResult
	if err := json.Unmarshal(result, &res); err != nil {
		return ResumeAgentsResult{}, fmt.Errorf("unmarshal resume result: %w", err)
	}
	return res, nil
}

// PauseDaemon pauses the daemon — stops refresh and blocks new agent launches.
func (c *Client) PauseDaemon() error {
	_, err := c.call("PauseDaemon", nil)
	return err
}

// ResumeDaemon resumes the daemon from paused state.
func (c *Client) ResumeDaemon() error {
	_, err := c.call("ResumeDaemon", nil)
	return err
}

// ShutdownDaemon triggers a graceful daemon shutdown.
func (c *Client) ShutdownDaemon() error {
	_, err := c.call("ShutdownDaemon", nil)
	return err
}

// GetDaemonStatus returns the current daemon state.
func (c *Client) GetDaemonStatus() (DaemonStatusResult, error) {
	result, err := c.call("GetDaemonStatus", nil)
	if err != nil {
		return DaemonStatusResult{}, err
	}
	var status DaemonStatusResult
	if err := json.Unmarshal(result, &status); err != nil {
		return DaemonStatusResult{}, fmt.Errorf("unmarshal daemon status: %w", err)
	}
	return status, nil
}

// ListAgentHistory returns agent history for the given time window (in hours).
func (c *Client) ListAgentHistory(windowHours int) ([]cccdb.AgentHistoryEntry, error) {
	result, err := c.call("ListAgentHistory", ListAgentHistoryParams{WindowHours: windowHours})
	if err != nil {
		return nil, err
	}
	var resp ListAgentHistoryResult
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal agent history: %w", err)
	}
	return resp.Entries, nil
}

// StreamAgentOutput returns the current event buffer for a running agent.
func (c *Client) StreamAgentOutput(agentID string) (StreamAgentOutputResult, error) {
	result, err := c.call("StreamAgentOutput", StreamAgentOutputParams{AgentID: agentID})
	if err != nil {
		return StreamAgentOutputResult{}, err
	}
	var resp StreamAgentOutputResult
	if err := json.Unmarshal(result, &resp); err != nil {
		return StreamAgentOutputResult{}, fmt.Errorf("unmarshal agent output: %w", err)
	}
	return resp, nil
}

// ReportLLMActivity reports an LLM activity event to the daemon.
func (c *Client) ReportLLMActivity(evt LLMActivityEvent) error {
	_, err := c.call("ReportLLMActivity", evt)
	return err
}

// ListLLMActivity returns all LLM activity events from the daemon's ring buffer.
func (c *Client) ListLLMActivity() ([]LLMActivityEvent, error) {
	result, err := c.call("ListLLMActivity", nil)
	if err != nil {
		return nil, err
	}
	var events []LLMActivityEvent
	if err := json.Unmarshal(result, &events); err != nil {
		return nil, fmt.Errorf("unmarshal llm activity: %w", err)
	}
	return events, nil
}

// Subscribe blocks, reading events forever. Must be called on a dedicated Client
// instance — this connection cannot be used for RPCs after subscribing.
func (c *Client) Subscribe(handler func(Event)) error {
	_, err := c.call("Subscribe", nil)
	if err != nil {
		return err
	}
	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			return err
		}
		var evt Event
		if err := json.Unmarshal(line, &evt); err != nil {
			continue // skip malformed events
		}
		handler(evt)
	}
}
