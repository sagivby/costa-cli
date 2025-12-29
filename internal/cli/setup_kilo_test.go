package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

func TestSetupKilo_DryRun(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

	// Setup temp directory with mock VS Code database
	tmpDir := t.TempDir()
	dbPath := setupMockVSCodeDB(t, tmpDir, nil)

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
	root.SetArgs([]string{"setup", "kilo", "--token", "test-token", "--dry-run"})

	// Reset flags after test
	defer func() {
		kiloSetupDryRun = false
		kiloSetupToken = ""
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

	// Verify database was not modified
	config := loadKiloConfigFromDB(t, dbPath)
	if config != nil {
		t.Errorf("Expected database to remain empty in dry-run mode, but config was found")
	}

	secret := loadSecretBufferFromDB(t, dbPath, "kilocode.kilo-code", "openAiApiKey")
	if secret != nil {
		t.Errorf("Expected no secret to be written in dry-run mode, but found one")
	}
}

func TestSetupKilo_ForceSkipsPrompts(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

	// Setup temp directory with mock VS Code database
	tmpDir := t.TempDir()
	setupMockVSCodeDB(t, tmpDir, nil)

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
	root.SetArgs([]string{"setup", "kilo", "--token", "test-token", "--force"})

	// Reset flags after test
	defer func() {
		kiloSetupForce = false
		kiloSetupToken = ""
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

	// Verify success message
	if !strings.Contains(output, "Successfully configured Kilo for Costa") {
		t.Errorf("Expected success message in output, got:\n%s", output)
	}

	// Verify database was updated
	dbPath := filepath.Join(tmpDir, "Library", "Application Support", "Code", "User", "globalStorage", "state.vscdb")
	config := loadKiloConfigFromDB(t, dbPath)
	if config == nil {
		t.Fatal("Expected config to be created")
	}

	// Verify config values
	if baseURL, ok := config["openAiBaseUrl"].(string); !ok || !strings.Contains(baseURL, "costa.app") {
		t.Errorf("Expected openAiBaseUrl to contain 'costa.app', got: %v", config["openAiBaseUrl"])
	}

	if modelID, ok := config["openAiModelId"].(string); !ok || modelID != "costa/auto" {
		t.Errorf("Expected openAiModelId to be 'costa/auto', got: %v", config["openAiModelId"])
	}

	secret := loadSecretBufferFromDB(t, dbPath, "kilocode.kilo-code", "openAiApiKey")
	if len(secret) == 0 {
		t.Errorf("Expected API key secret to be written, but it was empty")
	}
}

func TestSetupKilo_AlreadyConfigured(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

	// Setup temp directory with fully configured Kilo
	tmpDir := t.TempDir()
	existingConfig := map[string]any{
		"openAiBaseUrl": "https://ai.costa.app/api/v1",
		"openAiModelId": "costa/auto",
		"apiProvider":   "openai",
		"id":            "costa_default",
	}
	dbPath := setupMockVSCodeDB(t, tmpDir, existingConfig)

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

	// Run setup with same configuration
	root.SetArgs([]string{"setup", "kilo", "--token", "test-token"})

	// Reset flags after test
	defer func() {
		kiloSetupToken = ""
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify changes are shown for API key update
	if !strings.Contains(output, "Changes to apply:") {
		t.Errorf("Expected changes to apply message, got:\n%s", output)
	}
	if !strings.Contains(output, "openAiApiKey") {
		t.Errorf("Expected openAiApiKey change to be listed, got:\n%s", output)
	}

	// Verify config was not changed
	config := loadKiloConfigFromDB(t, dbPath)
	if config == nil {
		t.Fatal("Expected config to still exist")
	}

	if baseURL, ok := config["openAiBaseUrl"].(string); !ok || baseURL != "https://ai.costa.app/api/v1" {
		t.Errorf("Expected openAiBaseUrl to remain unchanged, got: %v", config["openAiBaseUrl"])
	}

	secret := loadSecretBufferFromDB(t, dbPath, "kilocode.kilo-code", "openAiApiKey")
	if len(secret) == 0 {
		t.Errorf("Expected API key secret to be written, but it was empty")
	}
}

func TestSetupKilo_UpdateExistingConfig(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

	// Setup temp directory with existing Kilo config that needs update
	tmpDir := t.TempDir()
	existingConfig := map[string]any{
		"openAiBaseUrl": "https://api.openai.com/v1",
		"openAiModelId": "gpt-4",
		"apiProvider":   "openai",
		"customField":   "should-remain",
		"id":            "old_config",
	}
	dbPath := setupMockVSCodeDB(t, tmpDir, existingConfig)

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

	// Run setup to update config
	root.SetArgs([]string{"setup", "kilo", "--token", "test-token", "--force"})

	// Reset flags after test
	defer func() {
		kiloSetupForce = false
		kiloSetupToken = ""
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify update message
	if !strings.Contains(output, "Changes to apply:") {
		t.Errorf("Expected changes to apply message, got:\n%s", output)
	}

	// Verify backup was created
	if !strings.Contains(output, "Backup created:") {
		t.Errorf("Expected backup message in output, got:\n%s", output)
	}

	// Verify config was updated
	config := loadKiloConfigFromDB(t, dbPath)
	if config == nil {
		t.Fatal("Expected config to exist after update")
	}

	// Verify Costa values were applied
	if baseURL, ok := config["openAiBaseUrl"].(string); !ok || !strings.Contains(baseURL, "costa.app") {
		t.Errorf("Expected openAiBaseUrl to be updated to Costa URL, got: %v", config["openAiBaseUrl"])
	}

	if modelID, ok := config["openAiModelId"].(string); !ok || modelID != "costa/auto" {
		t.Errorf("Expected openAiModelId to be updated to 'costa/auto', got: %v", config["openAiModelId"])
	}

	// Verify custom field was preserved
	if customField, ok := config["customField"].(string); !ok || customField != "should-remain" {
		t.Errorf("Expected customField to be preserved, got: %v", config["customField"])
	}

	secret := loadSecretBufferFromDB(t, dbPath, "kilocode.kilo-code", "openAiApiKey")
	if len(secret) == 0 {
		t.Errorf("Expected API key secret to be written, but it was empty")
	}
}

func TestSetupKilo_CustomBackupDir(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

	// Setup temp directory with existing config
	tmpDir := t.TempDir()
	customBackupDir := filepath.Join(tmpDir, "custom-backups")
	existingConfig := map[string]any{
		"openAiBaseUrl": "https://api.openai.com/v1",
		"openAiModelId": "gpt-4",
	}
	setupMockVSCodeDB(t, tmpDir, existingConfig)

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

	// Run setup with custom backup directory
	root.SetArgs([]string{"setup", "kilo", "--token", "test-token", "--force", "--backup-dir", customBackupDir})

	// Reset flags after test
	defer func() {
		kiloSetupForce = false
		kiloSetupToken = ""
		kiloSetupBackupDir = ""
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	// Verify backup was created in custom directory
	if !strings.Contains(output, customBackupDir) {
		t.Errorf("Expected backup path to contain custom directory, got:\n%s", output)
	}

	// Verify backup directory exists
	if _, err := os.Stat(customBackupDir); os.IsNotExist(err) {
		t.Errorf("Expected custom backup directory to exist at: %s", customBackupDir)
	}

	// Verify backup file exists
	entries, err := os.ReadDir(customBackupDir)
	if err != nil {
		t.Fatalf("Failed to read backup directory: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected exactly one backup file, found: %d", len(entries))
	}

	dbPath := filepath.Join(tmpDir, "Library", "Application Support", "Code", "User", "globalStorage", "state.vscdb")
	secret := loadSecretBufferFromDB(t, dbPath, "kilocode.kilo-code", "openAiApiKey")
	if len(secret) == 0 {
		t.Errorf("Expected API key secret to be written, but it was empty")
	}
}

func TestSetupKilo_WritesAPIKeySecret(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

	// Setup temp directory
	tmpDir := t.TempDir()
	setupMockVSCodeDB(t, tmpDir, nil)

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

	// Run setup
	root.SetArgs([]string{"setup", "kilo", "--token", "test-token-abc123", "--force"})

	// Reset flags after test
	defer func() {
		kiloSetupForce = false
		kiloSetupToken = ""
	}()

	err := root.Execute()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	output := outBuf.String()

	dbPath := filepath.Join(tmpDir, "Library", "Application Support", "Code", "User", "globalStorage", "state.vscdb")
	secret := loadSecretBufferFromDB(t, dbPath, "kilocode.kilo-code", "openAiApiKey")
	if len(secret) == 0 {
		t.Errorf("Expected API key secret to be written, but it was empty")
	}

	if strings.Contains(output, "test-token-abc123") {
		t.Errorf("Expected token to not be printed, got:\n%s", output)
	}
}

func TestSetupKilo_InvalidIDE(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

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

	// Run setup with invalid IDE
	root.SetArgs([]string{"setup", "kilo", "--token", "test-token", "--ide", "invalid-ide"})

	// Reset flags after test
	defer func() {
		kiloSetupToken = ""
		kiloSetupIDE = ""
	}()

	err := root.Execute()
	if err == nil {
		t.Fatal("Expected error for invalid IDE, but got none")
	}

	if !strings.Contains(err.Error(), "invalid IDE") {
		t.Errorf("Expected 'invalid IDE' error message, got: %v", err)
	}
}

func TestSetupKilo_UnsupportedIDE(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

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

	// Run setup with unsupported IDE (cursor is valid but not yet supported)
	root.SetArgs([]string{"setup", "kilo", "--token", "test-token", "--ide", "cursor"})

	// Reset flags after test
	defer func() {
		kiloSetupToken = ""
		kiloSetupIDE = ""
	}()

	err := root.Execute()
	if err == nil {
		t.Fatal("Expected error for unsupported IDE, but got none")
	}

	if !strings.Contains(err.Error(), "support is coming soon") {
		t.Errorf("Expected 'support is coming soon' error message, got: %v", err)
	}
}

func TestSetupKilo_IDENotInstalled(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

	// This test can't guarantee VS Code is not installed
	// So we'll just verify the error handling path exists
	t.Skip("Test requires VS Code to not be installed")
}

// Helper functions

// setupMockVSCodeDB creates a mock VS Code database for testing
func setupMockVSCodeDB(t *testing.T, tmpDir string, existingConfig map[string]any) string {
	t.Helper()

	dbDir := filepath.Join(tmpDir, "Library", "Application Support", "Code", "User", "globalStorage")
	dbPath := filepath.Join(dbDir, "state.vscdb")

	// Create directory
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("Failed to create database directory: %v", err)
	}

	// Create database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create ItemTable
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS ItemTable (key TEXT PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert existing config if provided
	if existingConfig != nil {
		configJSON, err := json.Marshal(existingConfig)
		if err != nil {
			t.Fatalf("Failed to marshal config: %v", err)
		}

		_, err = db.Exec("INSERT INTO ItemTable (key, value) VALUES (?, ?)", "kilocode.kilo-code", string(configJSON))
		if err != nil {
			t.Fatalf("Failed to insert config: %v", err)
		}
	}

	return dbPath
}

// loadKiloConfigFromDB loads the Kilo config from a database for testing
func loadKiloConfigFromDB(t *testing.T, dbPath string) map[string]any {
	t.Helper()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	var value string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = 'kilocode.kilo-code'").Scan(&value)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		t.Fatalf("Failed to query config: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	return config
}

func loadSecretBufferFromDB(t *testing.T, dbPath, extensionID, key string) []byte {
	t.Helper()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	secretKey := `secret://{"extensionId":"` + extensionID + `","key":"` + key + `"}`
	var value string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", secretKey).Scan(&value)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		t.Fatalf("Failed to query secret: %v", err)
	}

	var payload struct {
		Type string `json:"type"`
		Data []int  `json:"data"`
	}
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		t.Fatalf("Failed to unmarshal secret payload: %v", err)
	}
	if len(payload.Data) == 0 {
		return nil
	}

	out := make([]byte, len(payload.Data))
	for i, v := range payload.Data {
		out[i] = byte(v)
	}
	return out
}
