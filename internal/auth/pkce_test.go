package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	p, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE: %v", err)
	}

	// Verifier should be base64url-encoded 32 bytes → 43 chars.
	if len(p.CodeVerifier) != 43 {
		t.Errorf("code_verifier length = %d, want 43", len(p.CodeVerifier))
	}

	// Challenge should be SHA256(verifier) base64url-encoded → 43 chars.
	if len(p.CodeChallenge) != 43 {
		t.Errorf("code_challenge length = %d, want 43", len(p.CodeChallenge))
	}

	// Verify the challenge matches the verifier.
	h := sha256.Sum256([]byte(p.CodeVerifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])
	if p.CodeChallenge != expected {
		t.Errorf("code_challenge mismatch:\n  got:  %s\n  want: %s", p.CodeChallenge, expected)
	}

	// No padding characters.
	for _, c := range p.CodeVerifier + p.CodeChallenge {
		if c == '=' {
			t.Error("found padding character '=' in PKCE values")
		}
	}
}

func TestGeneratePKCE_Uniqueness(t *testing.T) {
	p1, _ := GeneratePKCE()
	p2, _ := GeneratePKCE()
	if p1.CodeVerifier == p2.CodeVerifier {
		t.Error("two calls to GeneratePKCE returned the same verifier")
	}
}

func TestPKCEParams_AuthURLParams(t *testing.T) {
	p := PKCEParams{CodeChallenge: "test-challenge"}
	opts := p.AuthURLParams()
	if len(opts) != 2 {
		t.Fatalf("AuthURLParams returned %d options, want 2", len(opts))
	}
}

func TestPKCEParams_ExchangeParams(t *testing.T) {
	p := PKCEParams{CodeVerifier: "test-verifier"}
	opts := p.ExchangeParams()
	if len(opts) != 1 {
		t.Fatalf("ExchangeParams returned %d options, want 1", len(opts))
	}
}
