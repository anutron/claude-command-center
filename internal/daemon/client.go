package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
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
