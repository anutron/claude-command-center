package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsShellHookInstalled_NoZshrc(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if IsShellHookInstalled() {
		t.Error("expected false when .zshrc does not exist")
	}
}

func TestIsShellHookInstalled_ZshrcWithoutHook(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	zshrc := filepath.Join(tmpHome, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("export PATH=$PATH:/usr/local/bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsShellHookInstalled() {
		t.Error("expected false when .zshrc exists but has no CCC hook")
	}
}

func TestIsShellHookInstalled_ZshrcWithHook(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	zshrc := filepath.Join(tmpHome, ".zshrc")
	content := "export PATH=$PATH\n" + shellHookSnippet + "\n"
	if err := os.WriteFile(zshrc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if !IsShellHookInstalled() {
		t.Error("expected true when .zshrc contains CCC hook")
	}
}

func TestInstallShellHook(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create empty .zshrc
	zshrc := filepath.Join(tmpHome, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("# existing content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InstallShellHook(); err != nil {
		t.Fatalf("InstallShellHook failed: %v", err)
	}

	if !IsShellHookInstalled() {
		t.Error("expected hook to be installed after InstallShellHook")
	}

	// Idempotent: installing again should not error
	if err := InstallShellHook(); err != nil {
		t.Fatalf("second InstallShellHook failed: %v", err)
	}
}

func TestUninstallShellHook(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	zshrc := filepath.Join(tmpHome, ".zshrc")
	content := "# before\n" + shellHookSnippet + "\n# after\n"
	if err := os.WriteFile(zshrc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UninstallShellHook(); err != nil {
		t.Fatalf("UninstallShellHook failed: %v", err)
	}

	if IsShellHookInstalled() {
		t.Error("expected hook to be removed after UninstallShellHook")
	}
}

func TestUninstallShellHook_NoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Should not error if .zshrc doesn't exist
	if err := UninstallShellHook(); err != nil {
		t.Fatalf("UninstallShellHook should not error for missing file: %v", err)
	}
}
