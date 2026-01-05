package kilo

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestKiloBuildsWithoutCGO verifies that the kilo package can be built
// with CGO_ENABLED=0, which is required for release builds.
// This test catches the issue reported in https://github.com/costa-app/costa-cli/issues/19
func TestKiloBuildsWithoutCGO(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping build test in short mode")
	}

	// Get the module root
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get module root: %v", err)
	}
	moduleRoot := strings.TrimSpace(string(output))

	// Create a temporary directory for the test build
	tmpDir := t.TempDir()
	testBinary := filepath.Join(tmpDir, "costa-test")
	if runtime.GOOS == "windows" {
		testBinary += ".exe"
	}

	// Attempt to build the entire costa binary with CGO_ENABLED=0
	buildCmd := exec.Command("go", "build", "-o", testBinary, "./cmd/costa")
	buildCmd.Dir = moduleRoot
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err = buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Build with CGO_ENABLED=0 failed (this is the bug we're fixing):\n%s\nError: %v", output, err)
	}

	// Verify the binary was created
	if _, err := os.Stat(testBinary); os.IsNotExist(err) {
		t.Fatalf("Binary was not created at %s", testBinary)
	}

	t.Logf("Successfully built costa with CGO_ENABLED=0")
}

// TestKiloSetupRunsWithoutCGO verifies that the kilo setup command can actually
// execute in a CGO_ENABLED=0 binary. This is a more comprehensive test that ensures
// the runtime behavior works correctly.
func TestKiloSetupRunsWithoutCGO(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if runtime.GOOS != "darwin" {
		t.Skip("Kilo setup only supported on macOS")
	}

	// Get the module root
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get module root: %v", err)
	}
	moduleRoot := strings.TrimSpace(string(output))

	// Create a temporary directory for the test build and mock VS Code
	tmpDir := t.TempDir()
	testBinary := filepath.Join(tmpDir, "costa-test")

	// Create mock VS Code database structure
	vscodeDir := filepath.Join(tmpDir, "Library", "Application Support", "Code", "User", "globalStorage")
	if err := os.MkdirAll(vscodeDir, 0700); err != nil {
		t.Fatalf("Failed to create VS Code directory: %v", err)
	}

	// Create a dummy SQLite database file
	// We can't create a real one without CGO, but we can create the file
	// to get past the existence check
	dbPath := filepath.Join(vscodeDir, "state.vscdb")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0600); err != nil {
		t.Fatalf("Failed to create dummy database file: %v", err)
	}

	// Build the binary with CGO_ENABLED=0
	buildCmd := exec.Command("go", "build", "-o", testBinary, "./cmd/costa")
	buildCmd.Dir = moduleRoot
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Build failed:\n%s\nError: %v", output, err)
	}

	// Try to run the kilo setup command with --format json
	// This should NOT fail with a CGO error
	runCmd := exec.Command(testBinary, "setup", "kilo", "--format", "json", "--token", "test-token")
	runCmd.Env = append(os.Environ(), "HOME="+tmpDir, "COSTA_SAFE_STORAGE_PASSWORD=test-pass")

	output, _ = runCmd.CombinedOutput()
	outputStr := string(output)

	// Check for the specific CGO error that was reported in the issue
	if strings.Contains(outputStr, "go-sqlite3 requires cgo to work") {
		t.Fatalf("Command failed with CGO error (the bug from issue #19):\n%s", outputStr)
	}

	if strings.Contains(outputStr, "Binary was compiled with 'CGO_ENABLED=0'") {
		t.Fatalf("Command failed with CGO stub error (the bug from issue #19):\n%s", outputStr)
	}

	if strings.Contains(strings.ToLower(outputStr), "cgo") {
		t.Fatalf("Command failed with CGO-related error (the bug from issue #19):\n%s", outputStr)
	}

	// The command may fail for other reasons (e.g., invalid database format),
	// but it should not fail with a CGO-related error
	t.Logf("Successfully executed kilo setup with CGO_ENABLED=0 binary (no CGO errors)")
	t.Logf("Output: %s", outputStr)
}
