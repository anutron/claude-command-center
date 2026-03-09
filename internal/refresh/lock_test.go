package refresh

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAcquireAndRelease(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	// Lock file should exist with our PID
	data, err := os.ReadFile(filepath.Join(dir, lockFileName))
	if err != nil {
		t.Fatalf("reading lock file: %v", err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("parsing pid: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("expected pid %d, got %d", os.Getpid(), pid)
	}

	// Should be locked
	if !IsLocked(dir) {
		t.Error("expected IsLocked to return true")
	}

	// Release
	release()

	// Should no longer be locked
	if IsLocked(dir) {
		t.Error("expected IsLocked to return false after release")
	}
}

func TestAcquireWhileLocked(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("first AcquireLock failed: %v", err)
	}
	defer release()

	// Second acquire should fail
	_, err = AcquireLock(dir)
	if err == nil {
		t.Error("expected second AcquireLock to fail")
	}
}

func TestAcquireStaleLock(t *testing.T) {
	dir := t.TempDir()

	// Write a lock file with a non-existent PID
	lockPath := filepath.Join(dir, lockFileName)
	// PID 99999999 is almost certainly not running
	os.WriteFile(lockPath, []byte("99999999"), 0o644)

	// IsLocked should return false for stale lock
	if IsLocked(dir) {
		t.Skip("PID 99999999 is somehow alive on this system")
	}

	// Should be able to acquire over stale lock
	release, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock over stale lock failed: %v", err)
	}
	release()
}

func TestIsLockedNoFile(t *testing.T) {
	dir := t.TempDir()
	if IsLocked(dir) {
		t.Error("expected IsLocked to return false when no lock file exists")
	}
}
