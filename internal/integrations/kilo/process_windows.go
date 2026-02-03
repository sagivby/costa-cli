//go:build windows
// +build windows

package kilo

import (
	"os/exec"
	"strings"
)

// isIDERunning checks if the IDE is currently running on Windows
func isIDERunning(processNames []string) bool {
	// Try tasklist first
	for _, proc := range processNames {
		// Map process names to Windows executable names
		var exeName string
		switch proc {
		case "Code":
			exeName = "Code.exe"
		case "code":
			exeName = "Code.exe"
		case "code-oss":
			exeName = "code-oss.exe"
		case "codium":
			exeName = "VSCodium.exe"
		case "vscodium":
			exeName = "VSCodium.exe"
		case "cursor":
			exeName = "Cursor.exe"
		default:
			exeName = proc
			// Add .exe extension if not present
			if !strings.HasSuffix(exeName, ".exe") {
				exeName += ".exe"
			}
		}

		// Use tasklist to check if process is running
		cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq "+exeName)
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), exeName) {
			return true
		}
	}

	// Fallback: try PowerShell if tasklist fails
	for _, proc := range processNames {
		var exeName string
		switch proc {
		case "Code":
			exeName = "Code.exe"
		case "code":
			exeName = "Code.exe"
		case "code-oss":
			exeName = "code-oss.exe"
		case "codium":
			exeName = "VSCodium.exe"
		case "vscodium":
			exeName = "VSCodium.exe"
		case "cursor":
			exeName = "Cursor.exe"
		default:
			exeName = proc
			if !strings.HasSuffix(exeName, ".exe") {
				exeName += ".exe"
			}
		}

		// Use PowerShell to check if process is running
		cmd := exec.Command("powershell", "-Command", "Get-Process -Name '"+strings.TrimSuffix(exeName, ".exe")+"' -ErrorAction SilentlyContinue")
		if cmd.Run() == nil {
			return true
		}
	}

	return false
}

// getIDEVersion gets the version of the IDE on Windows
func getIDEVersion(ide string) string {
	switch ide {
	case "vscode":
		candidates := []string{"code", "code-oss", "codium", "vscodium"}
		for _, cmdName := range candidates {
			if _, err := exec.LookPath(cmdName); err != nil {
				continue
			}
			cmd := exec.Command(cmdName, "--version")
			output, err := cmd.Output()
			if err != nil {
				// Try with .exe extension
				cmd := exec.Command(cmdName+".exe", "--version")
				output, err = cmd.Output()
				if err != nil {
					continue
				}
			}
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			if len(lines) > 0 {
				return lines[0]
			}
		}
		return "unknown"
	case "cursor":
		cmd := exec.Command("cursor", "--version")
		output, err := cmd.Output()
		if err != nil {
			cmd := exec.Command("cursor.exe", "--version")
			output, err = cmd.Output()
			if err != nil {
				return "unknown"
			}
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