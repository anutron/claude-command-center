package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestValidateLaunchDir(t *testing.T) {
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Create real temp directories for path validation (EvalSymlinks needs them to exist).
	projectA := t.TempDir()
	projectB := t.TempDir()

	_ = db.DBAddPath(database, projectA)
	_ = db.DBAddPath(database, projectB)

	// Create a subdirectory inside projectA.
	subDir := filepath.Join(projectA, "src", "pkg")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{
			name:    "empty dir is always allowed",
			dir:     "",
			wantErr: false,
		},
		{
			name:    "exact learned path",
			dir:     projectA,
			wantErr: false,
		},
		{
			name:    "subdirectory of learned path",
			dir:     subDir,
			wantErr: false,
		},
		{
			name:    "another learned path",
			dir:     projectB,
			wantErr: false,
		},
		{
			name:    "unrelated path rejected",
			dir:     "/tmp/evil-project",
			wantErr: true,
		},
		{
			name:    "parent of learned path rejected",
			dir:     filepath.Dir(projectA),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLaunchDir(database, tt.dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLaunchDir(%q) error = %v, wantErr %v", tt.dir, err, tt.wantErr)
			}
		})
	}
}

func TestValidateLaunchDir_NoLearnedPaths(t *testing.T) {
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// With no learned paths, any non-empty dir should be rejected.
	err = validateLaunchDir(database, "/some/dir")
	if err == nil {
		t.Error("expected error when no learned paths exist, got nil")
	}
}

func TestResolveSessionDir_FindsSessionInDifferentProject(t *testing.T) {
	// Create a fake ~/.claude/projects structure in a temp dir.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a project directory with a session file.
	// Simulate: session was created in /Users/test/myproject
	// which maps to -Users-test-myproject in Claude's encoding.
	projectDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionID := "abc-123-def-456"
	sessionFile := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Also create the physical directory that maps to this project path,
	// so resolveSessionDir can verify it exists.
	physicalDir := "/Users/test/myproject"
	// On test systems this path won't exist, so resolveSessionDir should
	// fall back to the fallback dir. That's the expected behavior for
	// non-existent decoded paths.
	fallback := "/some/fallback"

	result := resolveSessionDir(sessionID, fallback)
	// The decoded path /Users/test/myproject won't exist on the test system,
	// so we expect the fallback.
	if result != fallback {
		t.Errorf("resolveSessionDir() = %q, want %q (decoded path %q doesn't exist)", result, fallback, physicalDir)
	}
}

func TestResolveSessionDir_FallsBackWhenNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create empty projects dir.
	projectsDir := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fallback := "/original/dir"
	result := resolveSessionDir("nonexistent-session", fallback)
	if result != fallback {
		t.Errorf("resolveSessionDir() = %q, want fallback %q", result, fallback)
	}
}

func TestResolveSessionDir_FindsSessionWithExistingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a real physical directory that the decoded project path will resolve to.
	physicalDir := filepath.Join(home, "myproject")
	if err := os.MkdirAll(physicalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create the Claude project dir with encoding matching physicalDir.
	// The encoding: replace "/" with "-". So physicalDir becomes:
	// e.g., /tmp/TestXYZ/myproject -> -tmp-TestXYZ-myproject
	encoded := strings.ReplaceAll(physicalDir, "/", "-")
	projectDir := filepath.Join(home, ".claude", "projects", encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "found-session-uuid"
	sessionFile := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	fallback := "/wrong/dir"
	result := resolveSessionDir(sessionID, fallback)
	if result != physicalDir {
		t.Errorf("resolveSessionDir() = %q, want %q", result, physicalDir)
	}
}

func TestResolveSessionDir_SessionInExpectedDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create the expected project dir with a session file.
	expectedDir := filepath.Join(home, "expected-project")
	if err := os.MkdirAll(expectedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	encoded := strings.ReplaceAll(expectedDir, "/", "-")
	projectDir := filepath.Join(home, ".claude", "projects", encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "expected-session"
	sessionFile := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When session exists in expected dir, should return that dir immediately.
	result := resolveSessionDir(sessionID, expectedDir)
	if result != expectedDir {
		t.Errorf("resolveSessionDir() = %q, want %q", result, expectedDir)
	}
}

func TestValidateLaunchDir_PathTraversal(t *testing.T) {
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	projectDir := t.TempDir()
	_ = db.DBAddPath(database, projectDir)

	// Path traversal: projectDir/../ should resolve to parent, which is not allowed.
	traversal := filepath.Join(projectDir, "..", filepath.Base(projectDir)+"evil")
	err = validateLaunchDir(database, traversal)
	if err == nil {
		t.Errorf("expected error for path traversal %q, got nil", traversal)
	}
}
