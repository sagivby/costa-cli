//go:build windows
// +build windows

package kilo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// getIDEDBPath returns the path to the VS Code database file for Windows
func getIDEDBPath(ide string) (string, error) {
	// Get the AppData path
	appData := os.Getenv("APPDATA")
	if appData == "" {
		// Fallback to USERPROFILE\AppData\Roaming if APPDATA is not set
		userProfile := os.Getenv("USERPROFILE")
		if userProfile == "" {
			return "", fmt.Errorf("neither APPDATA nor USERPROFILE environment variables are set")
		}
		appData = filepath.Join(userProfile, "AppData", "Roaming")
	}

	switch ide {
	case "vscode":
		// Check for different VS Code variants
		variants := []string{
			"Code",           // VS Code Stable
			"Code - Insiders", // VS Code Insiders
			"VSCodium",        // VSCodium
		}

		// Try each variant in order
		for _, variant := range variants {
			path := filepath.Join(appData, variant, "User", "globalStorage", "state.vscdb")
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}

		// If none exist, return the default path (VS Code Stable)
		return filepath.Join(appData, "Code", "User", "globalStorage", "state.vscdb"), nil
	case "cursor":
		return filepath.Join(appData, "Cursor", "User", "globalStorage", "state.vscdb"), nil
	case "jetbrains":
		// JetBrains uses different config structure - will need to be implemented
		return "", fmt.Errorf("JetBrains configuration path not yet implemented")
	default:
		return "", fmt.Errorf("unsupported IDE: %s", ide)
	}
}

// getIDENames returns the display name and a list of process names for the IDE on Windows
func getIDENames(ide string) (displayName string, processNames []string) {
	switch ide {
	case "vscode":
		return "VS Code", []string{"Code", "code", "Code.exe"}
	case "cursor":
		return "Cursor", []string{"cursor", "Cursor.exe"}
	case "jetbrains":
		// This will need refinement for different JetBrains IDEs
		return "JetBrains", []string{"idea", "pycharm", "webstorm", "goland"}
	default:
		return "Unknown", []string{"unknown"}
	}
}

// getIDELocalStatePath returns the path to the VS Code Local State file for Windows
func getIDELocalStatePath(ide string) (string, error) {
	// Get the AppData path
	appData := os.Getenv("APPDATA")
	if appData == "" {
		// Fallback to USERPROFILE\AppData\Roaming if APPDATA is not set
		userProfile := os.Getenv("USERPROFILE")
		if userProfile == "" {
			return "", fmt.Errorf("neither APPDATA nor USERPROFILE environment variables are set")
		}
		appData = filepath.Join(userProfile, "AppData", "Roaming")
	}

	switch ide {
	case "vscode":
		return filepath.Join(appData, "Code", "Local State"), nil
	case "cursor":
		return filepath.Join(appData, "Cursor", "Local State"), nil
	default:
		return "", fmt.Errorf("unsupported IDE for Local State: %s", ide)
	}
}

// isIDEInstalled checks if the IDE is installed on Windows
func isIDEInstalled(ide string) bool {
	switch ide {
	case "vscode":
		// Check PATH first
		candidates := []string{"code", "code-oss", "codium", "vscodium"}
		for _, cmd := range candidates {
			if _, err := exec.LookPath(cmd); err == nil {
				return true
			}
		}

		// Check common installation locations
		locations := []string{
			// User installs
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "Code.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code Insiders", "Code - Insiders.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "VSCodium", "VSCodium.exe"),
			// System-wide installs
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code", "Code.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft VS Code", "Code.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code Insiders", "Code - Insiders.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "VSCodium", "VSCodium.exe"),
		}

		for _, location := range locations {
			if location != "" {
				if _, err := os.Stat(location); err == nil {
					return true
				}
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