# CCC Daemon + Session Registry + Agent Runner Migration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ai-cron with a persistent daemon that owns the refresh cycle and session registry, add a sessions UI for cross-terminal visibility, and migrate the agent runner into the daemon so agents survive TUI restarts.

**Architecture:** Three-phase evolution. Phase 1 builds the daemon (JSON-RPC over Unix socket) with refresh loop and session registry. Phase 2 adds the sessions UI to the sessions plugin. Phase 3 moves the agent runner from `internal/agent/` behind daemon RPCs. Each phase is independently shippable.

**Tech Stack:** Go, `net` (Unix sockets), `encoding/json` (JSON-RPC), SQLite (existing), bubbletea (existing TUI)

**Spec:** `docs/superpowers/specs/2026-03-22-ccc-daemon-sessions-design.md`

---

## Phase 1: Daemon Core + Session Registry

### File Structure

**Create:**
- `internal/daemon/daemon.go` — Daemon server: lifecycle, socket listener, signal handling
- `internal/daemon/rpc.go` — JSON-RPC request/response handling, method dispatch
- `internal/daemon/refresh.go` — Refresh loop (timer + on-demand trigger)
- `internal/daemon/sessions.go` — Session registry: register, update, liveness, pruning
- `internal/daemon/types.go` — Shared RPC request/response types, session record
- `internal/daemon/client.go` — Client library for TUI and CLI to talk to daemon
- `internal/daemon/daemon_test.go` — Daemon integration tests
- `internal/daemon/sessions_test.go` — Session registry unit tests
- `internal/daemon/client_test.go` — Client unit tests
- `examples/hooks/session-register.sh` — Example Claude Code hook for session registration

**Modify:**
- `cmd/ccc/main.go` — Add `daemon start|stop|status`, `register`, `update-session`, `refresh` subcommands
- `internal/db/schema.go` — Add `cc_sessions` table
- `internal/db/read.go` — Add `DBLoadSessions()`, `DBLoadActiveSessions()`
- `internal/db/write.go` — Add `DBInsertSession()`, `DBUpdateSession()`, `DBUpdateSessionState()`
- `internal/config/config.go` — Add `Daemon` config section (refresh interval, session retention)
- `internal/tui/model.go` — Connect to daemon on startup, handle reconnection
- `internal/lockfile/lockfile.go` — Add `TODO(daemon-stable)` comment, add skipped removal test
- `go.mod` — No new dependencies expected (stdlib `net`, `encoding/json` sufficient)

---

### Task 1: cc_sessions Schema + DB Functions

**Files:**
- Modify: `internal/db/schema.go`
- Modify: `internal/db/write.go`
- Modify: `internal/db/read.go`
- Create: `internal/db/sessions_test.go`

- [ ] **Step 1: Write failing tests for session DB operations**

```go
// internal/db/sessions_test.go
package db_test

import (
    "database/sql"
    "testing"
    "time"

    "github.com/anutron/claude-command-center/internal/db"
)

func setupTestDB(t *testing.T) *sql.DB {
    t.Helper()
    d, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { d.Close() })
    return d
}

func TestInsertAndLoadSession(t *testing.T) {
    d := setupTestDB(t)
    sess := db.SessionRecord{
        SessionID:    "sess-123",
        PID:          12345,
        Project:      "/home/user/project",
        Repo:         "owner/repo",
        Branch:       "main",
        State:        "active",
        RegisteredAt: time.Now().UTC().Format(time.RFC3339),
    }
    err := db.DBInsertSession(d, sess)
    if err != nil {
        t.Fatal(err)
    }
    sessions, err := db.DBLoadActiveSessions(d)
    if err != nil {
        t.Fatal(err)
    }
    if len(sessions) != 1 {
        t.Fatalf("expected 1 session, got %d", len(sessions))
    }
    if sessions[0].SessionID != "sess-123" {
        t.Fatalf("expected sess-123, got %s", sessions[0].SessionID)
    }
}

func TestUpdateSessionTopic(t *testing.T) {
    d := setupTestDB(t)
    sess := db.SessionRecord{
        SessionID:    "sess-456",
        PID:          12345,
        Project:      "/home/user/project",
        State:        "active",
        RegisteredAt: time.Now().UTC().Format(time.RFC3339),
    }
    db.DBInsertSession(d, sess)
    err := db.DBUpdateSession(d, "sess-456", map[string]interface{}{"topic": "Refactoring auth"})
    if err != nil {
        t.Fatal(err)
    }
    sessions, _ := db.DBLoadActiveSessions(d)
    if sessions[0].Topic != "Refactoring auth" {
        t.Fatalf("expected topic 'Refactoring auth', got '%s'", sessions[0].Topic)
    }
}

func TestUpdateSessionState(t *testing.T) {
    d := setupTestDB(t)
    sess := db.SessionRecord{
        SessionID:    "sess-789",
        PID:          99999,
        Project:      "/tmp/proj",
        State:        "active",
        RegisteredAt: time.Now().UTC().Format(time.RFC3339),
    }
    db.DBInsertSession(d, sess)
    err := db.DBUpdateSessionState(d, "sess-789", "ended")
    if err != nil {
        t.Fatal(err)
    }
    all, _ := db.DBLoadSessions(d) // all states
    if all[0].State != "ended" {
        t.Fatalf("expected ended, got %s", all[0].State)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/ -run TestInsertAndLoadSession -v`
Expected: FAIL — `SessionRecord` type not defined, functions not defined

- [ ] **Step 3: Add SessionRecord type to db/types.go**

Add to `internal/db/types.go`:

```go
type SessionRecord struct {
    SessionID    string
    Topic        string
    PID          int
    Project      string
    Repo         string
    Branch       string
    WorktreePath string
    State        string // active | ended | archived
    RegisteredAt string
    EndedAt      string
}
```

- [ ] **Step 4: Add cc_sessions table to schema.go**

Add the CREATE TABLE statement to the schema initialization in `internal/db/schema.go`, alongside existing table creation:

```sql
CREATE TABLE IF NOT EXISTS cc_sessions (
    session_id TEXT PRIMARY KEY,
    topic TEXT,
    pid INTEGER,
    project TEXT,
    repo TEXT,
    branch TEXT,
    worktree_path TEXT,
    state TEXT NOT NULL DEFAULT 'active',
    registered_at TEXT NOT NULL,
    ended_at TEXT
);
```

- [ ] **Step 5: Add DB write functions to write.go**

Add to `internal/db/write.go`:

```go
func DBInsertSession(d *sql.DB, s SessionRecord) error {
    _, err := d.Exec(`INSERT OR REPLACE INTO cc_sessions
        (session_id, topic, pid, project, repo, branch, worktree_path, state, registered_at, ended_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        s.SessionID, s.Topic, s.PID, s.Project, s.Repo, s.Branch,
        s.WorktreePath, s.State, s.RegisteredAt, s.EndedAt)
    return err
}

func DBUpdateSession(d *sql.DB, sessionID string, fields map[string]interface{}) error {
    // Build SET clause dynamically from fields map
    // Only allow known fields: topic, pid, project, repo, branch, worktree_path, state, ended_at
    allowed := map[string]bool{"topic": true, "pid": true, "project": true, "repo": true,
        "branch": true, "worktree_path": true, "state": true, "ended_at": true}
    var setClauses []string
    var args []interface{}
    for k, v := range fields {
        if !allowed[k] {
            return fmt.Errorf("unknown session field: %s", k)
        }
        setClauses = append(setClauses, k+" = ?")
        args = append(args, v)
    }
    if len(setClauses) == 0 {
        return nil
    }
    args = append(args, sessionID)
    _, err := d.Exec("UPDATE cc_sessions SET "+strings.Join(setClauses, ", ")+" WHERE session_id = ?", args...)
    return err
}

func DBUpdateSessionState(d *sql.DB, sessionID, state string) error {
    endedAt := ""
    if state == "ended" {
        endedAt = time.Now().UTC().Format(time.RFC3339)
    }
    _, err := d.Exec("UPDATE cc_sessions SET state = ?, ended_at = ? WHERE session_id = ?",
        state, endedAt, sessionID)
    return err
}
```

- [ ] **Step 6: Add DB read functions to read.go**

Add to `internal/db/read.go`:

```go
func DBLoadSessions(d *sql.DB) ([]SessionRecord, error) {
    rows, err := d.Query(`SELECT session_id, COALESCE(topic,''), COALESCE(pid,0),
        COALESCE(project,''), COALESCE(repo,''), COALESCE(branch,''),
        COALESCE(worktree_path,''), state, registered_at, COALESCE(ended_at,'')
        FROM cc_sessions ORDER BY registered_at DESC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var sessions []SessionRecord
    for rows.Next() {
        var s SessionRecord
        if err := rows.Scan(&s.SessionID, &s.Topic, &s.PID, &s.Project, &s.Repo,
            &s.Branch, &s.WorktreePath, &s.State, &s.RegisteredAt, &s.EndedAt); err != nil {
            return nil, err
        }
        sessions = append(sessions, s)
    }
    return sessions, nil
}

func DBLoadActiveSessions(d *sql.DB) ([]SessionRecord, error) {
    all, err := DBLoadSessions(d)
    if err != nil {
        return nil, err
    }
    var active []SessionRecord
    for _, s := range all {
        if s.State == "active" || s.State == "ended" {
            active = append(active, s)
        }
    }
    return active, nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/db/ -run "TestInsertAndLoadSession|TestUpdateSessionTopic|TestUpdateSessionState" -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/db/
git commit -m "Add cc_sessions table and DB functions for session registry"
```

---

### Task 2: Daemon Types + JSON-RPC Protocol

**Files:**
- Create: `internal/daemon/types.go`
- Create: `internal/daemon/rpc.go`
- Create: `internal/daemon/rpc_test.go`

- [ ] **Step 1: Write failing test for JSON-RPC encoding/decoding**

```go
// internal/daemon/rpc_test.go
package daemon_test

import (
    "bytes"
    "testing"

    "github.com/anutron/claude-command-center/internal/daemon"
)

func TestEncodeDecodeRequest(t *testing.T) {
    req := daemon.RPCRequest{
        Method: "RegisterSession",
        ID:     1,
        Params: json.RawMessage(`{"session_id":"abc"}`),
    }
    var buf bytes.Buffer
    err := daemon.WriteMessage(&buf, req)
    if err != nil {
        t.Fatal(err)
    }
    var decoded daemon.RPCRequest
    err = daemon.ReadMessage(&buf, &decoded)
    if err != nil {
        t.Fatal(err)
    }
    if decoded.Method != "RegisterSession" {
        t.Fatalf("expected RegisterSession, got %s", decoded.Method)
    }
}

func TestEncodeDecodeResponse(t *testing.T) {
    resp := daemon.RPCResponse{
        ID:     1,
        Result: json.RawMessage(`{"ok":true}`),
    }
    var buf bytes.Buffer
    daemon.WriteMessage(&buf, resp)
    var decoded daemon.RPCResponse
    daemon.ReadMessage(&buf, &decoded)
    if decoded.ID != 1 {
        t.Fatalf("expected ID 1, got %d", decoded.ID)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run TestEncodeDecode -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Create types.go with RPC types and session request/response types**

```go
// internal/daemon/types.go
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
```

- [ ] **Step 4: Create rpc.go with message encoding**

```go
// internal/daemon/rpc.go
package daemon

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
)

// WriteMessage writes a newline-delimited JSON message.
func WriteMessage(w io.Writer, v interface{}) error {
    data, err := json.Marshal(v)
    if err != nil {
        return err
    }
    _, err = fmt.Fprintf(w, "%s\n", data)
    return err
}

// ReadMessage reads a newline-delimited JSON message.
func ReadMessage(r io.Reader, v interface{}) error {
    scanner := bufio.NewScanner(r)
    if !scanner.Scan() {
        if err := scanner.Err(); err != nil {
            return err
        }
        return io.EOF
    }
    return json.Unmarshal(scanner.Bytes(), v)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run TestEncodeDecode -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/
git commit -m "Add daemon RPC types and JSON-RPC wire protocol"
```

---

### Task 3: Daemon Server Skeleton

**Files:**
- Create: `internal/daemon/daemon.go`
- Create: `internal/daemon/daemon_test.go`

- [ ] **Step 1: Write failing test for daemon start/stop/connect**

```go
// internal/daemon/daemon_test.go
package daemon_test

import (
    "database/sql"
    "path/filepath"
    "testing"
    "time"

    "github.com/anutron/claude-command-center/internal/daemon"
    "github.com/anutron/claude-command-center/internal/db"
)

func testDB(t *testing.T) *sql.DB {
    t.Helper()
    d, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { d.Close() })
    return d
}

func TestDaemonStartStop(t *testing.T) {
    dir := t.TempDir()
    d := testDB(t)
    srv := daemon.NewServer(daemon.ServerConfig{
        SocketPath: filepath.Join(dir, "daemon.sock"),
        DB:         d,
    })
    go srv.Serve()
    defer srv.Shutdown()

    // Wait for socket to appear
    time.Sleep(50 * time.Millisecond)

    client, err := daemon.NewClient(filepath.Join(dir, "daemon.sock"))
    if err != nil {
        t.Fatal(err)
    }
    defer client.Close()

    // Ping should work
    err = client.Ping()
    if err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run TestDaemonStartStop -v`
Expected: FAIL — `NewServer`, `NewClient`, `Ping` not defined

- [ ] **Step 3: Implement daemon server**

Create `internal/daemon/daemon.go`:

```go
package daemon

import (
    "context"
    "database/sql"
    "encoding/json"
    "log"
    "net"
    "os"
    "sync"
)

type ServerConfig struct {
    SocketPath string
    DB         *sql.DB
}

type Server struct {
    cfg       ServerConfig
    listener  net.Listener
    ctx       context.Context
    cancel    context.CancelFunc
    mu        sync.Mutex
    clients   []net.Conn
    subscribers []net.Conn
}

func NewServer(cfg ServerConfig) *Server {
    ctx, cancel := context.WithCancel(context.Background())
    return &Server{cfg: cfg, ctx: ctx, cancel: cancel}
}

func (s *Server) Serve() error {
    os.Remove(s.cfg.SocketPath) // clean stale socket
    ln, err := net.Listen("unix", s.cfg.SocketPath)
    if err != nil {
        return err
    }
    os.Chmod(s.cfg.SocketPath, 0600)
    s.listener = ln

    for {
        conn, err := ln.Accept()
        if err != nil {
            select {
            case <-s.ctx.Done():
                return nil
            default:
                log.Printf("accept error: %v", err)
                continue
            }
        }
        go s.handleConn(conn)
    }
}

func (s *Server) Shutdown() {
    s.cancel()
    if s.listener != nil {
        s.listener.Close()
    }
    s.mu.Lock()
    for _, c := range s.clients {
        c.Close()
    }
    s.mu.Unlock()
    os.Remove(s.cfg.SocketPath)
}

func (s *Server) handleConn(conn net.Conn) {
    s.mu.Lock()
    s.clients = append(s.clients, conn)
    s.mu.Unlock()
    defer conn.Close()

    for {
        var req RPCRequest
        if err := ReadMessage(conn, &req); err != nil {
            return
        }
        resp := s.dispatch(req)
        WriteMessage(conn, resp)
    }
}

func (s *Server) dispatch(req RPCRequest) RPCResponse {
    switch req.Method {
    case "Ping":
        result, _ := json.Marshal(map[string]bool{"ok": true})
        return RPCResponse{ID: req.ID, Result: result}
    default:
        return RPCResponse{ID: req.ID, Error: &RPCError{Code: -1, Message: "unknown method: " + req.Method}}
    }
}
```

- [ ] **Step 4: Implement client**

Create `internal/daemon/client.go`:

```go
package daemon

import (
    "encoding/json"
    "fmt"
    "net"
    "sync"
    "sync/atomic"
)

type Client struct {
    conn  net.Conn
    mu    sync.Mutex
    nextID atomic.Int64
}

func NewClient(socketPath string) (*Client, error) {
    conn, err := net.Dial("unix", socketPath)
    if err != nil {
        return nil, fmt.Errorf("connect to daemon: %w", err)
    }
    return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
    return c.conn.Close()
}

func (c *Client) call(method string, params interface{}) (json.RawMessage, error) {
    c.mu.Lock()
    defer c.mu.Unlock()

    id := int(c.nextID.Add(1))
    var rawParams json.RawMessage
    if params != nil {
        var err error
        rawParams, err = json.Marshal(params)
        if err != nil {
            return nil, err
        }
    }
    req := RPCRequest{Method: method, ID: id, Params: rawParams}
    if err := WriteMessage(c.conn, req); err != nil {
        return nil, err
    }
    var resp RPCResponse
    if err := ReadMessage(c.conn, &resp); err != nil {
        return nil, err
    }
    if resp.Error != nil {
        return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
    }
    return resp.Result, nil
}

func (c *Client) Ping() error {
    _, err := c.call("Ping", nil)
    return err
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/daemon/ -run TestDaemonStartStop -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/
git commit -m "Add daemon server skeleton with Unix socket and client"
```

---

### Task 4: Session Registry RPCs

**Files:**
- Modify: `internal/daemon/daemon.go` — Add session state + dispatch methods
- Create: `internal/daemon/sessions.go` — Session registry logic
- Create: `internal/daemon/sessions_test.go` — Integration tests via client

- [ ] **Step 1: Write failing tests for session registration and listing**

```go
// internal/daemon/sessions_test.go
package daemon_test

import (
    "path/filepath"
    "testing"
    "time"

    "github.com/anutron/claude-command-center/internal/daemon"
)

func startTestDaemon(t *testing.T) (*daemon.Server, *daemon.Client) {
    t.Helper()
    dir := t.TempDir()
    d := testDB(t)
    sockPath := filepath.Join(dir, "daemon.sock")
    srv := daemon.NewServer(daemon.ServerConfig{
        SocketPath: sockPath,
        DB:         d,
    })
    go srv.Serve()
    t.Cleanup(srv.Shutdown)
    time.Sleep(50 * time.Millisecond)
    client, err := daemon.NewClient(sockPath)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { client.Close() })
    return srv, client
}

func TestRegisterAndListSessions(t *testing.T) {
    _, client := startTestDaemon(t)

    err := client.RegisterSession(daemon.RegisterSessionParams{
        SessionID: "sess-abc",
        PID:       os.Getpid(), // use own PID so liveness check passes
        Project:   "/tmp/myproject",
    })
    if err != nil {
        t.Fatal(err)
    }

    sessions, err := client.ListSessions()
    if err != nil {
        t.Fatal(err)
    }
    if len(sessions) != 1 {
        t.Fatalf("expected 1 session, got %d", len(sessions))
    }
    if sessions[0].SessionID != "sess-abc" {
        t.Fatalf("expected sess-abc, got %s", sessions[0].SessionID)
    }
    if sessions[0].State != "active" {
        t.Fatalf("expected active, got %s", sessions[0].State)
    }
}

func TestUpdateSessionTopic(t *testing.T) {
    _, client := startTestDaemon(t)

    client.RegisterSession(daemon.RegisterSessionParams{
        SessionID: "sess-topic",
        PID:       os.Getpid(),
        Project:   "/tmp/proj",
    })

    err := client.UpdateSession(daemon.UpdateSessionParams{
        SessionID: "sess-topic",
        Topic:     "Auth refactor",
    })
    if err != nil {
        t.Fatal(err)
    }

    sessions, _ := client.ListSessions()
    if sessions[0].Topic != "Auth refactor" {
        t.Fatalf("expected 'Auth refactor', got '%s'", sessions[0].Topic)
    }
}

func TestLivenessDetection(t *testing.T) {
    _, client := startTestDaemon(t)

    // Register with a PID that doesn't exist
    client.RegisterSession(daemon.RegisterSessionParams{
        SessionID: "sess-dead",
        PID:       999999999, // won't exist
        Project:   "/tmp/proj",
    })

    // Trigger a liveness check
    // The daemon should detect the PID is dead and mark ended
    // This test depends on the daemon's liveness ticker running
    // For testing, expose a manual prune method
    sessions, _ := client.ListSessions()
    // After prune, session should be ended
    // (Implementation detail: either expose PruneSessions RPC or
    // check after a short sleep if daemon runs ticker fast in tests)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run "TestRegisterAndList|TestUpdateSessionTopic" -v`
Expected: FAIL — methods not defined

- [ ] **Step 3: Implement sessions.go with registry logic**

Create `internal/daemon/sessions.go` — in-memory registry backed by DB:

```go
package daemon

import (
    "database/sql"
    "encoding/json"
    "os"
    "sync"
    "time"

    "github.com/anutron/claude-command-center/internal/db"
)

type sessionRegistry struct {
    mu       sync.RWMutex
    sessions map[string]*SessionInfo
    database *sql.DB
}

func newSessionRegistry(database *sql.DB) *sessionRegistry {
    r := &sessionRegistry{
        sessions: make(map[string]*SessionInfo),
        database: database,
    }
    r.loadFromDB()
    r.pruneDead()
    return r
}

func (r *sessionRegistry) loadFromDB() {
    records, err := db.DBLoadSessions(r.database)
    if err != nil {
        return
    }
    for _, rec := range records {
        info := sessionRecordToInfo(rec)
        r.sessions[rec.SessionID] = &info
    }
}

func (r *sessionRegistry) register(params RegisterSessionParams) (*SessionInfo, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now().UTC().Format(time.RFC3339)
    // Derive repo and branch from project directory
    repo, branch := gitInfoFromDir(params.Project)

    info := &SessionInfo{
        SessionID:    params.SessionID,
        PID:          params.PID,
        Project:      params.Project,
        Repo:         repo,
        Branch:       branch,
        WorktreePath: params.WorktreePath,
        State:        "active",
        RegisteredAt: now,
    }
    r.sessions[params.SessionID] = info

    // Synchronous DB write
    db.DBInsertSession(r.database, infoToRecord(*info))
    return info, nil
}

func (r *sessionRegistry) update(params UpdateSessionParams) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    info, ok := r.sessions[params.SessionID]
    if !ok {
        return fmt.Errorf("session not found: %s", params.SessionID)
    }
    if params.Topic != "" {
        info.Topic = params.Topic
    }
    db.DBUpdateSession(r.database, params.SessionID, map[string]interface{}{"topic": info.Topic})
    return nil
}

func (r *sessionRegistry) list() []SessionInfo {
    r.mu.RLock()
    defer r.mu.RUnlock()
    var result []SessionInfo
    for _, info := range r.sessions {
        if info.State != "archived" {
            result = append(result, *info)
        }
    }
    return result
}

func (r *sessionRegistry) pruneDead() []string {
    r.mu.Lock()
    defer r.mu.Unlock()
    var ended []string
    for id, info := range r.sessions {
        if info.State != "active" {
            continue
        }
        if !processAlive(info.PID) {
            info.State = "ended"
            info.EndedAt = time.Now().UTC().Format(time.RFC3339)
            db.DBUpdateSessionState(r.database, id, "ended")
            ended = append(ended, id)
        }
    }
    return ended
}

func processAlive(pid int) bool {
    if pid <= 0 {
        return false
    }
    proc, err := os.FindProcess(pid)
    if err != nil {
        return false
    }
    // kill -0 checks existence without sending a signal
    err = proc.Signal(syscall.Signal(0))
    return err == nil
}

func gitInfoFromDir(dir string) (repo, branch string) {
    // Best-effort: run git commands to get remote and branch
    // If git isn't available or dir isn't a repo, return empty strings
    // Implementation: exec.Command("git", "-C", dir, "remote", "get-url", "origin")
    // and exec.Command("git", "-C", dir, "branch", "--show-current")
    return "", ""
}
```

- [ ] **Step 4: Wire session RPCs into daemon dispatch**

Add to `internal/daemon/daemon.go`:
- Add `registry *sessionRegistry` field to `Server`
- Initialize in `NewServer`: `registry: newSessionRegistry(cfg.DB)`
- Add dispatch cases for `RegisterSession`, `UpdateSession`, `ListSessions`
- Start a liveness ticker goroutine (30s interval) that calls `registry.pruneDead()` and emits `session.ended` events
- In the same ticker, call `registry.archiveStale(retentionDuration)` — moves sessions in `ended` state older than `session_retention` config to `archived` state (hidden from TUI, kept in DB)

- [ ] **Step 5: Add client methods**

Add to `internal/daemon/client.go`:

```go
func (c *Client) RegisterSession(params RegisterSessionParams) error {
    _, err := c.call("RegisterSession", params)
    return err
}

func (c *Client) UpdateSession(params UpdateSessionParams) error {
    _, err := c.call("UpdateSession", params)
    return err
}

func (c *Client) ListSessions() ([]SessionInfo, error) {
    result, err := c.call("ListSessions", nil)
    if err != nil {
        return nil, err
    }
    var sessions []SessionInfo
    json.Unmarshal(result, &sessions)
    return sessions, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run "TestRegisterAndList|TestUpdateSessionTopic" -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/
git commit -m "Add session registry with register, update, list, and liveness detection"
```

---

### Task 5: Event Subscription

**Files:**
- Modify: `internal/daemon/daemon.go` — Subscriber management
- Modify: `internal/daemon/client.go` — Subscribe + event reader
- Create: `internal/daemon/events_test.go`

- [ ] **Step 1: Write failing test for event subscription**

```go
// internal/daemon/events_test.go
package daemon_test

func TestSubscribeReceivesSessionEvents(t *testing.T) {
    _, client := startTestDaemon(t)

    // Start a subscriber in background
    events := make(chan daemon.Event, 10)
    subClient, _ := daemon.NewClient(sockPath) // need second connection
    go subClient.Subscribe(func(e daemon.Event) {
        events <- e
    })

    // Register a session — should produce session.registered event
    client.RegisterSession(daemon.RegisterSessionParams{
        SessionID: "sess-evt",
        PID:       os.Getpid(),
        Project:   "/tmp/proj",
    })

    select {
    case evt := <-events:
        if evt.Type != "session.registered" {
            t.Fatalf("expected session.registered, got %s", evt.Type)
        }
    case <-time.After(2 * time.Second):
        t.Fatal("timed out waiting for event")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run TestSubscribeReceives -v`
Expected: FAIL — `Subscribe` not defined

- [ ] **Step 3: Add subscriber tracking to server**

In `daemon.go`, add `subscribers []net.Conn` to the Server. Add a `Subscribe` RPC method that adds the connection to the subscribers list. Add a `broadcast(event Event)` method that writes the event to all subscribers.

Call `broadcast` after session registration, update, and liveness prune.

- [ ] **Step 4: Add Subscribe to client**

**Important:** Subscription requires a **dedicated connection** — the subscriber connection becomes unidirectional (daemon pushes events). RPC calls must use a separate `Client` instance. The TUI should create two connections: one for RPCs, one for event subscription.

```go
// Subscribe blocks, reading events forever. Must be called on a dedicated Client
// instance — this connection cannot be used for RPCs after subscribing.
func (c *Client) Subscribe(handler func(Event)) error {
    _, err := c.call("Subscribe", nil)
    if err != nil {
        return err
    }
    for {
        var evt Event
        if err := ReadMessage(c.conn, &evt); err != nil {
            return err
        }
        handler(evt)
    }
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/daemon/ -run TestSubscribeReceives -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/
git commit -m "Add event subscription for live session updates"
```

---

### Task 6: Refresh Loop in Daemon

**Files:**
- Create: `internal/daemon/refresh.go`
- Modify: `internal/daemon/daemon.go` — Start refresh loop, add Refresh RPC
- Create: `internal/daemon/refresh_test.go`

- [ ] **Step 1: Write failing test for refresh cycle**

Test that the daemon's refresh RPC triggers a refresh, and that the timer-based refresh works. Use a mock DataSource to avoid hitting real APIs.

```go
// internal/daemon/refresh_test.go
package daemon_test

func TestRefreshRPCTriggersRefresh(t *testing.T) {
    dir := t.TempDir()
    d := testDB(t)
    sockPath := filepath.Join(dir, "daemon.sock")

    refreshCalled := atomic.Bool{}
    srv := daemon.NewServer(daemon.ServerConfig{
        SocketPath:    sockPath,
        DB:            d,
        RefreshFunc:   func() error { refreshCalled.Store(true); return nil },
        RefreshInterval: 0, // disable timer for this test
    })
    go srv.Serve()
    t.Cleanup(srv.Shutdown)
    time.Sleep(50 * time.Millisecond)

    client, _ := daemon.NewClient(sockPath)
    defer client.Close()

    err := client.Refresh()
    if err != nil {
        t.Fatal(err)
    }
    if !refreshCalled.Load() {
        t.Fatal("refresh was not called")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run TestRefreshRPC -v`
Expected: FAIL — `RefreshFunc`, `Refresh` not defined

- [ ] **Step 3: Implement refresh.go**

```go
// internal/daemon/refresh.go
package daemon

import (
    "log"
    "sync"
    "time"
)

type refreshLoop struct {
    fn       func() error
    interval time.Duration
    mu       sync.Mutex
    running  bool
    stopCh   chan struct{}
}

func newRefreshLoop(fn func() error, interval time.Duration) *refreshLoop {
    return &refreshLoop{fn: fn, interval: interval, stopCh: make(chan struct{})}
}

func (r *refreshLoop) start() {
    if r.interval <= 0 {
        return
    }
    go func() {
        ticker := time.NewTicker(r.interval)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                r.run()
            case <-r.stopCh:
                return
            }
        }
    }()
}

func (r *refreshLoop) stop() {
    close(r.stopCh)
}

func (r *refreshLoop) run() error {
    r.mu.Lock()
    if r.running {
        r.mu.Unlock()
        return nil // already running, skip
    }
    r.running = true
    r.mu.Unlock()
    defer func() {
        r.mu.Lock()
        r.running = false
        r.mu.Unlock()
    }()
    return r.fn()
}
```

- [ ] **Step 4: Wire into daemon**

Add `RefreshFunc` and `RefreshInterval` to `ServerConfig`. In `NewServer`, create the refresh loop. In `Serve`, start it. Add `Refresh` to dispatch. Add `client.Refresh()` method. Broadcast `data.refreshed` event after each successful refresh.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/daemon/ -run TestRefreshRPC -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/
git commit -m "Add refresh loop with timer and on-demand RPC trigger"
```

---

### Task 7: Daemon Config

**Files:**
- Modify: `internal/config/config.go` — Add `Daemon` config section

- [ ] **Step 1: Write failing test for daemon config parsing**

```go
// Add to existing config tests
func TestDaemonConfigDefaults(t *testing.T) {
    cfg := config.Defaults()
    if cfg.Daemon.RefreshInterval != "5m" {
        t.Fatalf("expected 5m, got %s", cfg.Daemon.RefreshInterval)
    }
    if cfg.Daemon.SessionRetention != "7d" {
        t.Fatalf("expected 7d, got %s", cfg.Daemon.SessionRetention)
    }
}
```

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Add DaemonConfig to config.go**

```go
type DaemonConfig struct {
    RefreshInterval  string `yaml:"refresh_interval"`  // default "5m"
    SessionRetention string `yaml:"session_retention"` // default "7d"
}
```

Add `Daemon DaemonConfig` field to `Config`. Set defaults in `Defaults()`.

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "Add daemon config section with refresh interval and session retention"
```

---

### Task 8: CLI Subcommands

**Files:**
- Modify: `cmd/ccc/main.go` — Add `daemon`, `register`, `update-session`, `refresh` subcommands

- [ ] **Step 1: Add `daemon start|stop|status` subcommand**

Add to the subcommand switch in `main.go`:

- `daemon start` — forks a detached daemon process. Writes PID to `~/.config/ccc/daemon.pid`. Removes existing crontab entry via `config.UninstallSchedule()`. The daemon process calls `daemon.NewServer(...)` with a `RefreshFunc` that builds DataSources and calls `refresh.Run()` (same logic as ai-cron `main.go`).
- `daemon stop` — reads PID from file, sends SIGTERM.
- `daemon status` — checks if PID is alive, tries to connect to socket and Ping.

- [ ] **Step 2: Add `register` subcommand**

```
ccc register --session-id <id> --pid <pid> --project <dir> [--worktree-path <path>]
```

Connects to daemon via `daemon.NewClient()`, calls `RegisterSession`. If daemon not running, auto-starts it. If auto-start fails, falls back to direct `db.DBInsertSession()`.

- [ ] **Step 3: Add `update-session` subcommand**

```
ccc update-session --session-id <id> --topic <topic>
```

Connects to daemon, calls `UpdateSession`.

- [ ] **Step 4: Add `refresh` subcommand**

```
ccc refresh
```

Connects to daemon, calls `Refresh` RPC. Replaces running ai-cron manually.

- [ ] **Step 5: Test CLI commands manually**

```bash
go build -o /tmp/ccc ./cmd/ccc
/tmp/ccc daemon start
/tmp/ccc daemon status
/tmp/ccc register --session-id test-123 --pid $$ --project $(pwd)
/tmp/ccc update-session --session-id test-123 --topic "Testing CLI"
/tmp/ccc refresh
/tmp/ccc daemon stop
```

- [ ] **Step 6: Commit**

```bash
git add cmd/ccc/
git commit -m "Add daemon, register, update-session, and refresh CLI subcommands"
```

---

### Task 9: TUI Daemon Connection

**Files:**
- Modify: `internal/tui/model.go` — Connect to daemon, handle events, auto-start/reconnect

- [ ] **Step 1: Add daemon client to TUI Model**

Add `daemonClient *daemon.Client` field to Model. In `NewModel`, attempt to connect. If connection fails, auto-start daemon (run `ccc daemon start` as subprocess), retry connection. If still fails, set `daemonClient = nil` and log warning.

- [ ] **Step 2: Subscribe to daemon events**

After successful connection, start a goroutine that calls `daemonClient.Subscribe()`. Convert daemon events into `tea.Msg` types and send via `program.Send()`:

```go
type DaemonEventMsg struct {
    Event daemon.Event
}

type DaemonDisconnectedMsg struct{}
```

Route `DaemonEventMsg` through the existing event bus so plugins receive session events.

- [ ] **Step 3: Handle reconnection**

On `DaemonDisconnectedMsg`, attempt to restart daemon and reconnect. If restart fails, show flash message "Daemon disconnected — restart failed". Retry on a timer (every 10s).

- [ ] **Step 4: Test manually**

```bash
make build && make install
ccc  # should auto-start daemon
# In another terminal:
ccc register --session-id manual-test --pid $$ --project $(pwd)
# Session should appear in CCC after event push
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "Connect TUI to daemon with auto-start and event subscription"
```

---

### Task 10: Migration Cleanup + flock Removal Tracking

**Files:**
- Modify: `internal/lockfile/lockfile.go` — Add TODO comment
- Create: `internal/lockfile/lockfile_test.go` — Add skipped removal test
- Create: `examples/hooks/session-register.sh` — Example hook

- [ ] **Step 1: Add TODO and skipped test for flock removal**

In `internal/lockfile/lockfile.go`, add at the top:

```go
// TODO(daemon-stable): Remove flock once daemon is proven stable.
// The daemon is the sole refresh writer; flock exists only for backward
// compatibility during the ai-cron → daemon transition.
```

Add skipped test:

```go
// internal/lockfile/lockfile_test.go
func TestFlockRemovedAfterDaemonMigration(t *testing.T) {
    t.Skip("TODO(daemon-stable): Unskip this test when ready to remove flock. " +
        "If this test fails, it means flock references still exist and should be cleaned up.")
    // Grep for flock usage — should find nothing once removed
    // This is a reminder, not an automated check
}
```

- [ ] **Step 2: Create example session registration hook**

```bash
#!/bin/bash
# examples/hooks/session-register.sh
#
# Claude Code hook for automatic CCC session registration.
# Add to your Claude Code settings.json:
#
#   "hooks": {
#     "session_start": [{
#       "command": "/usr/local/bin/ccc register --session-id $SESSION_ID --pid $PPID --project $PWD"
#     }]
#   }
#
# For named sessions, add to your CLAUDE.md:
#
#   After understanding the session topic, register it with CCC:
#   ```bash
#   ccc update-session --session-id $SESSION_ID --topic "Your Topic Here"
#   ```

ccc register --session-id "$SESSION_ID" --pid "$PPID" --project "$PWD"
```

- [ ] **Step 3: Commit**

```bash
git add internal/lockfile/ examples/hooks/
git commit -m "Add flock removal tracking and example session registration hook"
```

---

## Phase 2: Sessions Plugin Upgrade

### Task 11: Active Sessions View

**Files:**
- Modify: `internal/builtin/sessions/sessions.go` — Add "Active Sessions" sub-tab
- Create: `internal/builtin/sessions/active.go` — Active sessions list view
- Create: `internal/builtin/sessions/active_test.go`

- [ ] **Step 1: Write failing test for active sessions view rendering**

Test that the view renders sessions grouped by status (running, ended), shows topic/project/age, and handles empty state.

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Implement active.go**

New sub-view within the sessions plugin. Uses `daemon.Client.ListSessions()` to fetch data (or reads from event-bus-pushed state). Renders as a list with status indicators:

- `●` (green) Running — PID alive
- `○` (gray) Ended — available for resume

Columns: status indicator, topic (or project/branch fallback), age (relative time), project path.

Grouped by project, sorted by recency.

- [ ] **Step 4: Wire into sessions plugin**

Add "Active" as a sub-tab alongside existing "New Session" and "Resume" views. Sessions plugin subscribes to `session.registered`, `session.updated`, `session.ended` events via the event bus to update the list live.

- [ ] **Step 5: Run test, verify pass**

- [ ] **Step 6: Test manually**

```bash
make build && make install
ccc  # navigate to Sessions tab → Active sub-tab
# In another terminal:
ccc register --session-id demo --pid $$ --project $(pwd)
ccc update-session --session-id demo --topic "Demo session"
# Should appear live in the Active Sessions view
```

- [ ] **Step 7: Commit**

```bash
git add internal/builtin/sessions/
git commit -m "Add Active Sessions view with live daemon updates"
```

---

### Task 12: Session Actions (Enter, Bookmark, Dismiss)

**Files:**
- Modify: `internal/builtin/sessions/active.go` — Key handlers

- [ ] **Step 1: Write failing test for key actions**

Test that Enter produces an `ActionLaunch` with the correct `claude --resume` command, `b` creates a bookmark, and `d` on an ended session removes it from the view. Note: `w` (open session viewer) is deferred to Phase 3 when agent events are available via daemon RPC.

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Implement key handlers**

```go
func (v *activeView) HandleKey(msg tea.KeyMsg) plugin.Action {
    switch msg.String() {
    case "enter":
        sess := v.selectedSession()
        if sess == nil {
            return plugin.NoopAction()
        }
        dir := sess.Project
        if sess.WorktreePath != "" {
            dir = sess.WorktreePath
        }
        return plugin.Action{
            Type: plugin.ActionLaunch,
            Payload: "claude",
            Args: map[string]string{
                "args": "--resume " + sess.SessionID,
                "dir":  dir,
            },
        }
    case "b":
        sess := v.selectedSession()
        if sess == nil {
            return plugin.NoopAction()
        }
        // Copy to cc_bookmarks — DBInsertBookmark takes a db.Session + label
        s := db.Session{
            SessionID:    sess.SessionID,
            Project:      sess.Project,
            Repo:         sess.Repo,
            Branch:       sess.Branch,
            WorktreePath: sess.WorktreePath,
        }
        db.DBInsertBookmark(v.db, s, sess.Topic)
        return plugin.Action{Type: plugin.ActionFlash, Payload: "Session bookmarked"}
    case "d":
        sess := v.selectedSession()
        if sess == nil || sess.State == "active" {
            return plugin.NoopAction() // can't dismiss live sessions
        }
        // Archive the session
        v.daemonClient.UpdateSession(daemon.UpdateSessionParams{
            SessionID: sess.SessionID,
        })
        // Or call a dedicated archive RPC
        return plugin.Action{Type: plugin.ActionFlash, Payload: "Session dismissed"}
    }
    return plugin.NoopAction()
}
```

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Test manually**

- [ ] **Step 6: Commit**

```bash
git add internal/builtin/sessions/
git commit -m "Add session actions: resume, bookmark, dismiss"
```

---

## Phase 3: Agent Runner Migration to Daemon

> **Prerequisite:** Rebase onto main to pick up commit `4360403` (Extract shared agent runner to `internal/agent/` package) before starting Phase 3. The `internal/agent/` package provides `Runner`, `Request`, `Status`, and related types that the daemon will host.

### Task 13: Agent Runner Daemon RPCs

**Files:**
- Modify: `internal/daemon/daemon.go` — Import `internal/agent/`, host runner
- Create: `internal/daemon/agents.go` — Agent RPC handlers
- Create: `internal/daemon/agents_test.go`

- [ ] **Step 1: Write failing test for LaunchAgent and ListAgents**

Test that `LaunchAgent` RPC queues an agent, `ListAgents` returns it, and `AgentStatus` shows its state. Use a mock command (e.g., `echo hello`) instead of real `claude`.

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Implement agents.go**

The daemon imports `internal/agent/` and creates a `Runner` instance. New RPC handlers:

- `LaunchAgent` → calls `runner.LaunchOrQueue()`
- `StopAgent` → calls `runner.Stop()`
- `AgentStatus` → calls `runner.Status()`
- `ListAgents` → calls `runner.Active()`
- `AgentEvents` → streams events from runner's event log
- `SendInput` → calls `runner.SendInput()`

The runner's callbacks (on status change, on blocked, on complete) broadcast events to subscribers.

- [ ] **Step 4: Add client methods**

```go
func (c *Client) LaunchAgent(req agent.Request) error
func (c *Client) StopAgent(id string) error
func (c *Client) AgentStatus(id string) (agent.Status, error)
func (c *Client) ListAgents() ([]agent.Status, error)
func (c *Client) SendInput(id string, message string) error
```

- [ ] **Step 5: Run test, verify pass**

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/
git commit -m "Add agent runner RPCs to daemon"
```

---

### Task 14: Command Center Plugin → Daemon RPC

**Files:**
- Modify: `internal/builtin/commandcenter/commandcenter.go` — Switch from direct agent calls to daemon RPC
- Modify: `internal/builtin/commandcenter/agent_runner.go` — Remove direct process management, replace with daemon client calls

- [ ] **Step 1: Write failing test**

Test that command center's `launchAgent` method calls the daemon client instead of spawning a process directly.

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Replace direct agent calls with daemon RPCs**

In the command center plugin, replace all `agent.Runner` calls with `daemon.Client` calls:

- `runner.LaunchOrQueue(req)` → `daemonClient.LaunchAgent(req)`
- `runner.Stop(id)` → `daemonClient.StopAgent(id)`
- `runner.Status(id)` → `daemonClient.AgentStatus(id)`
- `runner.Active()` → `daemonClient.ListAgents()`

The session viewer connects to `daemonClient.AgentEvents(id, since)` instead of reading from a local stdout pipe.

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Run full test suite**

Run: `make test`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/builtin/commandcenter/
git commit -m "Switch command center from direct agent calls to daemon RPCs"
```

---

### Task 15: PR Plugin + Automation Framework → Daemon RPC

**Files:**
- Modify: `internal/builtin/prs/` — Switch agent calls to daemon RPC
- Modify: `internal/automation/` — Run automations via daemon

- [ ] **Step 1: Switch PR plugin to daemon RPCs**

Same pattern as Task 14 — replace direct `agent.Runner` calls with `daemon.Client` calls.

- [ ] **Step 2: Migrate automation framework**

Automations run post-refresh. Since refresh is now in the daemon, automations should run there too. Move automation execution from `cmd/ai-cron/main.go` into the daemon's refresh callback.

- [ ] **Step 3: Run full test suite**

Run: `make test`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/builtin/prs/ internal/automation/
git commit -m "Migrate PR plugin and automation framework to daemon RPCs"
```

---

### Task 16: Blocked Status + Session Viewer Reconnection

**Files:**
- Modify: `internal/builtin/sessions/active.go` — Add blocked status indicator
- Modify: `internal/builtin/commandcenter/cc_view_session.go` — Connect via daemon events

- [ ] **Step 1: Add blocked status to active sessions view**

Now that the agent runner is in the daemon, blocked detection (stream-JSON `AskUser`/`SendUserMessage` events) is available via RPC. Add `● Blocked` (yellow) status indicator to the active sessions list for CCC-spawned agents that are waiting for input.

- [ ] **Step 2: Update session viewer to use daemon events**

Session viewer switches from reading local stdout pipe to `daemonClient.AgentEvents(id, since)`. This enables reconnection — if TUI restarts, it reconnects to the daemon and replays events from the last offset.

- [ ] **Step 3: Test manually**

```bash
make build && make install
ccc  # launch a todo agent that will block on AskUser
# Verify blocked indicator appears
# Quit CCC, relaunch — agent should still be running, viewer should reconnect
```

- [ ] **Step 4: Commit**

```bash
git add internal/builtin/sessions/ internal/builtin/commandcenter/
git commit -m "Add blocked status indicator and session viewer daemon reconnection"
```

---

### Task 17: CLAUDE.md Integration

**Files:**
- Modify: Files in `~/Personal/AI-RON/claude-rules/` — Add CCC update-session call

- [ ] **Step 1: Find the /set-topic invocation in AI-RON claude-rules**

Look in `/Users/aaron/Personal/AI-RON/claude-rules/` for the file that contains the `/set-topic` instruction.

- [ ] **Step 2: Add CCC update-session call right before it**

Add a line instructing Claude to call `ccc update-session --session-id $SESSION_ID --topic "..."` right before the `/set-topic` invocation. This ensures CCC gets the topic before the statusline does.

- [ ] **Step 3: Commit in AI-RON repo**

```bash
cd ~/Personal/AI-RON
git add claude-rules/
git commit -m "Add CCC session topic registration before /set-topic"
```

---

## Final Verification

- [ ] **Run full test suite:** `make test` — all green
- [ ] **Build:** `make build` — compiles cleanly
- [ ] **Manual end-to-end test:**
  1. `ccc daemon start` — daemon running
  2. Open CCC TUI — connects to daemon, shows Active Sessions
  3. Open new iTerm tab, start `claude` — hook registers session
  4. Set topic in claude session — appears in CCC
  5. Close claude session — CCC shows as ended
  6. Resume from CCC — opens in new tab
  7. Launch todo agent from CCC — runs in daemon, survives TUI restart
