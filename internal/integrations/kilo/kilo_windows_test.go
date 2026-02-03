//go:build windows
// +build windows

package kilo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetIDEDBPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Test only runs on Windows")
	}

	// Test with APPDATA environment variable
	appData := os.Getenv("APPDATA")
	if appData == "" {
		t.Skip("APPDATA environment variable not set")
	}

	// Test VS Code path
	path, err := getIDEDBPath("vscode")
	if err != nil {
		t.Fatalf("Failed to get IDE DB path: %v", err)
	}

	expected := filepath.Join(appData, "Code", "User", "globalStorage", "state.vscdb")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetIDEDBPath_WindowsUnsupportedIDE(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Test only runs on Windows")
	}

	// Test unsupported IDE
	_, err := getIDEDBPath("unsupported")
	if err == nil {
		t.Error("Expected error for unsupported IDE, got nil")
	}
}

func TestEncryptWithSafeStorage_EnvOverride(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Test only runs on Windows")
	}

	// Test with environment variable override
	os.Setenv("COSTA_SAFE_STORAGE_PASSWORD", "test-password")
	defer os.Unsetenv("COSTA_SAFE_STORAGE_PASSWORD")

	plaintext := "test-api-key"
	encrypted, err := encryptWithSafeStorage(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt with safe storage: %v", err)
	}

	// Check that it starts with "v10" prefix
	if len(encrypted) < 3 || string(encrypted[:3]) != "v10" {
		t.Errorf("Expected encrypted data to start with 'v10', got %v", string(encrypted[:3]))
	}
}