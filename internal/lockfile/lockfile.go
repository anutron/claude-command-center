package lockfile

// TODO(daemon-stable): Remove flock once daemon is proven stable.
// The daemon is the sole refresh writer; flock exists only for backward
// compatibility during the ai-cron → daemon transition.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const lockFileName = "refresh.lock"

// ErrAlreadyLocked is returned when another process holds the lock.
var ErrAlreadyLocked = errors.New("refresh already running")

// AcquireLock attempts to acquire the refresh lock using flock for atomic
// advisory locking. Returns a release function on success. If another process
// holds the lock, returns ErrAlreadyLocked (wrapped with PID info if available).
func AcquireLock(stateDir string) (release func(), err error) {
	lockPath := filepath.Join(stateDir, lockFileName)

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating state dir: %w", err)
	}

	// Open/create the lock file
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	// Try non-blocking exclusive flock
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Read existing PID for informational error message
		var pidInfo string
		if data, readErr := os.ReadFile(lockPath); readErr == nil {
			pidStr := strings.TrimSpace(string(data))
			if _, parseErr := strconv.Atoi(pidStr); parseErr == nil {
				pidInfo = " (pid " + pidStr + ")"
			}
		}
		f.Close()
		return nil, fmt.Errorf("%w%s", ErrAlreadyLocked, pidInfo)
	}

	// Flock acquired — write our PID for informational purposes
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	fmt.Fprint(f, os.Getpid())
	_ = f.Sync()

	release = func() {
		// Closing the fd releases the flock automatically
		f.Close()
		os.Remove(lockPath)
	}
	return release, nil
}

// IsLocked returns true if the refresh lock is currently held.
func IsLocked(stateDir string) bool {
	lockPath := filepath.Join(stateDir, lockFileName)

	f, err := os.OpenFile(lockPath, os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer f.Close()

	// Try to acquire — if we can't, someone else holds it
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return true // Lock is held by another process
	}

	// We got the lock, so it wasn't held — release immediately
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}
