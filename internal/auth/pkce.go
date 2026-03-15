package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"golang.org/x/oauth2"
)

// PKCEParams holds the PKCE code verifier and challenge for an OAuth2 flow.
type PKCEParams struct {
	CodeVerifier  string
	CodeChallenge string
}

// GeneratePKCE creates a new PKCE code verifier (32 random bytes, base64url-encoded)
// and its corresponding S256 code challenge per RFC 7636.
func GeneratePKCE() (PKCEParams, error) {
	// 32 bytes of cryptographic randomness → 43-char base64url string
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return PKCEParams{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// S256: code_challenge = BASE64URL(SHA256(code_verifier))
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	return PKCEParams{
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
	}, nil
}

// AuthURLParams returns the oauth2.AuthCodeOption values to add the PKCE
// challenge to an authorization URL.
func (p PKCEParams) AuthURLParams() []oauth2.AuthCodeOption {
	return []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("code_challenge", p.CodeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	}
}

// ExchangeParams returns the oauth2.AuthCodeOption to add the PKCE verifier
// to a token exchange request.
func (p PKCEParams) ExchangeParams() []oauth2.AuthCodeOption {
	return []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("code_verifier", p.CodeVerifier),
	}
}
