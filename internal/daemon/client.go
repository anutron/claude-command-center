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
