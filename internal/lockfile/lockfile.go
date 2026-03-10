package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const lockFileName = "refresh.lock"

// AcquireLock attempts to acquire the refresh lock. Returns a release function
// on success. If another process holds the lock, returns an error.
func AcquireLock(stateDir string) (release func(), err error) {
	lockPath := filepath.Join(stateDir, lockFileName)

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating state dir: %w", err)
	}

	// Check for existing lock
	if data, err := os.ReadFile(lockPath); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			if isProcessAlive(pid) {
				return nil, fmt.Errorf("refresh already running (pid %d)", pid)
			}
			// Stale lock — process is dead, we can take over
		}
	}

	// Write our PID
	pid := os.Getpid()
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return nil, fmt.Errorf("writing lock file: %w", err)
	}

	release = func() {
		os.Remove(lockPath)
	}
	return release, nil
}

// IsLocked returns true if the refresh lock is held by a live process.
func IsLocked(stateDir string) bool {
	lockPath := filepath.Join(stateDir, lockFileName)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}
	return isProcessAlive(pid)
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
