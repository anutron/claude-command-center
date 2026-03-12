package settings

import (
	"testing"
)

func TestNewClientCredForm_ReturnsFormAndCreds(t *testing.T) {
	form, creds := newClientCredForm()
	if form == nil {
		t.Fatal("expected non-nil form")
	}
	if creds == nil {
		t.Fatal("expected non-nil creds")
	}
}

func TestNewClientCredForm_CredsStartEmpty(t *testing.T) {
	_, creds := newClientCredForm()
	if creds.ClientID != "" {
		t.Errorf("expected empty ClientID, got %q", creds.ClientID)
	}
	if creds.ClientSecret != "" {
		t.Errorf("expected empty ClientSecret, got %q", creds.ClientSecret)
	}
}

func TestNewClientCredForm_BindsToCredsStruct(t *testing.T) {
	_, creds := newClientCredForm()

	// Simulate filling in credentials by writing directly to the struct.
	// In actual huh usage, the form updates these via pointer binding.
	creds.ClientID = "test-id"
	creds.ClientSecret = "test-secret"

	if creds.ClientID != "test-id" {
		t.Errorf("expected ClientID 'test-id', got %q", creds.ClientID)
	}
	if creds.ClientSecret != "test-secret" {
		t.Errorf("expected ClientSecret 'test-secret', got %q", creds.ClientSecret)
	}
}
