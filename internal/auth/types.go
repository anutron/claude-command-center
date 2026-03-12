package auth

import (
	"time"

	"golang.org/x/oauth2"
)

// GoogleTokenFile represents a stored OAuth2 token file used by Google APIs.
type GoogleTokenFile struct {
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiryDate   int64  `json:"expiry_date,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// ToOAuth2Token converts the stored token file to an oauth2.Token.
func (g *GoogleTokenFile) ToOAuth2Token() *oauth2.Token {
	tok := &oauth2.Token{
		AccessToken:  g.AccessToken,
		RefreshToken: g.RefreshToken,
		TokenType:    g.TokenType,
	}
	if g.ExpiryDate > 0 {
		tok.Expiry = time.UnixMilli(g.ExpiryDate)
	}
	return tok
}
