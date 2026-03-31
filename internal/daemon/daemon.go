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
	"sync/atomic"
	"syscall"
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
	GovernedRunner  *agent.GovernedRunner // optional; enables budget RPCs
	BinaryPath      string               // path to the daemon binary (for staleness detection)
	BinaryMtime     time.Time            // mtime of the binary at startup
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
	governed    *agent.GovernedRunner // non-nil when budget governance is enabled
	paused      atomic.Bool           // when true, refresh and agent launches are blocked
}

// NewServer creates a new daemon server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		cfg:      cfg,
		ctx:      ctx,
		cancel:   cancel,
		registry: newSessionRegistry(cfg.DB),
		runner:   cfg.AgentRunner,
		governed: cfg.GovernedRunner,
	}
	// Wire refresh loop with a post-refresh callback that broadcasts
	// data.refreshed to subscribers and prunes dead sessions.
	s.refresh = newRefreshLoop(cfg.RefreshFunc, cfg.RefreshInterval, func() {
		s.registry.pruneDead()
		s.Broadcast(Event{Type: "data.refreshed"})
	})
	return s
}

// Serve starts listening on the Unix socket and accepting connections.
// It blocks until Shutdown is called or an unrecoverable error occurs.
func (s *Server) Serve() error {
	// Remove stale socket file if it exists.
	os.Remove(s.cfg.SocketPath)

	// Set restrictive umask before creating the socket so it is never
	// world-accessible, even briefly (avoids TOCTOU race with Chmod).
	oldMask := syscall.Umask(0177)
	ln, err := net.Listen("unix", s.cfg.SocketPath)
	syscall.Umask(oldMask)
	if err != nil {
		return fmt.Errorf("daemon listen: %w", err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.refresh.start()

	// Start binary staleness monitor.
	if s.cfg.BinaryPath != "" && !s.cfg.BinaryMtime.IsZero() {
		go s.monitorBinaryStaleness()
	}

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

// monitorBinaryStaleness checks every 30 seconds whether the daemon binary has
// been updated on disk. If the binary's mtime is newer than at startup, the
// daemon shuts down and re-execs itself.
func (s *Server) monitorBinaryStaleness() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(s.cfg.BinaryPath)
			if err != nil {
				// Binary deleted or unreadable — skip silently.
				continue
			}
			if info.ModTime().After(s.cfg.BinaryMtime) {
				fmt.Printf("Binary updated (was %s, now %s), restarting...\n",
					s.cfg.BinaryMtime.Format(time.RFC3339),
					info.ModTime().Format(time.RFC3339))
				s.Shutdown()
				// Re-exec ourselves with the same arguments.
				if err := syscall.Exec(s.cfg.BinaryPath, os.Args, os.Environ()); err != nil {
					fmt.Printf("Re-exec failed: %v\n", err)
					os.Exit(1)
				}
				return // unreachable after successful Exec
			}
		}
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
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // 4MB max for large agent prompts
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
		go func() {
			if err := s.refresh.run(); err == nil {
				s.registry.pruneDead()
				s.Broadcast(Event{Type: "data.refreshed"})
			}
		}()
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

	case "EndSession":
		var params EndSessionParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
		}
		if err := s.registry.end(params.SessionID); err != nil {
			return nil, &RPCError{Code: -32000, Message: err.Error()}
		}
		return map[string]bool{"ok": true}, nil

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

	case "GetBudgetStatus":
		return s.handleGetBudgetStatus(req)
	case "StopAllAgents":
		return s.handleStopAllAgents(req)
	case "ResumeAgents":
		return s.handleResumeAgents(req)

	case "PauseDaemon":
		return s.handlePauseDaemon(req)
	case "ResumeDaemon":
		return s.handleResumeDaemon(req)
	case "ShutdownDaemon":
		return s.handleShutdownDaemon(req)
	case "GetDaemonStatus":
		return s.handleGetDaemonStatus(req)

	default:
		return nil, &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}
}

// Paused returns whether the daemon is in paused state.
func (s *Server) Paused() bool {
	return s.paused.Load()
}

func (s *Server) handlePauseDaemon(req *RPCRequest) (interface{}, *RPCError) {
	s.paused.Store(true)
	s.refresh.paused.Store(true)

	data, _ := json.Marshal(map[string]bool{"paused": true})
	s.Broadcast(Event{Type: "daemon.paused", Data: data})

	return map[string]bool{"ok": true}, nil
}

func (s *Server) handleResumeDaemon(req *RPCRequest) (interface{}, *RPCError) {
	s.paused.Store(false)
	s.refresh.paused.Store(false)

	data, _ := json.Marshal(map[string]bool{"resumed": true})
	s.Broadcast(Event{Type: "daemon.resumed", Data: data})

	return map[string]bool{"ok": true}, nil
}

func (s *Server) handleShutdownDaemon(req *RPCRequest) (interface{}, *RPCError) {
	// Respond first, then shut down asynchronously.
	go func() {
		time.Sleep(100 * time.Millisecond) // let response flush
		s.Shutdown()
	}()
	return map[string]bool{"ok": true}, nil
}

func (s *Server) handleGetDaemonStatus(req *RPCRequest) (interface{}, *RPCError) {
	state := "running"
	if s.paused.Load() {
		state = "paused"
	}

	var activeAgents int
	if s.runner != nil {
		activeAgents = len(s.runner.Active())
	}

	return DaemonStatusResult{
		State:        state,
		ActiveAgents: activeAgents,
	}, nil
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
