package kilo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetIDEDBPath_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	// Test with default XDG_CONFIG_HOME
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home dir: %v", err)
	}

	// Test VS Code path
	path, err := getIDEDBPath("vscode")
	if err != nil {
		t.Fatalf("Failed to get IDE DB path: %v", err)
	}

	expected := filepath.Join(home, ".config", "Code", "User", "globalStorage", "state.vscdb")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}

	// Test with custom XDG_CONFIG_HOME
	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	defer os.Unsetenv("XDG_CONFIG_HOME")

	path, err = getIDEDBPath("vscode")
	if err != nil {
		t.Fatalf("Failed to get IDE DB path with custom XDG_CONFIG_HOME: %v", err)
	}

	expected = filepath.Join("/custom/config", "Code", "User", "globalStorage", "state.vscdb")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetIDEDBPath_LinuxUnsupportedIDE(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	// Test unsupported IDE
	_, err := getIDEDBPath("unsupported")
	if err == nil {
		t.Error("Expected error for unsupported IDE, got nil")
	}
}
