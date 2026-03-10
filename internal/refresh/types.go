package refresh

import (
	"github.com/anutron/claude-command-center/internal/db"
)

// FreshData holds newly fetched data from all sources.
type FreshData struct {
	Calendar db.CalendarData
	Todos    []db.Todo
	Threads  []db.Thread
}
