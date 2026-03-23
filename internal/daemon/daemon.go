package daemon

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
)

// ServerConfig holds the configuration for a daemon server.
type ServerConfig struct {
	SocketPath      string
	DB              *sql.DB
	RefreshFunc     func() error
	RefreshInterval time.Duration
	AgentRunner     agent.Runner
}

// Server listens on a Unix socket and dispatches JSON-RPC requests.
type Server struct {
	cfg         ServerConfig
	listener    net.Listener
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	clients     []net.Conn
	subscribers subscriberSet
	registry    *sessionRegistry
	refresh     *refreshLoop
	runner      agent.Runner
}

// NewServer creates a new daemon server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		cfg:      cfg,
		ctx:      ctx,
		cancel:   cancel,
		registry: newSessionRegistry(cfg.DB),
		refresh:  newRefreshLoop(cfg.RefreshFunc, cfg.RefreshInterval),
		runner:   cfg.AgentRunner,
	}
}

// Serve starts listening on the Unix socket and accepting connections.
// It blocks until Shutdown is called or an unrecoverable error occurs.
func (s *Server) Serve() error {
	// Remove stale socket file if it exists.
	os.Remove(s.cfg.SocketPath)

	ln, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("daemon listen: %w", err)
	}
	// Set socket permissions to owner-only.
	os.Chmod(s.cfg.SocketPath, 0600)

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.refresh.start()

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Check if we're shutting down.
			select {
			case <-s.ctx.Done():
				return nil
			default:
				return fmt.Errorf("daemon accept: %w", err)
			}
		}
		s.mu.Lock()
		s.clients = append(s.clients, conn)
		s.mu.Unlock()

		go s.handleConn(conn)
	}
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() {
	if s.runner != nil {
		s.runner.Shutdown()
	}
	s.refresh.stop()
	s.cancel()

	s.mu.Lock()
	if s.listener != nil {
		s.listener.Close()
	}
	for _, c := range s.clients {
		c.Close()
	}
	s.clients = nil
	s.mu.Unlock()

	os.Remove(s.cfg.SocketPath)
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.removeClient(conn)
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		var req RPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			resp := RPCResponse{
				Error: &RPCError{Code: -32700, Message: "parse error"},
			}
			WriteMessage(conn, resp)
			continue
		}

		// Subscribe is special: after OK response, the conn becomes push-only.
		if req.Method == "Subscribe" {
			resp := RPCResponse{ID: req.ID}
			raw, _ := json.Marshal(map[string]bool{"ok": true})
			resp.Result = raw
			WriteMessage(conn, resp)
			s.subscribers.add(conn)
			// Block until shutdown — the conn is now a subscriber.
			<-s.ctx.Done()
			s.subscribers.remove(conn)
			return
		}

		result, rpcErr := s.dispatch(&req)
		resp := RPCResponse{ID: req.ID}
		if rpcErr != nil {
			resp.Error = rpcErr
		} else if result != nil {
			raw, _ := json.Marshal(result)
			resp.Result = raw
		}
		WriteMessage(conn, resp)
	}
}

func (s *Server) dispatch(req *RPCRequest) (interface{}, *RPCError) {
	switch req.Method {
	case "Ping":
		return map[string]bool{"ok": true}, nil
	case "Refresh":
		go s.refresh.run()
		return map[string]bool{"ok": true}, nil

	case "RegisterSession":
		var params RegisterSessionParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
		}
		if err := s.registry.register(params); err != nil {
			return nil, &RPCError{Code: -32000, Message: err.Error()}
		}
		return map[string]bool{"ok": true}, nil

	case "UpdateSession":
		var params UpdateSessionParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
		}
		if err := s.registry.update(params); err != nil {
			return nil, &RPCError{Code: -32000, Message: err.Error()}
		}
		return map[string]bool{"ok": true}, nil

	case "ListSessions":
		sessions := s.registry.list()
		return sessions, nil

	case "ArchiveSession":
		var params ArchiveSessionParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
		}
		if err := s.registry.archive(params.SessionID); err != nil {
			return nil, &RPCError{Code: -32000, Message: err.Error()}
		}
		return map[string]bool{"ok": true}, nil

	case "LaunchAgent":
		return s.handleLaunchAgent(req)
	case "StopAgent":
		return s.handleStopAgent(req)
	case "AgentStatus":
		return s.handleAgentStatus(req)
	case "ListAgents":
		return s.handleListAgents(req)
	case "SendAgentInput":
		return s.handleSendAgentInput(req)

	default:
		return nil, &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}
}

func (s *Server) removeClient(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.clients {
		if c == conn {
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			return
		}
	}
}
