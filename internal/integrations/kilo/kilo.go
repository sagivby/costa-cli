package kilo

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	// Import sqlite3 driver for database/sql (pure-Go version that works without CGO)
	_ "modernc.org/sqlite"

	"github.com/zalando/go-keyring"

	"github.com/costa-app/costa-cli/internal/auth"
	"github.com/costa-app/costa-cli/internal/debug"
	"github.com/costa-app/costa-cli/internal/integrations"
)

// Kilo implements the Integration interface for Kilo Code (VS Code extension)
type Kilo struct{}

// New creates a new Kilo integration
func New() *Kilo {
	return &Kilo{}
}

// Name returns the name of the integration
func (k *Kilo) Name() string {
	return "kilo"
}

// Apply applies the Kilo configuration
func (k *Kilo) Apply(ctx context.Context, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	result := integrations.ApplyResult{}

	// Support macOS and Linux
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return result, fmt.Errorf("Kilo setup is currently only supported on macOS and Linux")
	}

	// Default to vscode if not specified
	ide := opts.IDE
	if ide == "" {
		ide = "vscode"
	}

	// Validate IDE and check if it's supported yet
	if err := validateIDE(ide); err != nil {
		return result, err
	}

	// Check if IDE is installed
	ideName, processNames := getIDENames(ide)
	if !isIDEInstalled(ide) {
		return result, fmt.Errorf("%s not found. Please install %s first", ideName, ideName)
	}

	// Check if IDE is running
	if isIDERunning(processNames) {
		return result, fmt.Errorf("%s is running. Please close %s before running this command", ideName, ideName)
	}

	// Get IDE database path
	dbPath, err := getIDEDBPath(ide)
	if err != nil {
		return result, fmt.Errorf("failed to locate %s database: %w", ideName, err)
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return result, fmt.Errorf("%s database not found at %s. Make sure Kilo extension is installed", ideName, dbPath)
	}

	result.ConfigPath = dbPath

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

	// Load existing config
	existing, err := loadKiloConfig(dbPath)
	if err != nil {
		return result, fmt.Errorf("failed to load Kilo config: %w", err)
	}

	// Build desired config
	baseURL := auth.GetBaseURL() + "/api/v1"
	modelID := "costa/auto"

	debug.Printf("DEBUG: COSTA_BASE_URL env var = %q\n", os.Getenv("COSTA_BASE_URL"))
	debug.Printf("DEBUG: Resolved base URL = %q\n", baseURL)
	debug.Printf("DEBUG: Model ID = %q\n", modelID)

	// Determine what needs to be updated
	var updatedKeys []string
	var unchangedKeys []string

	configExists := len(existing) > 0

	if configExists {
		// Update mode
		if existing["openAiBaseUrl"] == baseURL {
			unchangedKeys = append(unchangedKeys, "openAiBaseUrl")
		} else {
			updatedKeys = append(updatedKeys, "openAiBaseUrl")
		}
		if existing["openAiModelId"] == modelID {
			unchangedKeys = append(unchangedKeys, "openAiModelId")
		} else {
			updatedKeys = append(updatedKeys, "openAiModelId")
		}
		updatedKeys = append(updatedKeys, "openAiApiKey")
	} else {
		// Insert mode - all keys are new
		updatedKeys = append(updatedKeys, "all Kilo configuration keys", "openAiApiKey")
	}

	result.UpdatedKeys = updatedKeys
	result.UnchangedKeys = unchangedKeys
	result.Changed = len(updatedKeys) > 0

	// If no changes and not dry run, we're done
	if !result.Changed {
		return result, nil
	}

	// Dry run stops here
	if opts.DryRun {
		return result, nil
	}

	// Create backup
	backupPath, err := createBackup(dbPath, opts.BackupDir)
	if err != nil {
		return result, fmt.Errorf("failed to create backup: %w", err)
	}
	result.BackupPath = backupPath

	// Apply configuration
	if err := applyKiloConfig(dbPath, baseURL, modelID, configExists, existing); err != nil {
		return result, fmt.Errorf("failed to apply configuration: %w", err)
	}

	if err := setKiloAPIKeyInDB(dbPath, token); err != nil {
		return result, err
	}

	return result, nil
}

// Status returns the current status of Kilo configuration
func (k *Kilo) Status(ctx context.Context, scope integrations.Scope) (integrations.StatusResult, error) {
	result := integrations.StatusResult{
		Scope: scope,
	}

	// Support macOS and Linux
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return result, fmt.Errorf("Kilo setup is currently only supported on macOS and Linux")
	}

	// Default to vscode for status checks
	ide := "vscode"

	// Check if IDE is installed
	result.Installed = isIDEInstalled(ide)
	if result.Installed {
		result.Version = getIDEVersion(ide)
	}

	// Get IDE database path
	dbPath, err := getIDEDBPath(ide)
	if err != nil {
		return result, fmt.Errorf("failed to locate VS Code database: %w", err)
	}
	result.ConfigPath = dbPath

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		result.ConfigExists = false
		return result, nil
	}

	result.ConfigExists = true

	// Load existing config
	existing, err := loadKiloConfig(dbPath)
	if err != nil {
		return result, fmt.Errorf("failed to load Kilo config: %w", err)
	}

	// Check if configured for Costa
	isCosta, missing := checkCostaConfig(existing)
	result.IsCosta = isCosta
	result.Missing = missing

	// Extract current model
	if model, ok := existing["openAiModelId"].(string); ok {
		result.Model = model
	}

	return result, nil
}

// Helper functions

// validateIDE checks if the IDE is valid and supported
func validateIDE(ide string) error {
	validIDEs := []string{"vscode", "cursor", "jetbrains"}

	// Check if IDE is valid
	isValid := false
	for _, valid := range validIDEs {
		if ide == valid {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("invalid IDE: %s. Supported values: vscode, cursor, jetbrains", ide)
	}

	// Only vscode is supported for now
	if ide != "vscode" {
		return fmt.Errorf("IDE '%s' support is coming soon. Currently only 'vscode' is supported", ide)
	}

	return nil
}

// getIDENames returns the display name and a list of process names for the IDE
func getIDENames(ide string) (displayName string, processNames []string) {
	switch ide {
	case "vscode":
		if runtime.GOOS == "darwin" {
			return "VS Code", []string{"Code"}
		}
		return "VS Code", []string{"code", "code-oss", "codium", "vscodium"}
	case "cursor":
		return "Cursor", []string{"cursor"}
	case "jetbrains":
		// This will need refinement for different JetBrains IDEs
		return "JetBrains", []string{"idea", "pycharm", "webstorm", "goland"}
	default:
		return "Unknown", []string{"unknown"}
	}
}

func isIDEInstalled(ide string) bool {
	switch ide {
	case "vscode":
		candidates := []string{"code", "code-oss", "codium", "vscodium"}
		for _, cmd := range candidates {
			if _, err := exec.LookPath(cmd); err == nil {
				return true
			}
		}
		return false
	case "cursor":
		_, err := exec.LookPath("cursor")
		return err == nil
	case "jetbrains":
		// Check for common JetBrains IDEs
		for _, cmd := range []string{"idea", "pycharm", "webstorm", "goland"} {
			if _, err := exec.LookPath(cmd); err == nil {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func isIDERunning(processNames []string) bool {
	for _, proc := range processNames {
		cmd := exec.Command("pgrep", "-x", proc) // #nosec G204
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

func getIDEVersion(ide string) string {
	switch ide {
	case "vscode":
		candidates := []string{"code", "code-oss", "codium", "vscodium"}
		for _, cmdName := range candidates {
			if _, err := exec.LookPath(cmdName); err != nil {
				continue
			}
			cmd := exec.Command(cmdName, "--version") // #nosec G204
			output, err := cmd.Output()
			if err != nil {
				return "unknown"
			}
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			if len(lines) > 0 {
				return lines[0]
			}
			return "unknown"
		}
		return "unknown"
	case "cursor":
		cmd := exec.Command("cursor", "--version") // #nosec G204
		output, err := cmd.Output()
		if err != nil {
			return "unknown"
		}
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) > 0 {
			return lines[0]
		}
		return "unknown"
	case "jetbrains":
		// JetBrains version detection is more complex, return generic for now
		return "JetBrains IDE"
	default:
		return "unknown"
	}
}

func getIDEDBPath(ide string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		switch ide {
		case "vscode":
			return filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "state.vscdb"), nil
		case "cursor":
			return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb"), nil
		case "jetbrains":
			// JetBrains uses different config structure - will need to be implemented
			return "", fmt.Errorf("JetBrains configuration path not yet implemented")
		default:
			return "", fmt.Errorf("unsupported IDE: %s", ide)
		}
	case "linux":
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}

		switch ide {
		case "vscode":
			return filepath.Join(configHome, "Code", "User", "globalStorage", "state.vscdb"), nil
		case "cursor":
			return filepath.Join(configHome, "Cursor", "User", "globalStorage", "state.vscdb"), nil
		case "vscodium":
			return filepath.Join(configHome, "VSCodium", "User", "globalStorage", "state.vscdb"), nil
		case "jetbrains":
			return "", fmt.Errorf("JetBrains configuration path not yet implemented")
		default:
			return "", fmt.Errorf("unsupported IDE: %s", ide)
		}
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func loadKiloConfig(dbPath string) (map[string]any, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = db.Close()
	}()

	var value string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = 'kilocode.kilo-code'").Scan(&value)
	if err == sql.ErrNoRows {
		return nil, nil // No config exists yet
	}
	if err != nil {
		return nil, err
	}

	// Parse JSON
	var config map[string]any
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return nil, err
	}

	return config, nil
}

func applyKiloConfig(dbPath, baseURL, modelID string, configExists bool, existing map[string]any) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	var configJSON string

	if configExists && existing != nil {
		// UPDATE existing config
		existing["openAiBaseUrl"] = baseURL
		existing["openAiModelId"] = modelID

		// Update listApiConfigMeta if it exists
		if listMeta, ok := existing["listApiConfigMeta"].([]any); ok {
			for _, meta := range listMeta {
				if metaMap, ok := meta.(map[string]any); ok {
					if metaMap["id"] == existing["id"] {
						metaMap["modelId"] = modelID
					}
				}
			}
		}

		jsonData, err := json.Marshal(existing)
		if err != nil {
			return err
		}
		configJSON = string(jsonData)

		_, err = db.Exec("UPDATE ItemTable SET value = ? WHERE key = ?", configJSON, "kilocode.kilo-code")
		if err != nil {
			return err
		}
	} else {
		// INSERT new config
		newConfig := map[string]any{
			"allowedCommands":                   []string{"git log", "git diff", "git show"},
			"telemetrySetting":                  "enabled",
			"firstInstallCompleted":             true,
			"customModes":                       []any{},
			"globalWorkflowToggles":             map[string]any{},
			"listApiConfigMeta":                 []any{map[string]any{"name": "default", "id": "costa_default", "apiProvider": "openai", "modelId": modelID}},
			"defaultCommandsMigrationCompleted": true,
			"currentApiConfigName":              "default",
			"apiProvider":                       "openai",
			"reasoningEffort":                   "medium",
			"openAiHeaders":                     map[string]any{},
			"openAiBaseUrl":                     baseURL,
			"openAiModelId":                     modelID,
			"openAiCustomModelInfo": map[string]any{
				"maxTokens":           -1,
				"contextWindow":       128000,
				"supportsImages":      true,
				"supportsPromptCache": false,
				"inputPrice":          0,
				"outputPrice":         0,
			},
			"openAiStreamingEnabled": true,
			"taskHistory":            []any{},
			"language":               "en",
			"alwaysAllowReadOnly":    true,
			"alwaysAllowWrite":       true,
			"alwaysAllowExecute":     true,
			"alwaysAllowBrowser":     true,
			"alwaysAllowMcp":         true,
			"deniedCommands":         []any{},
			"mode":                   "code",
			"id":                     "costa_default",
		}

		jsonData, err := json.Marshal(newConfig)
		if err != nil {
			return err
		}
		configJSON = string(jsonData)

		// Check if key exists
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM ItemTable WHERE key = ?", "kilocode.kilo-code").Scan(&count)
		if err != nil {
			return err
		}

		if count > 0 {
			_, err = db.Exec("UPDATE ItemTable SET value = ? WHERE key = ?", configJSON, "kilocode.kilo-code")
		} else {
			_, err = db.Exec("INSERT INTO ItemTable (key, value) VALUES (?, ?)", "kilocode.kilo-code", configJSON)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func setKiloAPIKeyInDB(dbPath, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("missing Costa API key")
	}

	// Support macOS and Linux
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("Kilo API key setup is currently only supported on macOS and Linux")
	}

	encrypted, err := encryptWithSafeStorage(apiKey)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"type": "Buffer",
		"data": bytesToInts(encrypted),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode Kilo API key payload: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	key := `secret://{"extensionId":"kilocode.kilo-code","key":"openAiApiKey"}`
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ItemTable WHERE key = ?", key).Scan(&count); err != nil {
		return err
	}

	if count > 0 {
		_, err = db.Exec("UPDATE ItemTable SET value = ? WHERE key = ?", string(payloadJSON), key)
	} else {
		_, err = db.Exec("INSERT INTO ItemTable (key, value) VALUES (?, ?)", key, string(payloadJSON))
	}
	if err != nil {
		return err
	}

	return nil
}

func encryptWithSafeStorage(plaintext string) ([]byte, error) {
	password, err := getSafeStoragePassword()
	if err != nil {
		return nil, err
	}

	key := pbkdf2SHA1([]byte(password), []byte("saltysalt"), 1003, 16)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	iv := bytes.Repeat([]byte(" "), aes.BlockSize)
	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ciphertext := make([]byte, len(padded))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	prefix := []byte("v10")
	return append(prefix, ciphertext...), nil
}

func getSafeStoragePassword() (string, error) {
	// Allow tests/CI to override via environment variable
	if v := os.Getenv("COSTA_SAFE_STORAGE_PASSWORD"); v != "" {
		return v, nil
	}

	if runtime.GOOS == "darwin" {
		return getMacSafeStoragePassword()
	}
	if runtime.GOOS == "linux" {
		return getLinuxSafeStoragePassword()
	}
	return "", fmt.Errorf("unsupported platform for safe storage: %s", runtime.GOOS)
}

func getLinuxSafeStoragePassword() (string, error) {
	// Allow tests/CI to override via environment variable
	if v := os.Getenv("COSTA_SAFE_STORAGE_PASSWORD"); v != "" {
		return v, nil
	}

	services := []string{
		"Code Safe Storage",
		"Visual Studio Code Safe Storage",
		"VS Code Safe Storage",
		"Electron Safe Storage",
		"Chrome Safe Storage",
	}

	currentUser, _ := user.Current()
	accounts := []string{""}
	if currentUser != nil {
		accounts = append(accounts, currentUser.Username)
	}

	for _, service := range services {
		for _, account := range accounts {
			password, err := keyring.Get(service, account)
			if err == nil && strings.TrimSpace(password) != "" {
				return strings.TrimSpace(password), nil
			}
		}
	}

	return "", fmt.Errorf("could not find VS Code safe storage key in keyring (tried common service names)")
}

func getMacSafeStoragePassword() (string, error) {
	// Allow tests/CI to override via environment variable
	if v := os.Getenv("COSTA_SAFE_STORAGE_PASSWORD"); v != "" {
		return v, nil
	}

	services := []string{
		"Code Safe Storage",
		"Visual Studio Code Safe Storage",
		"VS Code Safe Storage",
		"Microsoft VS Code Safe Storage",
		"com.microsoft.VSCode Safe Storage",
		"Electron Safe Storage",
		"Chrome Safe Storage",
		"Chromium Safe Storage",
		"Code - OSS Safe Storage",
	}

	for _, service := range services {
		// #nosec G204 -- service is selected from a fixed allowlist above.
		out, err := exec.Command("security", "find-generic-password", "-s", service, "-w").Output() // #nosec G204
		if err != nil {
			continue
		}
		password := strings.TrimSpace(string(out))
		if password != "" {
			return password, nil
		}
	}

	return "", fmt.Errorf("could not find VS Code safe storage key in Keychain (tried common service names)")
}

func pbkdf2SHA1(password, salt []byte, iter, keyLen int) []byte {
	hashLen := sha1.Size
	numBlocks := (keyLen + hashLen - 1) / hashLen
	var out []byte
	for block := 1; block <= numBlocks; block++ {
		u := pbkdf2Block(password, salt, iter, block)
		out = append(out, u...)
	}
	return out[:keyLen]
}

func pbkdf2Block(password, salt []byte, iter, block int) []byte {
	var u []byte
	mac := hmac.New(sha1.New, password)
	mac.Write(salt)
	mac.Write(intToBigEndian(block))
	u = mac.Sum(nil)

	out := make([]byte, len(u))
	copy(out, u)

	for i := 1; i < iter; i++ {
		mac = hmac.New(sha1.New, password)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}

	return out
}

func intToBigEndian(i int) []byte {
	return []byte{
		byte(i >> 24),
		byte(i >> 16),
		byte(i >> 8),
		byte(i),
	}
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - (len(data) % blockSize)
	if padLen == 0 {
		padLen = blockSize
	}
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}

func bytesToInts(data []byte) []int {
	out := make([]int, len(data))
	for i, b := range data {
		out[i] = int(b)
	}
	return out
}

func checkCostaConfig(config map[string]any) (bool, []string) {
	if len(config) == 0 {
		return false, []string{"config not found"}
	}

	var missing []string
	baseURL := auth.GetBaseURL() + "/api/v1"

	// Check base URL
	if url, ok := config["openAiBaseUrl"].(string); !ok || !strings.HasPrefix(url, auth.GetBaseURL()) {
		missing = append(missing, "openAiBaseUrl")
	}

	// Check model ID
	if model, ok := config["openAiModelId"].(string); !ok || !strings.HasPrefix(model, "costa/") {
		missing = append(missing, "openAiModelId")
	}

	// Check API provider
	if provider, ok := config["apiProvider"].(string); !ok || provider != "openai" {
		missing = append(missing, "apiProvider")
	}

	_ = baseURL // suppress unused warning

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
		backupDir = filepath.Join(configDir, "backups", "kilo")
	}

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("state-%s.vscdb", timestamp))

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
