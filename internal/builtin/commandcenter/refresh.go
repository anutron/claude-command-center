package commandcenter

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/lockfile"
)

// ccRefreshInterval is set during Init from config. Defaults to 5 minutes.
var ccRefreshInterval = 5 * time.Minute

// ccRefreshFinishedMsg is sent when the background refresh process completes.
type ccRefreshFinishedMsg struct {
	err error
}

// refreshCCCmd spawns the ccc-refresh binary to gather data from APIs.
// Skips if another refresh process already holds the lock.
func refreshCCCmd() tea.Cmd {
	return func() tea.Msg {
		stateDir := config.DataDir()
		if lockfile.IsLocked(stateDir) {
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
		if err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				err = fmt.Errorf("ccc-refresh failed: %s", errMsg)
			}
		}
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
