package db

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CommandCenter struct {
	GeneratedAt    time.Time       `json:"generated_at"`
	Calendar       CalendarData    `json:"calendar"`
	Todos          []Todo          `json:"todos"`
	Threads        []Thread        `json:"threads"`
	Suggestions    Suggestions     `json:"suggestions"`
	PendingActions []PendingAction `json:"pending_actions"`
	Warnings       []Warning       `json:"warnings,omitempty"`
}

type Warning struct {
	Source  string    `json:"source"`
	Message string   `json:"message"`
	At      time.Time `json:"at"`
}

type CalendarData struct {
	Today    []CalendarEvent `json:"today"`
	Tomorrow []CalendarEvent `json:"tomorrow"`
}

type CalendarConflict struct {
	EventA string
	EventB string
	Day    string // "today" or "tomorrow"
	Start  time.Time
	End    time.Time
}

// FindConflicts returns all pairs of overlapping events across today and tomorrow.
// Skips declined, all-day, and already-ended events to reduce noise.
func (cal *CalendarData) FindConflicts() []CalendarConflict {
	now := time.Now()
	var conflicts []CalendarConflict
	conflicts = append(conflicts, findOverlaps(cal.Today, "today", now)...)
	conflicts = append(conflicts, findOverlaps(cal.Tomorrow, "tomorrow", now)...)
	return conflicts
}

func findOverlaps(events []CalendarEvent, day string, now time.Time) []CalendarConflict {
	var real []CalendarEvent
	for _, ev := range events {
		if !ev.Declined && !ev.AllDay && ev.End.After(now) {
			real = append(real, ev)
		}
	}

	var conflicts []CalendarConflict
	for i := 0; i < len(real); i++ {
		for j := i + 1; j < len(real); j++ {
			a, b := real[i], real[j]
			if a.Start.Before(b.End) && b.Start.Before(a.End) {
				start := a.Start
				if b.Start.After(start) {
					start = b.Start
				}
				end := a.End
				if b.End.Before(end) {
					end = b.End
				}
				conflicts = append(conflicts, CalendarConflict{
					EventA: a.Title,
					EventB: b.Title,
					Day:    day,
					Start:  start,
					End:    end,
				})
			}
		}
	}
	return conflicts
}

type CalendarEvent struct {
	Title      string    `json:"title"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	AllDay     bool      `json:"all_day,omitempty"`
	Declined   bool      `json:"declined,omitempty"`
	CalendarID string    `json:"calendar_id,omitempty"`
}

type Todo struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	SourceRef   string     `json:"source_ref"`
	Context     string     `json:"context"`
	Detail      string     `json:"detail"`
	WhoWaiting  string     `json:"who_waiting"`
	ProjectDir  string     `json:"project_dir"`
	SessionID   string     `json:"session_id,omitempty"`
	Due         string     `json:"due"`
	Effort      string     `json:"effort"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type Thread struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	Repo        string     `json:"repo"`
	ProjectDir  string     `json:"project_dir"`
	Status      string     `json:"status"`
	Summary     string     `json:"summary"`
	CreatedAt   time.Time  `json:"created_at"`
	PausedAt    *time.Time `json:"paused_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type Suggestions struct {
	Focus         string            `json:"focus"`
	RankedTodoIDs []string          `json:"ranked_todo_ids"`
	Reasons       map[string]string `json:"reasons"`
}

type PendingAction struct {
	Type            string    `json:"type"`
	TodoID          string    `json:"todo_id"`
	DurationMinutes int       `json:"duration_minutes"`
	RequestedAt     time.Time `json:"requested_at"`
}

// SessionType distinguishes how a session should be resumed.
type SessionType int

const (
	SessionWinddown SessionType = iota
	SessionBookmark
)

// Session represents a resumable Claude Code session (bookmark or winddown).
type Session struct {
	Filename  string
	Project   string
	Repo      string
	Branch    string
	Created   time.Time
	Summary   string
	Type      SessionType
	SessionID string // Claude Code session UUID (bookmarks only)
}

// Bookmark is the JSON structure stored in bookmarks.json.
type Bookmark struct {
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Label     string `json:"label"`
	Summary   string `json:"summary"`
	Created   string `json:"created"`
}

// ---------------------------------------------------------------------------
// ID generation
// ---------------------------------------------------------------------------

func GenID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// ---------------------------------------------------------------------------
// Time helpers
// ---------------------------------------------------------------------------

func RelativeTime(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func DueUrgency(due string) string {
	if due == "" {
		return "none"
	}
	d, err := time.ParseInLocation("2006-01-02", due, time.Local)
	if err != nil {
		return "none"
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	dayAfter := today.AddDate(0, 0, 2)

	if d.Before(today) {
		return "overdue"
	}
	if d.Before(dayAfter) {
		return "soon"
	}
	return "later"
}

func FormatDueLabel(due string) string {
	if due == "" {
		return ""
	}
	d, err := time.Parse("2006-01-02", due)
	if err != nil {
		return ""
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	diff := int(d.Sub(today).Hours() / 24)
	switch {
	case diff < 0:
		return "overdue"
	case diff == 0:
		return "due today"
	case diff == 1:
		return "due tomorrow"
	default:
		return "due " + d.Format("Mon")
	}
}

// ---------------------------------------------------------------------------
// JSON file operations
// ---------------------------------------------------------------------------

func LoadCommandCenter(path string) (*CommandCenter, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cc CommandCenter
	if err := json.Unmarshal(data, &cc); err != nil {
		return nil, err
	}
	cc.EnforceDismissed()
	return &cc, nil
}

func SaveCommandCenter(path string, cc *CommandCenter) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// EnforceDismissed auto-dismisses any active todo whose source_ref matches
// a dismissed todo.
func (cc *CommandCenter) EnforceDismissed() {
	dismissedRefs := make(map[string]bool)
	for _, t := range cc.Todos {
		if t.Status == "dismissed" && t.SourceRef != "" {
			dismissedRefs[t.SourceRef] = true
		}
	}
	if len(dismissedRefs) == 0 {
		return
	}
	for i := range cc.Todos {
		if cc.Todos[i].Status == "active" && cc.Todos[i].SourceRef != "" {
			if dismissedRefs[cc.Todos[i].SourceRef] {
				cc.Todos[i].Status = "dismissed"
			}
		}
	}
}

// MutateSave reloads the JSON from disk, applies fn to the fresh copy, saves, and
// returns the fresh copy.
func MutateSave(path string, fallback *CommandCenter, fn func(*CommandCenter)) (*CommandCenter, error) {
	cc, err := LoadCommandCenter(path)
	if err != nil || cc == nil {
		cc = fallback
	}
	if cc == nil {
		cc = &CommandCenter{}
	}
	fn(cc)
	if err := SaveCommandCenter(path, cc); err != nil {
		return cc, err
	}
	return cc, nil
}

// ---------------------------------------------------------------------------
// CommandCenter mutation methods
// ---------------------------------------------------------------------------

func (cc *CommandCenter) CompleteTodo(id string) {
	now := time.Now()
	for i := range cc.Todos {
		if cc.Todos[i].ID == id {
			cc.Todos[i].Status = "completed"
			cc.Todos[i].CompletedAt = &now
			return
		}
	}
}

func (cc *CommandCenter) RestoreTodo(id, status string, completedAt *time.Time) {
	for i := range cc.Todos {
		if cc.Todos[i].ID == id {
			cc.Todos[i].Status = status
			cc.Todos[i].CompletedAt = completedAt
			return
		}
	}
}

func (cc *CommandCenter) AddTodo(title string) *Todo {
	t := Todo{
		ID:        GenID(),
		Title:     title,
		Status:    "active",
		Source:    "manual",
		CreatedAt: time.Now(),
	}
	cc.Todos = append(cc.Todos, t)
	return &cc.Todos[len(cc.Todos)-1]
}

func (cc *CommandCenter) RemoveTodo(id string) {
	for i := range cc.Todos {
		if cc.Todos[i].ID == id {
			cc.Todos[i].Status = "dismissed"
			return
		}
	}
}

func (cc *CommandCenter) DeferTodo(id string) {
	idx := -1
	for i := range cc.Todos {
		if cc.Todos[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	todo := cc.Todos[idx]
	cc.Todos = append(cc.Todos[:idx], cc.Todos[idx+1:]...)

	insertAt := len(cc.Todos)
	for i := range cc.Todos {
		if cc.Todos[i].Status != "active" {
			insertAt = i
			break
		}
	}

	cc.Todos = append(cc.Todos[:insertAt], append([]Todo{todo}, cc.Todos[insertAt:]...)...)
}

func (cc *CommandCenter) PromoteTodo(id string) {
	idx := -1
	for i := range cc.Todos {
		if cc.Todos[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	todo := cc.Todos[idx]
	cc.Todos = append(cc.Todos[:idx], cc.Todos[idx+1:]...)
	cc.Todos = append([]Todo{todo}, cc.Todos...)
}

func (cc *CommandCenter) PauseThread(id string) {
	now := time.Now()
	for i := range cc.Threads {
		if cc.Threads[i].ID == id {
			cc.Threads[i].Status = "paused"
			cc.Threads[i].PausedAt = &now
			return
		}
	}
}

func (cc *CommandCenter) StartThread(id string) {
	for i := range cc.Threads {
		if cc.Threads[i].ID == id {
			cc.Threads[i].Status = "active"
			cc.Threads[i].PausedAt = nil
			return
		}
	}
}

func (cc *CommandCenter) CloseThread(id string) {
	now := time.Now()
	for i := range cc.Threads {
		if cc.Threads[i].ID == id {
			cc.Threads[i].Status = "completed"
			cc.Threads[i].CompletedAt = &now
			return
		}
	}
}

func (cc *CommandCenter) AddThread(title, threadType string) *Thread {
	t := Thread{
		ID:        GenID(),
		Type:      threadType,
		Title:     title,
		Status:    "active",
		CreatedAt: time.Now(),
	}
	cc.Threads = append(cc.Threads, t)
	return &cc.Threads[len(cc.Threads)-1]
}

func (cc *CommandCenter) ActiveTodos() []Todo {
	var out []Todo
	for _, t := range cc.Todos {
		if t.Status == "active" {
			out = append(out, t)
		}
	}
	return out
}

func (cc *CommandCenter) CompletedTodos() []Todo {
	var out []Todo
	for _, t := range cc.Todos {
		if t.Status == "completed" {
			out = append(out, t)
		}
	}
	return out
}

func (cc *CommandCenter) ActiveThreads() []Thread {
	var out []Thread
	for _, t := range cc.Threads {
		if t.Status == "active" {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (cc *CommandCenter) PausedThreads() []Thread {
	var out []Thread
	for _, t := range cc.Threads {
		if t.Status == "paused" {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PausedAt == nil || out[j].PausedAt == nil {
			return false
		}
		return out[i].PausedAt.Before(*out[j].PausedAt)
	})
	return out
}

func (cc *CommandCenter) AddPendingBooking(todoID string, durationMinutes int) {
	cc.PendingActions = append(cc.PendingActions, PendingAction{
		Type:            "booking",
		TodoID:          todoID,
		DurationMinutes: durationMinutes,
		RequestedAt:     time.Now(),
	})
}

// ---------------------------------------------------------------------------
// Learned-paths CRUD (file-based)
// ---------------------------------------------------------------------------

func LoadPaths(file string) ([]string, error) {
	f, err := os.Open(file)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var paths []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			paths = append(paths, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if paths == nil {
		paths = []string{}
	}
	return paths, nil
}

func SavePaths(file string, paths []string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, p := range paths {
		if _, err := fmt.Fprintln(f, p); err != nil {
			return err
		}
	}
	return nil
}

func AddPath(paths []string, newPath string) []string {
	for _, p := range paths {
		if p == newPath {
			return paths
		}
	}
	return append(paths, newPath)
}

func RemovePath(paths []string, target string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != target {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Session parsing
// ---------------------------------------------------------------------------

func ParseSessionFile(path string) (Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}

	lines := strings.Split(string(data), "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Session{}, fmt.Errorf("no frontmatter in %s", path)
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return Session{}, fmt.Errorf("unclosed frontmatter in %s", path)
	}

	s := Session{Filename: filepath.Base(path)}
	for _, line := range lines[1:endIdx] {
		key, val, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "project":
			s.Project = val
		case "repo":
			s.Repo = val
		case "branch":
			s.Branch = val
		case "summary":
			s.Summary = val
		case "created":
			t, err := time.Parse("2006-01-02T15:04:05", val)
			if err != nil {
				t, err = time.Parse(time.RFC3339, val)
				if err != nil {
					return Session{}, fmt.Errorf("bad created time %q: %w", val, err)
				}
			}
			s.Created = t
		}
	}
	return s, nil
}

func LoadWinddownSessions(sessionsDir string) ([]Session, error) {
	entries, err := os.ReadDir(sessionsDir)
	if os.IsNotExist(err) {
		return []Session{}, nil
	}
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		s, err := ParseSessionFile(filepath.Join(sessionsDir, e.Name()))
		if err != nil {
			continue
		}
		s.Type = SessionWinddown
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []Session{}
	}
	return sessions, nil
}

func LoadBookmarks(bookmarksFile string) ([]Session, error) {
	data, err := os.ReadFile(bookmarksFile)
	if os.IsNotExist(err) {
		return []Session{}, nil
	}
	if err != nil {
		return nil, err
	}

	var bookmarks []Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return nil, fmt.Errorf("parse bookmarks: %w", err)
	}

	sessions := make([]Session, 0, len(bookmarks))
	for _, b := range bookmarks {
		t, _ := time.Parse(time.RFC3339, b.Created)
		sessions = append(sessions, Session{
			SessionID: b.SessionID,
			Project:   b.Project,
			Repo:      b.Repo,
			Branch:    b.Branch,
			Created:   t,
			Summary:   b.Summary,
			Type:      SessionBookmark,
		})
	}
	return sessions, nil
}

func LoadAllSessions(sessionsDir, bookmarksFile string) ([]Session, error) {
	winddowns, err := LoadWinddownSessions(sessionsDir)
	if err != nil {
		return nil, err
	}
	bookmarks, err := LoadBookmarks(bookmarksFile)
	if err != nil {
		bookmarks = []Session{}
	}

	all := append(winddowns, bookmarks...)
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].Created.After(all[i].Created) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	return all, nil
}

func RemoveBookmark(bookmarksFile, sessionID string) error {
	data, err := os.ReadFile(bookmarksFile)
	if err != nil {
		return err
	}

	var bookmarks []Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return err
	}

	filtered := make([]Bookmark, 0, len(bookmarks))
	for _, b := range bookmarks {
		if b.SessionID != sessionID {
			filtered = append(filtered, b)
		}
	}

	out, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bookmarksFile, out, 0o644)
}
