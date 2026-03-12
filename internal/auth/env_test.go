package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func setupEnvTest(t *testing.T, content string) (cleanup func()) {
	t.Helper()

	tmpDir := t.TempDir()
	envDir := filepath.Join(tmpDir, ".config", "ccc")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}

	envPath := filepath.Join(envDir, ".env")
	if content != "" {
		if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Override HOME so envFilePath() resolves to our temp dir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)

	return func() {
		os.Setenv("HOME", origHome)
	}
}

func TestReadEnv(t *testing.T) {
	cleanup := setupEnvTest(t, `
# A comment
FOO=bar
BAZ="quoted"
SINGLE='single_quoted'
EMPTY=
SPACED = value with spaces
`)
	defer cleanup()

	tests := []struct {
		key  string
		want string
	}{
		{"FOO", "bar"},
		{"BAZ", "quoted"},
		{"SINGLE", "single_quoted"},
		{"EMPTY", ""},
		{"SPACED", "value with spaces"},
		{"MISSING", ""},
	}

	for _, tt := range tests {
		got := ReadEnv(tt.key)
		if got != tt.want {
			t.Errorf("ReadEnv(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestWriteEnvValue_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	if err := WriteEnvValue("NEW_KEY", "new_value"); err != nil {
		t.Fatalf("WriteEnvValue: %v", err)
	}

	got := ReadEnv("NEW_KEY")
	if got != "new_value" {
		t.Errorf("ReadEnv after write = %q, want %q", got, "new_value")
	}
}

func TestWriteEnvValue_UpdateExisting(t *testing.T) {
	cleanup := setupEnvTest(t, "FOO=old\nBAR=keep\n")
	defer cleanup()

	if err := WriteEnvValue("FOO", "new"); err != nil {
		t.Fatalf("WriteEnvValue: %v", err)
	}

	if got := ReadEnv("FOO"); got != "new" {
		t.Errorf("FOO = %q, want %q", got, "new")
	}
	if got := ReadEnv("BAR"); got != "keep" {
		t.Errorf("BAR = %q, want %q", got, "keep")
	}
}

func TestWriteEnvValue_Append(t *testing.T) {
	cleanup := setupEnvTest(t, "EXISTING=yes\n")
	defer cleanup()

	if err := WriteEnvValue("ADDED", "appended"); err != nil {
		t.Fatalf("WriteEnvValue: %v", err)
	}

	if got := ReadEnv("EXISTING"); got != "yes" {
		t.Errorf("EXISTING = %q, want %q", got, "yes")
	}
	if got := ReadEnv("ADDED"); got != "appended" {
		t.Errorf("ADDED = %q, want %q", got, "appended")
	}
}

func TestLoadEnvFile_DoesNotOverwrite(t *testing.T) {
	cleanup := setupEnvTest(t, "TEST_LOAD_KEY=from_file\n")
	defer cleanup()

	os.Setenv("TEST_LOAD_KEY", "existing")
	defer os.Unsetenv("TEST_LOAD_KEY")

	LoadEnvFile()

	if got := os.Getenv("TEST_LOAD_KEY"); got != "existing" {
		t.Errorf("LoadEnvFile overwrote existing env: got %q, want %q", got, "existing")
	}
}

func TestLoadEnvFile_SetsUnset(t *testing.T) {
	cleanup := setupEnvTest(t, "TEST_LOAD_UNSET=from_file\n")
	defer cleanup()

	os.Unsetenv("TEST_LOAD_UNSET")

	LoadEnvFile()
	defer os.Unsetenv("TEST_LOAD_UNSET")

	if got := os.Getenv("TEST_LOAD_UNSET"); got != "from_file" {
		t.Errorf("LoadEnvFile didn't set unset var: got %q, want %q", got, "from_file")
	}
}
