package config

import (
	"testing"
)

func TestValidateCalendar_MissingCredentials(t *testing.T) {
	// Set HOME to a temp dir so credentials won't be found
	t.Setenv("HOME", t.TempDir())

	err := ValidateCalendar()
	if err == nil {
		t.Error("expected error when calendar credentials don't exist")
	}
}

func TestValidateGitHub_MissingCLI(t *testing.T) {
	// Override PATH so gh won't be found
	t.Setenv("PATH", t.TempDir())

	err := ValidateGitHub()
	if err == nil {
		t.Error("expected error when gh CLI is not available")
	}
}

func TestValidateGranola_MissingAccounts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := ValidateGranola()
	if err == nil {
		t.Error("expected error when Granola accounts don't exist")
	}
}
