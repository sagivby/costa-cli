package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"

	"github.com/costa-app/costa-cli/internal/debug"
)

const (
	// DefaultClockSkew is the time buffer before token expiry to trigger refresh
	DefaultClockSkew = 5 * time.Minute

	// Keyring service name
	keyringService = "costa-cli"

	// Keyring keys for different token components (these are labels/accounts, not credentials)
	keyringOAuthAccessToken  = "oauth-access-token"  // #nosec G101
	keyringOAuthRefreshToken = "oauth-refresh-token" // #nosec G101
	keyringCodingAccessToken = "coding-access-token" // #nosec G101
)

var (
	// tokenMutex guards against concurrent refresh/fetch operations
	tokenMutex sync.Mutex

	// useKeyring determines whether to use system keyring (true) or fallback to file (false)
	useKeyring = true
)

// TokenData represents a single token (CLI or OAuth)
type TokenData struct {
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token,omitempty"`
	TokenType    string     `json:"token_type"`
}

// IsExpiredWithSkew returns true if the token is expired or will expire within the skew window
func (td *TokenData) IsExpiredWithSkew(skew time.Duration) bool {
	if td == nil || td.ExpiresAt == nil {
		return false // No expiry means long-lived token
	}
	return time.Now().Add(skew).After(*td.ExpiresAt)
}

// IsValid returns true if the token exists and is not expired with default skew
func (td *TokenData) IsValid() bool {
	if td == nil || td.AccessToken == "" {
		return false
	}
	return !td.IsExpiredWithSkew(DefaultClockSkew)
}

// Token represents the stored authentication tokens
type Token struct {
	Coding *TokenData `json:"coding,omitempty"`
	OAuth  *TokenData `json:"oauth,omitempty"`
	// CLI is kept for backward-compatibility with older token files
	CLI *TokenData `json:"cli,omitempty"`
}

// TokenMetadata represents non-sensitive token metadata stored in a file
type TokenMetadata struct {
	OAuthExpiresAt  *time.Time `json:"oauth_expires_at,omitempty"`
	OAuthTokenType  string     `json:"oauth_token_type,omitempty"`
	CodingExpiresAt *time.Time `json:"coding_expires_at,omitempty"`
	CodingTokenType string     `json:"coding_token_type,omitempty"`
}

// GetConfigDir returns the costa config directory path
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "costa")
	return configDir, nil
}

// GetTokenPath returns the path to the token file (legacy)
func GetTokenPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "token.json"), nil
}

// GetMetadataPath returns the path to the token metadata file
func GetMetadataPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "token-metadata.json"), nil
}

// SaveToken saves the token using keyring (with file fallback)
func SaveToken(token *Token) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	if useKeyring {
		// Try to save to keyring
		if err := saveTokenToKeyring(token); err != nil {
			debug.Printf("Failed to save to keyring, falling back to file: %v\n", err)
			debug.Printf("Setting useKeyring=false for future operations\n")
			useKeyring = false
			return saveTokenToFile(token)
		}
		debug.Printf("Successfully saved to keyring\n")
		return nil
	}

	debug.Printf("Using file storage (useKeyring=false)\n")
	return saveTokenToFile(token)
}

// saveTokenToKeyring saves sensitive tokens to system keyring and metadata to file
func saveTokenToKeyring(token *Token) error {
	// Save OAuth tokens to keyring if present
	if token.OAuth != nil {
		if token.OAuth.AccessToken != "" {
			if err := keyring.Set(keyringService, keyringOAuthAccessToken, token.OAuth.AccessToken); err != nil {
				return fmt.Errorf("failed to save OAuth access token to keyring: %w", err)
			}
		}
		if token.OAuth.RefreshToken != "" {
			if err := keyring.Set(keyringService, keyringOAuthRefreshToken, token.OAuth.RefreshToken); err != nil {
				return fmt.Errorf("failed to save OAuth refresh token to keyring: %w", err)
			}
		}
	}

	// Save Coding token to keyring if present
	if token.Coding != nil && token.Coding.AccessToken != "" {
		if err := keyring.Set(keyringService, keyringCodingAccessToken, token.Coding.AccessToken); err != nil {
			return fmt.Errorf("failed to save coding access token to keyring: %w", err)
		}
	}

	// Save metadata (non-sensitive) to file
	metadata := TokenMetadata{}
	if token.OAuth != nil {
		metadata.OAuthExpiresAt = token.OAuth.ExpiresAt
		metadata.OAuthTokenType = token.OAuth.TokenType
	}
	if token.Coding != nil {
		metadata.CodingExpiresAt = token.Coding.ExpiresAt
		metadata.CodingTokenType = token.Coding.TokenType
	}

	metadataPath, err := GetMetadataPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metadataPath, data, 0600)
}

// saveTokenToFile saves the entire token to a file (fallback method)
func saveTokenToFile(token *Token) error {
	tokenPath, err := GetTokenPath()
	if err != nil {
		return err
	}

	debug.Printf("Saving token to file: %s\n", tokenPath)

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tokenPath, data, 0600); err != nil {
		debug.Printf("Failed to write token file: %v\n", err)
		return err
	}

	debug.Printf("Successfully saved token to file\n")
	return nil
}

// LoadToken loads the token from keyring (with file fallback)
func LoadToken() (*Token, error) {
	// First check if token file exists (file fallback mode from previous session)
	tokenPath, err := GetTokenPath()
	if err == nil {
		if _, statErr := os.Stat(tokenPath); statErr == nil {
			debug.Printf("Loading from file (file fallback mode detected)\n")
			useKeyring = false
			return loadTokenFromFile()
		}
	}

	// Otherwise try keyring mode
	if useKeyring {
		token, err := loadTokenFromKeyring()
		if err != nil {
			debug.Printf("Failed to load from keyring, falling back to file: %v\n", err)
			useKeyring = false
			return loadTokenFromFile()
		}
		return token, nil
	}

	return loadTokenFromFile()
}

// loadTokenFromKeyring loads tokens from system keyring and metadata from file
func loadTokenFromKeyring() (*Token, error) {
	// Load metadata
	metadataPath, err := GetMetadataPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata TokenMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	token := &Token{}

	// Load OAuth tokens from keyring
	if metadata.OAuthTokenType != "" {
		oauthAccess, err := keyring.Get(keyringService, keyringOAuthAccessToken)
		if err != nil && err != keyring.ErrNotFound {
			return nil, fmt.Errorf("failed to get OAuth access token from keyring: %w", err)
		}

		oauthRefresh, _ := keyring.Get(keyringService, keyringOAuthRefreshToken)

		if oauthAccess != "" {
			token.OAuth = &TokenData{
				AccessToken:  oauthAccess,
				RefreshToken: oauthRefresh,
				TokenType:    metadata.OAuthTokenType,
				ExpiresAt:    metadata.OAuthExpiresAt,
			}
		}
	}

	// Load Coding token from keyring
	if metadata.CodingTokenType != "" {
		codingAccess, err := keyring.Get(keyringService, keyringCodingAccessToken)
		if err != nil && err != keyring.ErrNotFound {
			return nil, fmt.Errorf("failed to get coding access token from keyring: %w", err)
		}

		if codingAccess != "" {
			token.Coding = &TokenData{
				AccessToken: codingAccess,
				TokenType:   metadata.CodingTokenType,
				ExpiresAt:   metadata.CodingExpiresAt,
			}
		}
	}

	return token, nil
}

// loadTokenFromFile loads the entire token from a file (fallback method)
func loadTokenFromFile() (*Token, error) {
	tokenPath, err := GetTokenPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, err
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	// Migrate old CLI field to Coding field
	if token.CLI != nil && token.Coding == nil {
		token.Coding = token.CLI
		token.CLI = nil
	}

	return &token, nil
}

// DeleteToken removes tokens from keyring and metadata file
func DeleteToken() error {
	// Delete keyring entries
	_ = keyring.Delete(keyringService, keyringOAuthAccessToken)
	_ = keyring.Delete(keyringService, keyringOAuthRefreshToken)
	_ = keyring.Delete(keyringService, keyringCodingAccessToken)

	// Delete metadata file
	metadataPath, err := GetMetadataPath()
	if err != nil {
		return err
	}
	if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Also remove legacy file if present
	tokenPath, err := GetTokenPath()
	if err == nil {
		_ = os.Remove(tokenPath)
	}

	return nil
}

// IsLoggedIn checks if a token exists
func IsLoggedIn() bool {
	debug.Printf("Checking if logged in...\n")

	// First check if token file exists (file fallback mode)
	tokenPath, err := GetTokenPath()
	if err == nil {
		if _, err := os.Stat(tokenPath); err == nil {
			debug.Printf("Token file exists at %s (file fallback mode)\n", tokenPath)
			return true
		}
	}

	// Then check keyring mode (metadata file + keyring entries)
	debug.Printf("Checking keyring mode...\n")
	metadataPath, err := GetMetadataPath()
	if err != nil {
		debug.Printf("Failed to get metadata path: %v\n", err)
		return false
	}
	if _, err := os.Stat(metadataPath); err != nil {
		debug.Printf("Metadata file does not exist: %v\n", err)
		return false
	}
	debug.Printf("Metadata file exists at %s\n", metadataPath)

	if _, err := keyring.Get(keyringService, keyringOAuthAccessToken); err == nil {
		debug.Printf("Found OAuth access token in keyring\n")
		return true
	}
	if _, err := keyring.Get(keyringService, keyringCodingAccessToken); err == nil {
		debug.Printf("Found coding access token in keyring\n")
		return true
	}
	debug.Printf("No tokens found in keyring\n")
	return false
}

// EnsureOAuthTokenValid checks if OAuth token is valid, refreshes if needed
// Returns the current valid OAuth token or error if refresh fails
func EnsureOAuthTokenValid(ctx context.Context) (*TokenData, error) {
	tokenMutex.Lock()
	defer tokenMutex.Unlock()

	debug.Printf("Checking OAuth token validity...\n")

	// Load current token
	token, err := LoadToken()
	if err != nil {
		return nil, fmt.Errorf("failed to load token: %w", err)
	}

	if token.OAuth == nil {
		return nil, fmt.Errorf("no OAuth token found, please login first")
	}

	// Check if refresh is needed
	if !token.OAuth.IsExpiredWithSkew(DefaultClockSkew) {
		debug.Printf("OAuth token is valid (expires: %v)\n", token.OAuth.ExpiresAt)
		return token.OAuth, nil
	}

	debug.Printf("OAuth token expired or near expiry, refreshing...\n")

	// Check if we have a refresh token
	if token.OAuth.RefreshToken == "" {
		return nil, fmt.Errorf("OAuth token expired and no refresh token available, please login again")
	}

	// Perform refresh
	config := OAuthConfig()

	// Create token source for refresh
	oldToken := &oauth2.Token{
		AccessToken:  token.OAuth.AccessToken,
		RefreshToken: token.OAuth.RefreshToken,
		TokenType:    token.OAuth.TokenType,
	}
	if token.OAuth.ExpiresAt != nil {
		oldToken.Expiry = *token.OAuth.ExpiresAt
	}

	tokenSource := config.TokenSource(ctx, oldToken)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh OAuth token: %w", err)
	}

	// Update and save refreshed token
	var expiresAt *time.Time
	if !newToken.Expiry.IsZero() {
		expiresAt = &newToken.Expiry
	}

	token.OAuth = &TokenData{
		AccessToken:  newToken.AccessToken,
		RefreshToken: newToken.RefreshToken,
		TokenType:    newToken.TokenType,
		ExpiresAt:    expiresAt,
	}

	if err := SaveToken(token); err != nil {
		return nil, fmt.Errorf("failed to save refreshed OAuth token: %w", err)
	}

	debug.Printf("OAuth token refreshed successfully (expires: %v)\n", expiresAt)

	return token.OAuth, nil
}

// CodingTokenResponse represents the API response from /api/v1/tokens/coding_current
type CodingTokenResponse struct {
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token"`
}

// GetCodingToken ensures OAuth is valid and returns a valid coding token
// Fetches a new coding token if the current one is expired or missing
func GetCodingToken(ctx context.Context) (*TokenData, error) {
	// Ensure OAuth token is valid first (may refresh)
	oauthToken, err := EnsureOAuthTokenValid(ctx)
	if err != nil {
		return nil, err
	}

	// Guard the remainder to avoid concurrent fetch/save races
	tokenMutex.Lock()
	defer tokenMutex.Unlock()

	// Load current token state
	token, err := LoadToken()
	if err != nil {
		return nil, fmt.Errorf("failed to load token: %w", err)
	}

	// Check if we have a valid coding token
	if token.Coding != nil && token.Coding.IsValid() {
		debug.Printf("Coding token is valid (expires: %v)\n", token.Coding.ExpiresAt)
		return token.Coding, nil
	}

	debug.Printf("Fetching coding token from %s\n", GetCodingTokenURL())

	// Fetch new coding token
	req, err := http.NewRequestWithContext(ctx, "GET", GetCodingTokenURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oauthToken.AccessToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch coding token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	debug.Printf("Coding token response: HTTP %d\n", resp.StatusCode)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed: HTTP %d - please login again", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("coding token endpoint not found: HTTP %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		debug.Printf("Coding token error response body: %s\n", string(body))
		return nil, fmt.Errorf("failed to fetch coding token: HTTP %d - %s", resp.StatusCode, string(body))
	}

	// Read the body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	debug.Printf("Coding token response body: %s\n", string(bodyBytes))

	var codingResp CodingTokenResponse
	if err := json.Unmarshal(bodyBytes, &codingResp); err != nil {
		return nil, fmt.Errorf("failed to decode coding token response: %w", err)
	}

	// Save coding token
	var expiresAt *time.Time
	if !codingResp.ExpiresAt.IsZero() {
		expiresAt = &codingResp.ExpiresAt
	}

	token.Coding = &TokenData{
		AccessToken: codingResp.Token,
		TokenType:   "Bearer", // Default to Bearer since API doesn't return token_type
		ExpiresAt:   expiresAt,
	}

	if err := SaveToken(token); err != nil {
		return nil, fmt.Errorf("failed to save coding token: %w", err)
	}

	debug.Printf("Coding token fetched successfully (expires: %v)\n", expiresAt)

	return token.Coding, nil
}

// Model represents a model from the Costa API
type Model struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// ModelsResponse represents the response from the models API endpoint
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// GetModels fetches the list of available models from Costa API
// Note: This function does NOT lock tokenMutex to avoid deadlocks when called
// from contexts that already hold the lock (e.g., during setup flows)
func GetModels(ctx context.Context) ([]Model, error) {
	// Load current token state without locking (read-only operation)
	token, err := LoadToken()
	if err != nil {
		return nil, fmt.Errorf("failed to load token: %w", err)
	}

	// Use OAuth token if available
	if token.OAuth == nil || !token.OAuth.IsValid() {
		return nil, fmt.Errorf("no valid OAuth token found - please login first")
	}

	oauthToken := token.OAuth

	debug.Printf("Fetching models from %s\n", GetModelsURL())

	// Fetch models
	req, err := http.NewRequestWithContext(ctx, "GET", GetModelsURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oauthToken.AccessToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	debug.Printf("Models response: HTTP %d\n", resp.StatusCode)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed: HTTP %d - please login again", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("models endpoint not found: HTTP %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		debug.Printf("Models error response body: %s\n", string(body))
		return nil, fmt.Errorf("failed to fetch models: HTTP %d - %s", resp.StatusCode, string(body))
	}

	// Read and parse response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	debug.Printf("Models response body: %s\n", string(bodyBytes))

	var modelsResp ModelsResponse
	if err := json.Unmarshal(bodyBytes, &modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode models response: %w", err)
	}

	debug.Printf("Fetched %d models successfully\n", len(modelsResp.Data))

	return modelsResp.Data, nil
}
