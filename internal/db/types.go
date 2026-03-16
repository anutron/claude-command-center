package db

import (
	"bufio"
	"crypto/rand"
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
	DisplayID   int        `json:"display_id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	SourceRef   string     `json:"source_ref"`
	Context     string     `json:"context"`
	Detail      string     `json:"detail"`
	WhoWaiting  string     `json:"who_waiting"`
	ProjectDir  string     `json:"project_dir"`
	LaunchMode  string     `json:"launch_mode,omitempty"`
	SessionID      string     `json:"session_id,omitempty"`
	Due            string     `json:"due"`
	Effort         string     `json:"effort"`
	ProposedPrompt string     `json:"proposed_prompt,omitempty"`
	SessionStatus  string     `json:"session_status,omitempty"`
	SessionSummary string     `json:"session_summary,omitempty"`
	SourceContext    string     `json:"source_context,omitempty"`
	SourceContextAt  string     `json:"source_context_at,omitempty"`
	TriageStatus   string     `json:"triage_status,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at"`
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

// SourceSync tracks the last sync status for a data source.
type SourceSync struct {
	Source      string     `json:"source"`
	LastSuccess *time.Time `json:"last_success,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
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

func (cc *CommandCenter) AcceptTodo(id string) {
	for i := range cc.Todos {
		if cc.Todos[i].ID == id {
			cc.Todos[i].TriageStatus = "accepted"
			return
		}
	}
}

func (cc *CommandCenter) AddTodo(title string) *Todo {
	// Compute next display_id from in-memory todos so the detail view
	// shows the correct ID before the next DB reload.
	maxDisplayID := 0
	for _, existing := range cc.Todos {
		if existing.DisplayID > maxDisplayID {
			maxDisplayID = existing.DisplayID
		}
	}
	t := Todo{
		ID:           GenID(),
		DisplayID:    maxDisplayID + 1,
		Title:        title,
		Status:       "active",
		Source:       "manual",
		TriageStatus: "accepted",
		CreatedAt:    time.Now(),
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

// SwapTodos swaps two todos by their slice indices.
func (cc *CommandCenter) SwapTodos(i, j int) {
	if i < 0 || j < 0 || i >= len(cc.Todos) || j >= len(cc.Todos) {
		return
	}
	cc.Todos[i], cc.Todos[j] = cc.Todos[j], cc.Todos[i]
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

// PathEntry holds full metadata for a learned path row.
type PathEntry struct {
	Path        string    `json:"path"`
	Description string    `json:"description"`
	AddedAt     time.Time `json:"added_at"`
	SortOrder   int       `json:"sort_order"`
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

