//go:build !windows
// +build !windows

package kilo

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// getIDEDBPath returns the path to the VS Code database file for Unix-like systems (macOS/Linux)
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