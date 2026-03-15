package lockfile

import (
	"errors"
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

	// Second acquire should fail with ErrAlreadyLocked
	_, err = AcquireLock(dir)
	if err == nil {
		t.Fatal("expected second AcquireLock to fail")
	}
	if !errors.Is(err, ErrAlreadyLocked) {
		t.Errorf("expected ErrAlreadyLocked, got: %v", err)
	}
}

func TestAcquireStaleLockFile(t *testing.T) {
	dir := t.TempDir()

	// Write a lock file with a non-existent PID (simulating a crashed process).
	// With flock, the OS releases the lock when the process dies, so the file
	// content is stale but the flock is not held — acquisition should succeed.
	lockPath := filepath.Join(dir, lockFileName)
	os.WriteFile(lockPath, []byte("99999999"), 0o644)

	// Should be able to acquire — no flock is held
	release, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("AcquireLock over stale lock file failed: %v", err)
	}
	release()
}

func TestIsLockedNoFile(t *testing.T) {
	dir := t.TempDir()
	if IsLocked(dir) {
		t.Error("expected IsLocked to return false when no lock file exists")
	}
}
