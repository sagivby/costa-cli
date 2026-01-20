package auth

import (
	"os"

	"golang.org/x/oauth2"
)

const (
	ClientID     = "439DF956-14AC-41FC-99A5-C17F6DA6264B"
	RedirectPort = "8765"
)

// GetBaseURL returns the base URL for OAuth endpoints, with env var override support
func GetBaseURL() string {
	if baseURL := os.Getenv("COSTA_BASE_URL"); baseURL != "" {
		return baseURL
	}
	return "https://ai.costa.app"
}

// GetAuthURL returns the OAuth authorization URL
func GetAuthURL() string {
	return GetBaseURL() + "/oauth/authorize"
}

// GetTokenURL returns the OAuth token URL
func GetTokenURL() string {
	return GetBaseURL() + "/oauth/token"
}

// GetRedirectURL returns the OAuth redirect URL
func GetRedirectURL() string {
	return "http://127.0.0.1:" + RedirectPort + "/costa-code-cli/callback"
}

// GetCodingTokenURL returns the coding token endpoint URL
func GetCodingTokenURL() string {
	return GetBaseURL() + "/api/v1/tokens/coding_current"
}

// GetModelsURL returns the models list endpoint URL
func GetModelsURL() string {
	return GetBaseURL() + "/api/v1/models"
}

// OAuthConfig returns a configured oauth2.Config for reuse across the CLI
func OAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:    ClientID,
		RedirectURL: GetRedirectURL(),
		Endpoint: oauth2.Endpoint{
			AuthURL:  GetAuthURL(),
			TokenURL: GetTokenURL(),
		},
		Scopes: []string{"api_tokens:read", "usage"},
	}
}
