package auth

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGoogleTokenFile_RoundTrip(t *testing.T) {
	original := GoogleTokenFile{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		ExpiryDate:   1700000000000,
		Scope:        "https://www.googleapis.com/auth/calendar",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded GoogleTokenFile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestGoogleTokenFile_ToOAuth2Token(t *testing.T) {
	tf := &GoogleTokenFile{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		ExpiryDate:   1700000000000,
	}

	tok := tf.ToOAuth2Token()

	if tok.AccessToken != "access-123" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "access-123")
	}
	if tok.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken = %q, want %q", tok.RefreshToken, "refresh-456")
	}
	if tok.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", tok.TokenType, "Bearer")
	}

	expected := time.UnixMilli(1700000000000)
	if !tok.Expiry.Equal(expected) {
		t.Errorf("Expiry = %v, want %v", tok.Expiry, expected)
	}
}

func TestGoogleTokenFile_ToOAuth2Token_ZeroExpiry(t *testing.T) {
	tf := &GoogleTokenFile{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
	}

	tok := tf.ToOAuth2Token()

	if !tok.Expiry.IsZero() {
		t.Errorf("Expiry should be zero when ExpiryDate is 0, got %v", tok.Expiry)
	}
}

func TestGoogleTokenFile_OmitsEmptyFields(t *testing.T) {
	tf := GoogleTokenFile{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
	}

	data, err := json.Marshal(tf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	if contains(s, "clientId") {
		t.Error("expected clientId to be omitted when empty")
	}
	if contains(s, "clientSecret") {
		t.Error("expected clientSecret to be omitted when empty")
	}
	if contains(s, "expiry_date") {
		t.Error("expected expiry_date to be omitted when zero")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
