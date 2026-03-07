package refresh

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TODO: import from internal/db when available
// These are local copies with the same field names and JSON tags.

type CommandCenter struct {
	GeneratedAt    time.Time       `json:"generated_at"`
	Calendar       CalendarData    `json:"calendar"`
	Todos          []Todo          `json:"todos"`
	Threads        []Thread        `json:"threads"`
	Suggestions    Suggestions     `json:"suggestions"`
	PendingActions []PendingAction `json:"pending_actions"`
	Warnings       []Warning       `json:"warnings,omitempty"`
}

type CalendarData struct {
	Today    []CalendarEvent `json:"today"`
	Tomorrow []CalendarEvent `json:"tomorrow"`
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

type Warning struct {
	Source  string    `json:"source"`
	Message string   `json:"message"`
	At      time.Time `json:"at"`
}

// FreshData holds newly fetched data from all sources.
type FreshData struct {
	Calendar CalendarData
	Todos    []Todo
	Threads  []Thread
}

type RawMeeting struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Transcript string    `json:"transcript"`
	Summary    string    `json:"summary"`
	Attendees  []string  `json:"attendees"`
}

type slackCandidate struct {
	Message       string
	Permalink     string
	Channel       string
	ChannelID     string
	Timestamp     string
	ThreadContext string
}

func genID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

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
