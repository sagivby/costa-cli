package claudecode

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

// ClaudeCode implements the Integration interface for Claude Code
type ClaudeCode struct{}

// New creates a new Claude Code integration
func New() *ClaudeCode {
	return &ClaudeCode{}
}

// Name returns the name of the integration
func (c *ClaudeCode) Name() string {
	return "claude-code"
}

// Apply applies the Claude Code configuration
func (c *ClaudeCode) Apply(ctx context.Context, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	result := integrations.ApplyResult{}

	// Detect Claude CLI
	_, claudeInstalled := detectClaudeCLI()
	if !claudeInstalled && opts.RequireInstalled {
		return result, fmt.Errorf("Claude CLI not found. Install it first: https://docs.claude.com/en/docs/claude-code/quickstart")
	}

	// Resolve settings path
	settingsPath, err := resolveSettingsPath(opts.Scope)
	if err != nil {
		return result, fmt.Errorf("failed to resolve settings path: %w", err)
	}
	result.ConfigPath = settingsPath

	// Load existing settings
	existing, err := loadJSONFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("failed to load existing settings: %w", err)
	}
	if existing == nil {
		existing = make(map[string]any)
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

	// Build desired settings
	desired := buildDesiredSettings(token, opts.EnableStatusLine)

	// Merge settings
	merged, updatedKeys, unchangedKeys := mergeSettings(existing, desired, opts.RefreshTokenOnly)

	result.UpdatedKeys = updatedKeys
	result.UnchangedKeys = unchangedKeys
	// Include onboarding flag in ~/.claude.json if needed
	homeDir, _ := os.UserHomeDir()
	onboardingPath := ""
	var onboardingData map[string]any
	needsOnboarding := false
	if homeDir != "" {
		onboardingPath = filepath.Join(homeDir, ".claude.json")
		data, err := loadJSONFile(onboardingPath)
		if err != nil {
			if os.IsNotExist(err) {
				onboardingData = make(map[string]any)
			} else {
				return result, fmt.Errorf("failed to load onboarding config: %w", err)
			}
		} else {
			onboardingData = data
		}
		if v, ok := onboardingData["hasCompletedOnboarding"].(bool); !ok || !v {
			needsOnboarding = true
			result.UpdatedKeys = append(result.UpdatedKeys, "~/.claude.json.hasCompletedOnboarding")
		}
	}
	result.Changed = len(result.UpdatedKeys) > 0

	// If no changes and not dry run, we're done
	if !result.Changed {
		return result, nil
	}

	// Dry run stops here
	if opts.DryRun {
		return result, nil
	}

	// Create backup
	backupPath, err := createBackup(settingsPath, opts.BackupDir)
	if err != nil {
		return result, fmt.Errorf("failed to create backup: %w", err)
	}
	result.BackupPath = backupPath

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil {
		return result, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write settings
	if err := writeJSONFile(settingsPath, merged); err != nil {
		return result, fmt.Errorf("failed to write settings: %w", err)
	}
	if needsOnboarding {
		onboardingData["hasCompletedOnboarding"] = true
		if err := writeJSONFile(onboardingPath, onboardingData); err != nil {
			return result, fmt.Errorf("failed to write onboarding config: %w", err)
		}
	}

	return result, nil
}

// Status returns the current status of Claude Code configuration
func (c *ClaudeCode) Status(ctx context.Context, scope integrations.Scope) (integrations.StatusResult, error) {
	result := integrations.StatusResult{
		Scope: scope,
	}

	// Detect Claude CLI
	claudePath, claudeInstalled := detectClaudeCLI()
	result.Installed = claudeInstalled
	if claudeInstalled {
		result.Version = getClaudeVersion(claudePath)
	}

	// Resolve settings path
	settingsPath, err := resolveSettingsPath(scope)
	if err != nil {
		return result, fmt.Errorf("failed to resolve settings path: %w", err)
	}
	result.ConfigPath = settingsPath

	// Load existing settings
	existing, err := loadJSONFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.ConfigExists = false
			return result, nil
		}
		return result, fmt.Errorf("failed to load settings: %w", err)
	}

	result.ConfigExists = true

	// Check Costa configuration
	isCosta, missing := checkCostaConfig(existing)
	result.IsCosta = isCosta
	result.Missing = missing

	// Extract current model
	if model, ok := existing["model"].(string); ok {
		result.Model = model
	}

	// Extract redacted token
	if env, ok := existing["env"].(map[string]any); ok {
		if token, ok := env["ANTHROPIC_AUTH_TOKEN"].(string); ok && token != "" {
			result.TokenRedacted = redactToken(token)
		}
	}

	return result, nil
}

// Helper functions

func detectClaudeCLI() (string, bool) {
	path, err := exec.LookPath("claude")
	return path, err == nil
}

func getClaudeVersion(claudePath string) string {
	cmd := exec.Command(claudePath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func resolveSettingsPath(scope integrations.Scope) (string, error) {
	if scope == integrations.ScopeProject {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".claude", "settings.json"), nil
	}

	// User scope
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
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

func buildDesiredSettings(token string, enableStatusLine bool) map[string]any {
	baseURL := auth.GetBaseURL() + "/api"

	// Debug: print what we're using
	debug.Printf("DEBUG: COSTA_BASE_URL env var = %q\n", os.Getenv("COSTA_BASE_URL"))
	debug.Printf("DEBUG: Resolved base URL = %q\n", auth.GetBaseURL())
	debug.Printf("DEBUG: ANTHROPIC_BASE_URL will be set to = %q\n", baseURL)

	settings := map[string]any{
		"model":                 "costa/auto",
		"alwaysThinkingEnabled": true,
		"env": map[string]any{
			"ANTHROPIC_BASE_URL":               baseURL,
			"ANTHROPIC_AUTH_TOKEN":             token,
			"ANTHROPIC_DEFAULT_TEXT_MODEL":     "costa/auto",
			"ANTHROPIC_DEFAULT_MESSAGES_MODEL": "costa/auto",
			"ANTHROPIC_DEFAULT_TOOL_USE_MODEL": "costa/auto",
			"CLAUDE_CODE_SUBAGENT_MODEL":       "costa/auto",
			"DISABLE_PROMPT_CACHING":           true,
		},
	}

	// Add status line if enabled
	if enableStatusLine {
		// Use current executable path to support embedded CLI scenarios (e.g., VS Code extension)
		costaPath, err := os.Executable()
		if err != nil {
			// Fallback to PATH lookup
			costaPath, err = exec.LookPath("costa")
			if err != nil {
				// Last resort fallback
				costaPath = "costa"
			}
		}

		// Quote path if it contains spaces or quotes to be shell-safe
		quotedPath := costaPath
		if strings.ContainsAny(quotedPath, " \t\"") {
			quotedPath = "\"" + strings.ReplaceAll(quotedPath, "\"", "\\\"") + "\""
		}

		settings["statusLine"] = map[string]any{
			"type":    "command",
			"command": quotedPath + " status --format claude-code",
			"padding": 0,
		}
	}

	return settings
}

// mergeSettings merges desired settings into existing settings.
// Always updates values when they differ from desired (no --update flag needed).
// TODO: In the future, add option to interactively choose which settings to update.
func mergeSettings(existing, desired map[string]any, refreshTokenOnly bool) (map[string]any, []string, []string) {
	merged := make(map[string]any)
	var updatedKeys []string
	var unchangedKeys []string

	// Copy existing
	for k, v := range existing {
		merged[k] = v
	}

	// Merge logic
	if refreshTokenOnly {
		// Only update token in env
		if env, ok := merged["env"].(map[string]any); ok {
			if desiredEnv, ok := desired["env"].(map[string]any); ok {
				if token, ok := desiredEnv["ANTHROPIC_AUTH_TOKEN"].(string); ok {
					if env["ANTHROPIC_AUTH_TOKEN"] != token {
						env["ANTHROPIC_AUTH_TOKEN"] = token
						updatedKeys = append(updatedKeys, "env.ANTHROPIC_AUTH_TOKEN")
					} else {
						unchangedKeys = append(unchangedKeys, "env.ANTHROPIC_AUTH_TOKEN")
					}
				}
			}
		} else {
			// Create env if it doesn't exist
			if desiredEnv, ok := desired["env"].(map[string]any); ok {
				merged["env"] = map[string]any{
					"ANTHROPIC_AUTH_TOKEN": desiredEnv["ANTHROPIC_AUTH_TOKEN"],
				}
				updatedKeys = append(updatedKeys, "env.ANTHROPIC_AUTH_TOKEN")
			}
		}
	} else {
		// Merge all settings - always update when values differ
		for key, desiredValue := range desired {
			if key == "env" {
				// Special handling for env object
				existingEnv, hasEnv := merged["env"].(map[string]any)
				if !hasEnv {
					existingEnv = make(map[string]any)
					merged["env"] = existingEnv
				}

				desiredEnv, ok := desiredValue.(map[string]any)
				if !ok {
					continue // Skip if not a map
				}
				for envKey, envValue := range desiredEnv {
					existingVal, exists := existingEnv[envKey]

					if !exists {
						existingEnv[envKey] = envValue
						updatedKeys = append(updatedKeys, fmt.Sprintf("env.%s", envKey))
					} else if existingVal != envValue {
						existingEnv[envKey] = envValue
						updatedKeys = append(updatedKeys, fmt.Sprintf("env.%s", envKey))
					} else {
						unchangedKeys = append(unchangedKeys, fmt.Sprintf("env.%s", envKey))
					}
				}
			} else if key == "statusLine" {
				// Special handling for statusLine object
				existingStatusLine, hasStatusLine := merged["statusLine"].(map[string]any)
				desiredStatusLine, ok := desiredValue.(map[string]any)
				if !ok {
					continue // Skip if not a map
				}

				// Check if statusLine changed
				statusLineChanged := !hasStatusLine
				if hasStatusLine {
					// Compare each field
					for slKey, slValue := range desiredStatusLine {
						if existingStatusLine[slKey] != slValue {
							statusLineChanged = true
							break
						}
					}
				}

				if statusLineChanged {
					merged["statusLine"] = desiredValue
					updatedKeys = append(updatedKeys, "statusLine")
				} else {
					unchangedKeys = append(unchangedKeys, "statusLine")
				}
			} else {
				// Top-level keys - always update when different
				existingVal, exists := merged[key]
				if !exists {
					merged[key] = desiredValue
					updatedKeys = append(updatedKeys, key)
				} else if existingVal != desiredValue {
					merged[key] = desiredValue
					updatedKeys = append(updatedKeys, key)
				} else {
					unchangedKeys = append(unchangedKeys, key)
				}
			}
		}
	}

	return merged, updatedKeys, unchangedKeys
}

func checkCostaConfig(settings map[string]any) (bool, []string) {
	var missing []string

	// Check top-level model
	if model, ok := settings["model"].(string); !ok || model != "costa/auto" {
		missing = append(missing, "model")
	}

	// Check env
	env, hasEnv := settings["env"].(map[string]any)
	if !hasEnv {
		return false, []string{"env object"}
	}

	requiredEnvKeys := []string{
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_DEFAULT_TEXT_MODEL",
		"CLAUDE_CODE_SUBAGENT_MODEL",
	}

	for _, key := range requiredEnvKeys {
		if _, ok := env[key]; !ok {
			missing = append(missing, "env."+key)
		}
	}

	return len(missing) == 0, missing
}

func createBackup(sourcePath, backupDir string) (string, error) {
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
		backupDir = filepath.Join(configDir, "backups", "claude-code")
	}

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("settings-%s.json", timestamp))

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
