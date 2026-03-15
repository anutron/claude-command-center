package tui

import (
	"os"
	"path/filepath"
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
