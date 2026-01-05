package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/costa-app/costa-cli/internal/integrations"
	"github.com/costa-app/costa-cli/internal/integrations/kilo"
)

var (
	kiloSetupToken     string
	kiloSetupForce     bool
	kiloSetupDryRun    bool
	kiloSetupBackupDir string
	kiloSetupIDE       string
	kiloSetupFormat    string
)

var setupKiloCmd = &cobra.Command{
	Use:     "kilo",
	Aliases: []string{"kilo-code"},
	Short:   "Setup Kilo to use Costa",
	Long:    `Configure Kilo (VS Code extension) to use Costa's API and token.`,
	RunE:    runSetupKilo,
}

func init() {
	setupKiloCmd.Flags().StringVar(&kiloSetupToken, "token", "", "Use explicit token instead of fetching from Costa")
	setupKiloCmd.Flags().BoolVar(&kiloSetupForce, "force", false, "Skip confirmation prompt (auto-yes)")
	setupKiloCmd.Flags().BoolVar(&kiloSetupDryRun, "dry-run", false, "Show what would change without writing")
	setupKiloCmd.Flags().StringVar(&kiloSetupBackupDir, "backup-dir", "", "Custom backup directory")
	setupKiloCmd.Flags().StringVar(&kiloSetupIDE, "ide", "vscode", "IDE to configure (vscode, cursor, jetbrains)")
	setupKiloCmd.Flags().StringVar(&kiloSetupFormat, "format", "", "Output format (json)")
}

func runSetupKilo(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	isJSONMode := kiloSetupFormat == "json"

	// Use a single reader for all prompts to avoid buffering issues
	inputReader := bufio.NewReader(cmd.InOrStdin())

	// Build options (Kilo doesn't use scope, refresh-token-only, require-installed, or statusline)
	opts := integrations.ApplyOpts{
		Scope:         integrations.ScopeUser, // Not used by Kilo but required by interface
		TokenOverride: kiloSetupToken,
		Force:         kiloSetupForce || isJSONMode,
		DryRun:        kiloSetupDryRun,
		BackupDir:     kiloSetupBackupDir,
		IDE:           kiloSetupIDE,
	}

	// Create integration
	integration := kilo.New()

	// Get status first to show context
	status, err := integration.Status(ctx, integrations.ScopeUser)
	if err != nil {
		return handleKiloError(cmd, isJSONMode, "failed to check status: %w", err)
	}

	// Show detection info
	if err := showKiloDetectionInfo(cmd, status, isJSONMode); err != nil {
		return err
	}

	// Phase 1: plan (dry run) to compute changes without writing
	planResult, err := planKiloChanges(ctx, integration, opts)
	if err != nil {
		return handleKiloError(cmd, isJSONMode, "%w", err)
	}

	// Check if already configured
	if !planResult.Changed {
		return handleKiloNoChanges(cmd, status, isJSONMode)
	}

	// Show planned changes
	showKiloPlannedChanges(cmd, planResult, isJSONMode)

	// Honor --dry-run (show but do not write)
	if kiloSetupDryRun {
		return handleKiloDryRun(cmd, planResult, status, isJSONMode)
	}

	// Confirm if not --force
	if !confirmKiloChanges(cmd, inputReader, isJSONMode) {
		return nil
	}

	// Inform user about keychain access
	if !isJSONMode {
		fmt.Fprintln(cmd.OutOrStdout(), "\nSetting up secure storage for your API key...")
		fmt.Fprintln(cmd.OutOrStdout(), "macOS will ask for Keychain access so Costa can safely encrypt and store your credentials.")
	}

	// Phase 2: write (actual apply)
	result, err := applyKiloChanges(ctx, integration, opts)
	if err != nil {
		return handleKiloError(cmd, isJSONMode, "%w", err)
	}

	// Output results
	return outputKiloResults(cmd, result, status, isJSONMode)
}

func showKiloDetectionInfo(cmd *cobra.Command, status integrations.StatusResult, isJSONMode bool) error {
	// Determine IDE display name
	ideName := "VS Code"
	installURL := "https://code.visualstudio.com/"
	if kiloSetupIDE == "cursor" {
		ideName = "Cursor"
		installURL = "https://cursor.sh/"
	} else if kiloSetupIDE == "jetbrains" {
		ideName = "JetBrains"
		installURL = "https://www.jetbrains.com/"
	}

	if !isJSONMode {
		if status.Installed {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s detected: %s\n", ideName, status.Version)
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s not detected\n", ideName)
			return fmt.Errorf("%s not found; install it first: %s", ideName, installURL)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "📁 Database path: %s\n", status.ConfigPath)
	} else if !status.Installed {
		return outputKiloJSON(cmd, "error", map[string]any{"error": fmt.Sprintf("%s not found", ideName)})
	}
	return nil
}

func planKiloChanges(ctx context.Context, integration *kilo.Kilo, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	planOpts := opts
	planOpts.DryRun = true
	return integration.Apply(ctx, planOpts)
}

func handleKiloNoChanges(cmd *cobra.Command, status integrations.StatusResult, isJSONMode bool) error {
	if isJSONMode {
		return outputKiloJSON(cmd, "success", map[string]any{
			"message":     "Already configured",
			"changed":     false,
			"config_path": status.ConfigPath,
		})
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✓ Already configured! No changes needed.")
	return nil
}

func showKiloPlannedChanges(cmd *cobra.Command, planResult integrations.ApplyResult, isJSONMode bool) {
	if !isJSONMode {
		fmt.Fprintln(cmd.OutOrStdout(), "\n📝 Changes to apply:")
		for _, change := range planResult.UpdatedKeys {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", change)
		}
	}
}

func handleKiloDryRun(cmd *cobra.Command, planResult integrations.ApplyResult, status integrations.StatusResult, isJSONMode bool) error {
	if isJSONMode {
		return outputKiloJSON(cmd, "success", map[string]any{
			"message":      "Dry run completed",
			"changed":      true,
			"updated_keys": planResult.UpdatedKeys,
			"config_path":  status.ConfigPath,
		})
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\n🔍 Dry run - no changes made")
	return nil
}

func confirmKiloChanges(cmd *cobra.Command, inputReader *bufio.Reader, isJSONMode bool) bool {
	if kiloSetupForce || isJSONMode {
		return true
	}
	fmt.Fprint(cmd.OutOrStdout(), "\nProceed with changes? [Y/n]: ")
	response, _ := inputReader.ReadString('\n')
	resp := strings.ToLower(strings.TrimSpace(response))

	if resp == "n" || resp == "no" {
		fmt.Fprintln(cmd.OutOrStdout(), "Canceled.")
		return false
	}
	return true
}

func applyKiloChanges(ctx context.Context, integration *kilo.Kilo, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	writeOpts := opts
	writeOpts.DryRun = false
	return integration.Apply(ctx, writeOpts)
}

func outputKiloResults(cmd *cobra.Command, result integrations.ApplyResult, status integrations.StatusResult, isJSONMode bool) error {
	if isJSONMode {
		data := map[string]any{
			"message":      "Successfully configured",
			"changed":      true,
			"updated_keys": result.UpdatedKeys,
			"config_path":  status.ConfigPath,
		}
		if result.BackupPath != "" {
			data["backup_path"] = result.BackupPath
		}
		if len(result.Warnings) > 0 {
			data["warnings"] = result.Warnings
		}
		return outputKiloJSON(cmd, "success", data)
	}

	if result.BackupPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "💾 Backup created: %s\n", result.BackupPath)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✅ Successfully configured Kilo for Costa!")

	// Show warnings (e.g., API key instructions)
	for _, warning := range result.Warnings {
		fmt.Fprintf(cmd.OutOrStdout(), "\n⚠️  %s\n", warning)
	}

	return nil
}

func handleKiloError(cmd *cobra.Command, isJSONMode bool, format string, err error) error {
	if isJSONMode {
		return outputKiloJSON(cmd, "error", map[string]any{"error": err.Error()})
	}
	return fmt.Errorf(format, err)
}

// outputKiloJSON outputs a JSON response with status and data for kilo setup
func outputKiloJSON(cmd *cobra.Command, status string, data map[string]any) error {
	output := map[string]any{
		"status": status,
		"data":   data,
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
