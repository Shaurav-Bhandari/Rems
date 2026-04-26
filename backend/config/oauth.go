package config

import (
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// OAuthConfig holds Google OAuth2 configuration.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Enabled      bool
}

// LoadOAuthConfig loads Google OAuth configuration from environment variables.
func LoadOAuthConfig() OAuthConfig {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirectURL := os.Getenv("GOOGLE_REDIRECT_URL")

	if redirectURL == "" {
		redirectURL = "http://localhost:8080/api/v1/auth/google/callback"
	}

	return OAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Enabled:      clientID != "" && clientSecret != "",
	}
}

// GoogleOAuth2Config returns a ready-to-use golang.org/x/oauth2 config for Google.
func (c *OAuthConfig) GoogleOAuth2Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.RedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}
