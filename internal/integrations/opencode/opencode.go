package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/costa-app/costa-cli/internal/auth"
	"github.com/costa-app/costa-cli/internal/debug"
	"github.com/costa-app/costa-cli/internal/integrations"
)

// OpenCode implements the Integration interface for OpenCode
type OpenCode struct{}

// New creates a new OpenCode integration
func New() *OpenCode {
	return &OpenCode{}
}

// Name returns the name of the integration
func (o *OpenCode) Name() string {
	return "opencode"
}

// Apply applies the OpenCode configuration
func (o *OpenCode) Apply(ctx context.Context, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	result := integrations.ApplyResult{}

	// Detect OpenCode CLI
	_, opencodeInstalled := detectOpenCodeCLI()
	if !opencodeInstalled && opts.RequireInstalled {
		return result, fmt.Errorf("OpenCode CLI not found. Please install it first: https://opencode.ai")
	}

	// Resolve config and auth paths
	configPath, err := resolveConfigPath(opts.Scope)
	if err != nil {
		return result, fmt.Errorf("failed to resolve config path: %w", err)
	}
	authPath, err := resolveAuthPath(opts.Scope)
	if err != nil {
		return result, fmt.Errorf("failed to resolve auth path: %w", err)
	}
	result.ConfigPath = configPath

	// Load existing configs
	existingConfig, err := loadJSONFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("failed to load existing config: %w", err)
	}
	if existingConfig == nil {
		existingConfig = make(map[string]any)
	}

	existingAuth, err := loadJSONFile(authPath)
	if err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("failed to load existing auth: %w", err)
	}
	if existingAuth == nil {
		existingAuth = make(map[string]any)
	}

	// Get token
	token := opts.TokenOverride
	if token == "" {
		debug.Printf("Fetching coding token from Costa...\n")
		tokenData, err := auth.GetCodingToken(ctx)
		if err != nil {
			return result, fmt.Errorf("failed to get Costa token: %w\nRun 'costa login' first", err)
		}
		token = tokenData.AccessToken
	}

	// Build desired configs
	desiredConfig := buildDesiredConfig(ctx)
	desiredAuth := buildDesiredAuth(token)

	// Merge configs
	mergedConfig, configUpdatedKeys, configUnchangedKeys := mergeConfig(existingConfig, desiredConfig, opts.RefreshTokenOnly)
	mergedAuth, authUpdatedKeys, authUnchangedKeys := mergeAuth(existingAuth, desiredAuth, opts.RefreshTokenOnly)

	// Combine updated/unchanged keys
	result.UpdatedKeys = make([]string, 0, len(configUpdatedKeys)+len(authUpdatedKeys))
	result.UpdatedKeys = append(result.UpdatedKeys, configUpdatedKeys...)
	result.UpdatedKeys = append(result.UpdatedKeys, authUpdatedKeys...)
	result.UnchangedKeys = make([]string, 0, len(configUnchangedKeys)+len(authUnchangedKeys))
	result.UnchangedKeys = append(result.UnchangedKeys, configUnchangedKeys...)
	result.UnchangedKeys = append(result.UnchangedKeys, authUnchangedKeys...)
	result.Changed = len(result.UpdatedKeys) > 0

	// If no changes and not dry run, we're done
	if !result.Changed {
		return result, nil
	}

	// Dry run stops here
	if opts.DryRun {
		return result, nil
	}

	// Create backups
	configBackup, err := createBackup(configPath, opts.BackupDir, "opencode-config")
	if err != nil {
		return result, fmt.Errorf("failed to create config backup: %w", err)
	}
	authBackup, err := createBackup(authPath, opts.BackupDir, "opencode-auth")
	if err != nil {
		return result, fmt.Errorf("failed to create auth backup: %w", err)
	}
	if configBackup != "" {
		result.BackupPath = configBackup
	} else if authBackup != "" {
		result.BackupPath = authBackup
	}

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return result, fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(authPath), 0700); err != nil {
		return result, fmt.Errorf("failed to create auth directory: %w", err)
	}

	// Write configs
	if err := writeJSONFile(configPath, mergedConfig); err != nil {
		return result, fmt.Errorf("failed to write config: %w", err)
	}
	if err := writeJSONFile(authPath, mergedAuth); err != nil {
		return result, fmt.Errorf("failed to write auth: %w", err)
	}

	return result, nil
}

// Status returns the current status of OpenCode configuration
func (o *OpenCode) Status(ctx context.Context, scope integrations.Scope) (integrations.StatusResult, error) {
	result := integrations.StatusResult{
		Scope: scope,
	}

	// Detect OpenCode CLI
	opencodePath, opencodeInstalled := detectOpenCodeCLI()
	result.Installed = opencodeInstalled
	if opencodeInstalled {
		result.Version = getOpenCodeVersion(opencodePath)
	}

	// Resolve config path
	configPath, err := resolveConfigPath(scope)
	if err != nil {
		return result, fmt.Errorf("failed to resolve config path: %w", err)
	}
	result.ConfigPath = configPath

	// Load existing config
	existingConfig, err := loadJSONFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.ConfigExists = false
			return result, nil
		}
		return result, fmt.Errorf("failed to load config: %w", err)
	}

	result.ConfigExists = true

	// Check Costa configuration
	authPath, _ := resolveAuthPath(scope)
	existingAuth, _ := loadJSONFile(authPath)
	isCosta, missing := checkCostaConfig(existingConfig, existingAuth)
	result.IsCosta = isCosta
	result.Missing = missing

	// Extract current model
	if model, ok := existingConfig["model"].(string); ok {
		result.Model = model
	}

	// Extract redacted token from auth
	if existingAuth != nil {
		if costa, ok := existingAuth["costa"].(map[string]any); ok {
			if key, ok := costa["key"].(string); ok && key != "" {
				result.TokenRedacted = redactToken(key)
			}
		}
	}

	return result, nil
}

// Helper functions

func detectOpenCodeCLI() (string, bool) {
	path, err := exec.LookPath("opencode")
	return path, err == nil
}

func getOpenCodeVersion(opencodePath string) string {
	cmd := exec.Command(opencodePath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func resolveConfigPath(scope integrations.Scope) (string, error) {
	if scope == integrations.ScopeProject {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".config", "opencode", "opencode.json"), nil
	}

	// User scope
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json"), nil
}

func resolveAuthPath(scope integrations.Scope) (string, error) {
	if scope == integrations.ScopeProject {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".local", "share", "opencode", "auth.json"), nil
	}

	// User scope
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json"), nil
}

func loadJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func writeJSONFile(path string, data map[string]any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file then rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0600); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func buildDesiredConfig(ctx context.Context) map[string]any {
	baseURL := auth.GetBaseURL() + "/api/v1"

	// Fetch models dynamically from Costa API
	models := fetchModels(ctx)

	config := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]any{
			"costa": map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "Costa Code",
				"options": map[string]any{
					"baseURL": baseURL,
				},
				"models": models,
			},
		},
		"model":       "costa/costa/auto",
		"small_model": "costa/orbit",
	}

	return config
}

// fetchModels fetches available models from Costa API with fallback to static list
func fetchModels(ctx context.Context) map[string]any {
	// Try to fetch models from API
	apiModels, err := auth.GetModels(ctx)
	if err != nil {
		debug.Printf("Failed to fetch models from API, using static fallback: %v\n", err)
		return getStaticModels()
	}

	// Convert API models to OpenCode format
	models := make(map[string]any)
	for _, model := range apiModels {
		// Use model name if available, otherwise use ID
		displayName := model.Name
		if displayName == "" {
			displayName = model.ID
		}
		models[model.ID] = map[string]any{"name": displayName}
	}

	// If no models returned, use static fallback
	if len(models) == 0 {
		debug.Printf("No models returned from API, using static fallback\n")
		return getStaticModels()
	}

	debug.Printf("Fetched %d models dynamically from Costa API\n", len(models))
	return models
}

// getStaticModels returns a fallback list of static models
func getStaticModels() map[string]any {
	return map[string]any{
		"costa/auto": map[string]any{"name": "Costa Auto"},
		"orbit":      map[string]any{"name": "Orbit"},
	}
}

func buildDesiredAuth(token string) map[string]any {
	auth := map[string]any{
		"costa": map[string]any{
			"type": "api",
			"key":  token,
		},
	}

	return auth
}

// mergeConfig merges desired config into existing config
func mergeConfig(existing, desired map[string]any, refreshTokenOnly bool) (map[string]any, []string, []string) {
	// If refreshTokenOnly, we don't modify the config file, only auth file
	if refreshTokenOnly {
		return existing, []string{}, []string{}
	}

	merged := make(map[string]any)
	var updatedKeys []string
	var unchangedKeys []string

	// Copy existing
	for k, v := range existing {
		merged[k] = v
	}

	// Merge top-level keys
	for key, desiredValue := range desired {
		if key == "provider" {
			// Special handling for provider object
			existingProvider, hasProvider := merged["provider"].(map[string]any)
			if !hasProvider {
				existingProvider = make(map[string]any)
				merged["provider"] = existingProvider
			}

			desiredProvider, ok := desiredValue.(map[string]any)
			if !ok {
				continue
			}

			// Merge costa provider
			if costaConfig, ok := desiredProvider["costa"].(map[string]any); ok {
				existingCosta, hasCosta := existingProvider["costa"].(map[string]any)

				costaChanged := !hasCosta
				if hasCosta {
					// Deep compare costa config
					if !deepEqual(existingCosta, costaConfig) {
						costaChanged = true
					}
				}

				if costaChanged {
					existingProvider["costa"] = costaConfig
					updatedKeys = append(updatedKeys, "provider.costa")
				} else {
					unchangedKeys = append(unchangedKeys, "provider.costa")
				}
			}
		} else {
			// Top-level keys
			existingVal, exists := merged[key]
			switch {
			case !exists:
				merged[key] = desiredValue
				updatedKeys = append(updatedKeys, key)
			case !deepEqual(existingVal, desiredValue):
				merged[key] = desiredValue
				updatedKeys = append(updatedKeys, key)
			default:
				unchangedKeys = append(unchangedKeys, key)
			}
		}
	}

	return merged, updatedKeys, unchangedKeys
}

// mergeAuth merges desired auth into existing auth (preserving other providers)
func mergeAuth(existing, desired map[string]any, refreshTokenOnly bool) (map[string]any, []string, []string) {
	merged := make(map[string]any)
	var updatedKeys []string
	var unchangedKeys []string

	// Copy existing (preserves other providers like openrouter)
	for k, v := range existing {
		merged[k] = v
	}

	// Merge costa section
	if costaAuth, ok := desired["costa"].(map[string]any); ok {
		existingCosta, hasCosta := merged["costa"].(map[string]any)

		if refreshTokenOnly {
			// Only update the key field
			if hasCosta {
				if key, ok := costaAuth["key"].(string); ok {
					if existingCosta["key"] != key {
						existingCosta["key"] = key
						updatedKeys = append(updatedKeys, "auth.costa.key")
					} else {
						unchangedKeys = append(unchangedKeys, "auth.costa.key")
					}
				}
			} else {
				// Create costa section if it doesn't exist
				merged["costa"] = costaAuth
				updatedKeys = append(updatedKeys, "auth.costa.key")
			}
		} else {
			// Full merge
			costaChanged := !hasCosta
			if hasCosta {
				if !deepEqual(existingCosta, costaAuth) {
					costaChanged = true
				}
			}

			if costaChanged {
				merged["costa"] = costaAuth
				updatedKeys = append(updatedKeys, "auth.costa")
			} else {
				unchangedKeys = append(unchangedKeys, "auth.costa")
			}
		}
	}

	return merged, updatedKeys, unchangedKeys
}

// deepEqual performs deep equality comparison
func deepEqual(a, b any) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

func checkCostaConfig(config, auth map[string]any) (bool, []string) {
	var missing []string

	// Check config file
	if config == nil {
		return false, []string{"config file"}
	}

	// Check model
	if model, ok := config["model"].(string); !ok || model != "costa/costa/auto" {
		missing = append(missing, "model")
	}

	// Check small_model
	if smallModel, ok := config["small_model"].(string); !ok || smallModel != "costa/orbit" {
		missing = append(missing, "small_model")
	}

	// Check provider.costa
	if provider, ok := config["provider"].(map[string]any); ok {
		if _, ok := provider["costa"]; !ok {
			missing = append(missing, "provider.costa")
		}
	} else {
		missing = append(missing, "provider")
	}

	// Check auth file
	if auth == nil {
		missing = append(missing, "auth file")
		return false, missing
	}

	// Check auth.costa
	if costa, ok := auth["costa"].(map[string]any); ok {
		if _, ok := costa["key"]; !ok {
			missing = append(missing, "auth.costa.key")
		}
	} else {
		missing = append(missing, "auth.costa")
	}

	return len(missing) == 0, missing
}

func createBackup(sourcePath, backupDir, name string) (string, error) {
	// Check if source exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return "", nil // No backup needed for non-existent file
	}

	// Determine backup directory
	if backupDir == "" {
		configDir, err := auth.GetConfigDir()
		if err != nil {
			return "", err
		}
		backupDir = filepath.Join(configDir, "backups", "opencode")
	}

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s-%s.json", name, timestamp))

	// Copy file
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", err
	}

	return backupPath, nil
}

func redactToken(token string) string {
	if len(token) <= 10 {
		return "****"
	}
	return token[:6] + "****" + token[len(token)-4:]
}
