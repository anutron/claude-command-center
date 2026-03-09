package refresh

import (
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

// FreshData holds newly fetched data from all sources.
type FreshData struct {
	Calendar db.CalendarData
	Todos    []db.Todo
	Threads  []db.Thread
}

// RawMeeting is a meeting transcript from Granola (refresh-internal).
type RawMeeting struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Transcript string    `json:"transcript"`
	Summary    string    `json:"summary"`
	Attendees  []string  `json:"attendees"`
}

// slackCandidate is a Slack message that may contain a commitment (refresh-internal).
type slackCandidate struct {
	Message       string
	Permalink     string
	Channel       string
	ChannelID     string
	Timestamp     string
	ThreadContext string
}
