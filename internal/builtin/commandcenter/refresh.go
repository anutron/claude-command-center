package commandcenter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/refresh"
)

const ccRefreshInterval = 5 * time.Minute

// ccRefreshFinishedMsg is sent when the background refresh process completes.
type ccRefreshFinishedMsg struct {
	err error
}

// refreshCCCmd spawns the ccc-refresh binary to gather data from APIs.
// Skips if another refresh process already holds the lock.
func refreshCCCmd() tea.Cmd {
	return func() tea.Msg {
		stateDir := config.DataDir()
		if refresh.IsLocked(stateDir) {
			return ccRefreshFinishedMsg{err: nil}
		}

		binary := findRefreshBinary()

		var cmd *exec.Cmd
		if binary != "" {
			cmd = exec.Command(binary)
		} else {
			cmd = exec.Command("ccc-refresh")
		}

		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err := cmd.Run()
		return ccRefreshFinishedMsg{err: err}
	}
}

// findRefreshBinary looks for ccc-refresh next to the current executable.
func findRefreshBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	candidate := filepath.Join(dir, "ccc-refresh")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}
