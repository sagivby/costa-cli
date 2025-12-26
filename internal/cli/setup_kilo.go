package cli

import (
	"bufio"
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
}

func runSetupKilo(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Use a single reader for all prompts to avoid buffering issues
	inputReader := bufio.NewReader(cmd.InOrStdin())

	// Build options (Kilo doesn't use scope, refresh-token-only, require-installed, or statusline)
	opts := integrations.ApplyOpts{
		Scope:         integrations.ScopeUser, // Not used by Kilo but required by interface
		TokenOverride: kiloSetupToken,
		Force:         kiloSetupForce,
		DryRun:        kiloSetupDryRun,
		BackupDir:     kiloSetupBackupDir,
		IDE:           kiloSetupIDE,
	}

	// Create integration
	integration := kilo.New()

	// Get status first to show context
	status, err := integration.Status(ctx, integrations.ScopeUser)
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	// Determine IDE display name
	ideName := "VS Code"
	if kiloSetupIDE == "cursor" {
		ideName = "Cursor"
	} else if kiloSetupIDE == "jetbrains" {
		ideName = "JetBrains"
	}

	// Show detection info
	if status.Installed {
		fmt.Fprintf(cmd.OutOrStdout(), "✓ %s detected: %s\n", ideName, status.Version)
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s not detected\n", ideName)
		installURL := "https://code.visualstudio.com/"
		if kiloSetupIDE == "cursor" {
			installURL = "https://cursor.sh/"
		} else if kiloSetupIDE == "jetbrains" {
			installURL = "https://www.jetbrains.com/"
		}
		return fmt.Errorf("%s not found; install it first: %s", ideName, installURL)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "📁 Database path: %s\n", status.ConfigPath)

	// Phase 1: plan (dry run) to compute changes without writing
	planOpts := opts
	planOpts.DryRun = true
	planResult, err := integration.Apply(ctx, planOpts)
	if err != nil {
		return err
	}

	// Check if already configured
	if !planResult.Changed {
		fmt.Fprintln(cmd.OutOrStdout(), "✓ Already configured! No changes needed.")
		return nil
	}

	// Show planned changes
	fmt.Fprintln(cmd.OutOrStdout(), "\n📝 Changes to apply:")
	for _, change := range planResult.UpdatedKeys {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", change)
	}

	// Honor --dry-run (show but do not write)
	if kiloSetupDryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "\n🔍 Dry run - no changes made")
		return nil
	}

	// Confirm if not --force
	if !kiloSetupForce {
		fmt.Fprint(cmd.OutOrStdout(), "\nProceed with changes? [Y/n]: ")
		response, _ := inputReader.ReadString('\n')
		resp := strings.ToLower(strings.TrimSpace(response))
		if resp == "n" || resp == "no" { // default YES
			fmt.Fprintln(cmd.OutOrStdout(), "Canceled.")
			return nil
		}
	}

	// Phase 2: write (actual apply)
	writeOpts := opts
	writeOpts.DryRun = false
	result, err := integration.Apply(ctx, writeOpts)
	if err != nil {
		return err
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
