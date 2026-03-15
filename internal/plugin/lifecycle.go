package plugin

// TabViewMsg is sent to a plugin when its tab becomes active.
// Route is the tab's route slug (e.g., "commandcenter").
type TabViewMsg struct{ Route string }

// TabLeaveMsg is sent to a plugin when its tab is being deactivated.
// Route is the route being left.
type TabLeaveMsg struct{ Route string }

// LaunchMsg is broadcast to all plugins before the TUI quits to launch Claude.
type LaunchMsg struct {
	Dir      string
	ResumeID string
}

// ReturnMsg is broadcast to all plugins when the TUI starts after returning
// from a Claude session.
type ReturnMsg struct {
	// TodoID is the todo that was being worked on, if any.
	TodoID string
	// WasResumeJoin is true if the session was a join/resume of an existing session.
	WasResumeJoin bool
}
