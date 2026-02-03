//go:build !windows
// +build !windows

package kilo

import (
	"os/exec"
	"runtime"
	"strings"
)

// isIDERunning checks if the IDE is currently running on Unix-like systems
func isIDERunning(processNames []string) bool {
	for _, proc := range processNames {
		cmd := exec.Command("pgrep", "-x", proc)
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

// isIDEInstalled checks if the IDE is installed on Unix-like systems
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

// getIDEVersion gets the version of the IDE on Unix-like systems
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
		cmd := exec.Command("cursor", "--version")
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

// getIDENames returns the display name and a list of process names for the IDE on Unix-like systems
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