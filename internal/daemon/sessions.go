package daemon

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

// sessionRegistry manages in-memory session state backed by SQLite.
type sessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]*SessionInfo
	database *sql.DB
}

// newSessionRegistry creates a registry and loads existing sessions from the DB.
func newSessionRegistry(database *sql.DB) *sessionRegistry {
	r := &sessionRegistry{
		sessions: make(map[string]*SessionInfo),
		database: database,
	}
	r.loadFromDB()
	return r
}

// loadFromDB populates the in-memory map from cc_sessions (non-archived only).
func (r *sessionRegistry) loadFromDB() {
	records, err := db.DBLoadVisibleSessions(r.database)
	if err != nil {
		return
	}
	for _, rec := range records {
		r.sessions[rec.SessionID] = recordToInfo(rec)
	}
}

// register adds a new session to the registry and persists it to the DB.
func (r *sessionRegistry) register(params RegisterSessionParams) error {
	// Resolve git info outside the lock — exec.Command can be slow.
	repo, branch := gitInfoFromDir(params.Project)

	r.mu.Lock()
	defer r.mu.Unlock()

	now := db.FormatTime(time.Now())

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

	rec := infoToRecord(info)
	if err := db.DBInsertSession(r.database, rec); err != nil {
		return fmt.Errorf("persist session: %w", err)
	}
	return nil
}

// update modifies mutable fields on a session and persists to DB.
func (r *sessionRegistry) update(params UpdateSessionParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.sessions[params.SessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", params.SessionID)
	}

	fields := make(map[string]interface{})
	if params.Topic != "" {
		info.Topic = params.Topic
		fields["topic"] = params.Topic
	}

	if len(fields) == 0 {
		return nil
	}
	return db.DBUpdateSession(r.database, params.SessionID, fields)
}

// end marks a session as "ended".
func (r *sessionRegistry) end(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	if info.State != "active" {
		return nil // already ended or archived — no-op
	}
	info.State = "ended"
	info.EndedAt = db.FormatTime(time.Now())
	return db.DBUpdateSessionState(r.database, sessionID, "ended")
}

// archive marks a session as "archived", removing it from the active list.
func (r *sessionRegistry) archive(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	if info.State == "active" {
		return fmt.Errorf("cannot archive active session: %s", sessionID)
	}
	info.State = "archived"
	return db.DBUpdateSessionState(r.database, sessionID, "archived")
}

// list returns all non-archived sessions.
func (r *sessionRegistry) list() []SessionInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []SessionInfo
	for _, info := range r.sessions {
		if info.State != "archived" {
			out = append(out, *info)
		}
	}
	return out
}

// sessionIDForPID returns the CCC session ID for a given PID, or "" if not found.
// Only checks active sessions.
func (r *sessionRegistry) sessionIDForPID(pid int) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, info := range r.sessions {
		if info.PID == pid && info.State == "active" {
			return info.SessionID
		}
	}
	return ""
}

// pruneDead checks each active session's PID and marks dead ones as "ended".
func (r *sessionRegistry) pruneDead() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, info := range r.sessions {
		if info.State != "active" {
			continue
		}
		if !isProcessAlive(info.PID) {
			info.State = "ended"
			info.EndedAt = db.FormatTime(time.Now())
			_ = db.DBUpdateSessionState(r.database, info.SessionID, "ended")
		}
	}
}

// isProcessAlive checks whether a process with the given PID is running.
// Uses os.FindProcess + signal 0, which works on Unix systems.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// gitInfoFromDir extracts the git remote URL and branch from a directory.
// Returns empty strings on any error (best-effort).
func gitInfoFromDir(dir string) (repo, branch string) {
	if dir == "" {
		return "", ""
	}

	// Get remote URL
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err == nil {
		repo = strings.TrimSpace(string(out))
	}

	// Get current branch
	out, err = exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err == nil {
		branch = strings.TrimSpace(string(out))
	}

	return repo, branch
}

// recordToInfo converts a db.SessionRecord to a SessionInfo.
func recordToInfo(rec db.SessionRecord) *SessionInfo {
	return &SessionInfo{
		SessionID:    rec.SessionID,
		Topic:        rec.Topic,
		PID:          rec.PID,
		Project:      rec.Project,
		Repo:         rec.Repo,
		Branch:       rec.Branch,
		WorktreePath: rec.WorktreePath,
		State:        rec.State,
		RegisteredAt: rec.RegisteredAt,
		EndedAt:      rec.EndedAt,
	}
}

// infoToRecord converts a SessionInfo to a db.SessionRecord.
func infoToRecord(info *SessionInfo) db.SessionRecord {
	return db.SessionRecord{
		SessionID:    info.SessionID,
		Topic:        info.Topic,
		PID:          info.PID,
		Project:      info.Project,
		Repo:         info.Repo,
		Branch:       info.Branch,
		WorktreePath: info.WorktreePath,
		State:        info.State,
		RegisteredAt: info.RegisteredAt,
		EndedAt:      info.EndedAt,
	}
}
