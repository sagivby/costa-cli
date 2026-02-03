//go:build windows
// +build windows

package kilo

import (
	"testing"
)

// TestWindowsBuild verifies that the Windows-specific code compiles correctly
func TestWindowsBuild(t *testing.T) {
	// This test just verifies that the Windows-specific functions can be called
	// without causing compilation errors

	// Test that getIDENames is available
	displayName, processNames := getIDENames("vscode")
	if displayName == "" || len(processNames) == 0 {
		// This is just to ensure the function exists and can be called
		// We don't care about the actual values in this compile test
	}

	// Test that getIDEDBPath is available
	_, err := getIDEDBPath("vscode")
	if err != nil {
		// Again, we just want to ensure the function exists
	}

	// Test that isIDEInstalled is available
	_ = isIDEInstalled("vscode")

	// Test that isIDERunning is available
	_ = isIDERunning([]string{"Code"})

	// Test that getIDEVersion is available
	_ = getIDEVersion("vscode")

	// Test that encryptWithSafeStorage is available
	_, err = encryptWithSafeStorage("test")
	if err != nil {
		// Again, we just want to ensure the function exists
	}
}