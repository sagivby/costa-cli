package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSetupClaudeCode_DryRun(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()

	// Mock HOME to point to temp dir
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Capture output
	var outBuf, errBuf bytes.Buffer

	// Create root and add setup command
	root := &cobra.Command{Use: "costa"}
	root.AddCommand(setupCmd)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	// Run setup with dry-run flag
	root.SetArgs([]string{"setup", "claude-code", "--token", "test-token", "--dry-run"})

	// Reset flags after test
	defer func() {
		ccSetupDryRun = false
		ccSetupToken = ""
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify dry-run message is present
	if !strings.Contains(output, "Dry run - no changes made") {
		t.Errorf("Expected dry-run message in output, got:\n%s", output)
	}

	// Verify changes are shown
	if !strings.Contains(output, "Changes to apply:") {
		t.Errorf("Expected changes list in output, got:\n%s", output)
	}

	// Verify no config file was created
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); err == nil {
		t.Errorf("Expected no config file to be created in dry-run mode, but found: %s", settingsPath)
	}
}

func TestSetupClaudeCode_ForceSkipsPrompts(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Create directory
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Mock HOME to point to temp dir
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Capture output
	var outBuf, errBuf bytes.Buffer

	// Create root and add setup command
	root := &cobra.Command{Use: "costa"}
	root.AddCommand(setupCmd)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	// Run setup with force flag (should skip prompts)
	root.SetArgs([]string{"setup", "claude-code", "--token", "test-token", "--force", "--skip-statusline"})

	// Reset flags after test
	defer func() {
		ccSetupForce = false
		ccSetupToken = ""
		ccSetupSkipStatusLine = false
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify no prompts are shown
	if strings.Contains(output, "Proceed with changes?") {
		t.Errorf("Expected no proceed prompt with --force, got:\n%s", output)
	}
	if strings.Contains(output, "Include status line?") {
		t.Errorf("Expected no status line prompt with --skip-statusline, got:\n%s", output)
	}

	// Verify success message
	if !strings.Contains(output, "Successfully configured Claude Code for Costa") {
		t.Errorf("Expected success message in output, got:\n%s", output)
	}

	// Verify config file was created
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Errorf("Expected config file to be created at: %s", settingsPath)
	}

	// Verify config contents
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Failed to parse settings: %v", err)
	}

	if model, ok := settings["model"].(string); !ok || model != "costa/auto" {
		t.Errorf("Expected model to be 'costa/auto', got: %v", settings["model"])
	}

	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatal("Expected env object in settings")
	}

	if token, ok := env["ANTHROPIC_AUTH_TOKEN"].(string); !ok || token != "test-token" {
		t.Errorf("Expected token to be 'test-token', got: %v", env["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestSetupClaudeCode_RefreshTokenOnly(t *testing.T) {
	// Setup temp directory with existing config
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Create directory
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Write existing config with old token and additional fields
	existingConfig := map[string]any{
		"model":                 "costa/auto",
		"alwaysThinkingEnabled": true,
		"customField":           "should-remain",
		"env": map[string]any{
			"ANTHROPIC_BASE_URL":   "https://ai.costa.app/api",
			"ANTHROPIC_AUTH_TOKEN": "old-token",
			"CUSTOM_ENV":           "should-remain",
		},
	}

	data, _ := json.MarshalIndent(existingConfig, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0600); err != nil {
		t.Fatalf("Failed to write existing config: %v", err)
	}

	// Mock HOME to point to temp dir
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Capture output
	var outBuf, errBuf bytes.Buffer

	// Create root and add setup command
	root := &cobra.Command{Use: "costa"}
	root.AddCommand(setupCmd)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	// Run setup with refresh-token-only flag
	root.SetArgs([]string{"setup", "claude-code", "--token", "new-token", "--force", "--refresh-token-only"})

	// Reset flags after test
	defer func() {
		ccSetupForce = false
		ccSetupToken = ""
		ccSetupRefreshTokenOnly = false
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Verify config was updated
	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Failed to parse settings: %v", err)
	}

	// Verify custom fields were preserved
	if custom, ok := settings["customField"].(string); !ok || custom != "should-remain" {
		t.Errorf("Expected customField to be preserved, got: %v", settings["customField"])
	}

	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatal("Expected env object in settings")
	}

	// Verify token was updated
	if token, ok := env["ANTHROPIC_AUTH_TOKEN"].(string); !ok || token != "new-token" {
		t.Errorf("Expected token to be updated to 'new-token', got: %v", env["ANTHROPIC_AUTH_TOKEN"])
	}

	// Verify custom env was preserved
	if customEnv, ok := env["CUSTOM_ENV"].(string); !ok || customEnv != "should-remain" {
		t.Errorf("Expected CUSTOM_ENV to be preserved, got: %v", env["CUSTOM_ENV"])
	}
}

func TestSetupClaudeCode_ForceSkipsStatusLinePrompt(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Create directory
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Mock HOME to point to temp dir
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Capture output
	var outBuf, errBuf bytes.Buffer

	// Create root and add setup command
	root := &cobra.Command{Use: "costa"}
	root.AddCommand(setupCmd)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	// Run setup with ONLY force flag (no --skip-statusline)
	// This should skip BOTH the proceed prompt AND the status line prompt
	root.SetArgs([]string{"setup", "claude-code", "--token", "test-token", "--force"})

	// Reset flags after test
	defer func() {
		ccSetupForce = false
		ccSetupToken = ""
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify no prompts are shown
	if strings.Contains(output, "Proceed with changes?") {
		t.Errorf("Expected no proceed prompt with --force, got:\n%s", output)
	}
	if strings.Contains(output, "Include status line?") {
		t.Errorf("Expected no status line prompt with --force, got:\n%s", output)
	}

	// Verify success message
	if !strings.Contains(output, "Successfully configured Claude Code for Costa") {
		t.Errorf("Expected success message in output, got:\n%s", output)
	}

	// Verify config file was created
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Errorf("Expected config file to be created at: %s", settingsPath)
	}
}

func TestSetupClaudeCode_FormatJSON(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Create directory
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Mock HOME to point to temp dir
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Capture output
	var outBuf, errBuf bytes.Buffer

	// Create root and add setup command
	root := &cobra.Command{Use: "costa"}
	root.AddCommand(setupCmd)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	// Run setup with format json flag
	root.SetArgs([]string{"setup", "claude-code", "--token", "test-token", "--format", "json"})

	// Reset flags after test
	defer func() {
		ccSetupToken = ""
		ccSetupFormat = ""
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify JSON output
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Expected valid JSON output, got: %s\nError: %v", output, err)
	}

	// Verify status field
	if status, ok := result["status"].(string); !ok || status != "success" {
		t.Errorf("Expected status 'success', got: %v", result["status"])
	}

	// Verify data field
	resData, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatalf("Expected data field to be an object, got: %v", result["data"])
	}

	// Verify data contains expected fields
	if message, ok := resData["message"].(string); !ok || message == "" {
		t.Errorf("Expected message in data, got: %v", resData["message"])
	}

	if changed, ok := resData["changed"].(bool); !ok || !changed {
		t.Errorf("Expected changed to be true, got: %v", resData["changed"])
	}

	if configPath, ok := resData["config_path"].(string); !ok || configPath != settingsPath {
		t.Errorf("Expected config_path to be %s, got: %v", settingsPath, resData["config_path"])
	}

	// Verify no human-readable output
	if strings.Contains(output, "✓") || strings.Contains(output, "📝") {
		t.Errorf("Expected no human-readable output in JSON mode, got: %s", output)
	}

	// Verify config file was created
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Errorf("Expected config file to be created at: %s", settingsPath)
	}

	// Verify config contents include statusLine
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("Failed to parse settings: %v", err)
	}

	// Verify statusLine is configured when using --format json
	statusLine, hasStatusLine := settings["statusLine"].(map[string]any)
	if !hasStatusLine {
		t.Errorf("Expected statusLine to be configured with --format json, but it's missing")
	} else {
		if slType, ok := statusLine["type"].(string); !ok || slType != "command" {
			t.Errorf("Expected statusLine.type to be 'command', got: %v", statusLine["type"])
		}
		if cmd, ok := statusLine["command"].(string); !ok || !strings.Contains(cmd, "costa status") {
			t.Errorf("Expected statusLine.command to contain 'costa status', got: %v", statusLine["command"])
		}
	}
}

func TestSetupClaudeCode_ForceWithFormatJSON(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".claude")

	// Create directory
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Mock HOME to point to temp dir
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Capture output
	var outBuf, errBuf bytes.Buffer

	// Create root and add setup command
	root := &cobra.Command{Use: "costa"}
	root.AddCommand(setupCmd)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	// Run setup with both force and format json flags
	root.SetArgs([]string{"setup", "claude-code", "--token", "test-token", "--force", "--format", "json"})

	// Reset flags after test
	defer func() {
		ccSetupToken = ""
		ccSetupFormat = ""
		ccSetupForce = false
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify JSON output
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Expected valid JSON output, got: %s\nError: %v", output, err)
	}

	// Verify status field
	if status, ok := result["status"].(string); !ok || status != "success" {
		t.Errorf("Expected status 'success', got: %v", result["status"])
	}

	// Verify no prompts in output
	if strings.Contains(output, "Proceed with changes?") || strings.Contains(output, "Include status line?") {
		t.Errorf("Expected no prompts with --force and --format json, got: %s", output)
	}
}

func TestSetupClaudeCode_AlreadyConfigured(t *testing.T) {
	// Setup temp directory with fully configured settings
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")
	onboardingPath := filepath.Join(tmpDir, ".claude.json")

	// Create directory
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Write fully configured config
	existingConfig := map[string]any{
		"model":                 "costa/auto",
		"alwaysThinkingEnabled": true,
		"env": map[string]any{
			"ANTHROPIC_BASE_URL":               "https://ai.costa.app/api",
			"ANTHROPIC_AUTH_TOKEN":             "test-token",
			"ANTHROPIC_DEFAULT_TEXT_MODEL":     "costa/auto",
			"ANTHROPIC_DEFAULT_MESSAGES_MODEL": "costa/auto",
			"ANTHROPIC_DEFAULT_TOOL_USE_MODEL": "costa/auto",
			"CLAUDE_CODE_SUBAGENT_MODEL":       "costa/auto",
			"DISABLE_PROMPT_CACHING":           true,
		},
	}

	data, _ := json.MarshalIndent(existingConfig, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0600); err != nil {
		t.Fatalf("Failed to write existing config: %v", err)
	}

	// Write onboarding file to avoid that being treated as a change
	onboardingConfig := map[string]any{
		"hasCompletedOnboarding": true,
	}
	onboardingData, _ := json.MarshalIndent(onboardingConfig, "", "  ")
	if err := os.WriteFile(onboardingPath, onboardingData, 0600); err != nil {
		t.Fatalf("Failed to write onboarding file: %v", err)
	}

	// Mock HOME to point to temp dir
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Capture output
	var outBuf, errBuf bytes.Buffer

	// Create root and add setup command
	root := &cobra.Command{Use: "costa"}
	root.AddCommand(setupCmd)
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	// Run setup with same token and skip statusline to avoid changes
	root.SetArgs([]string{"setup", "claude-code", "--token", "test-token", "--skip-statusline"})

	// Reset flags after test
	defer func() {
		ccSetupToken = ""
		ccSetupSkipStatusLine = false
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify "already configured" message
	if !strings.Contains(output, "Already configured! No changes needed.") {
		t.Errorf("Expected already configured message, got:\n%s", output)
	}

	// Verify no prompts shown
	if strings.Contains(output, "Proceed with changes?") {
		t.Errorf("Should not show proceed prompt when already configured, got:\n%s", output)
	}
}
