package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/costa-app/costa-cli/internal/integrations"
	"github.com/costa-app/costa-cli/internal/integrations/opencode"
)

var (
	ocSetupUser             bool
	ocSetupProject          bool
	ocSetupToken            string
	ocSetupForce            bool
	ocSetupDryRun           bool
	ocSetupBackupDir        string
	ocSetupRefreshTokenOnly bool
	ocSetupRequireInstalled bool
	ocSetupFormat           string
)

var setupOpenCodeCmd = &cobra.Command{
	Use:     "opencode",
	Aliases: []string{"open-code"},
	Short:   "Setup OpenCode to use Costa",
	Long:    `Configure OpenCode to use Costa's API and token.`,
	RunE:    runSetupOpenCode,
}

func init() {
	setupOpenCodeCmd.Flags().BoolVar(&ocSetupUser, "user", false, "Setup for current user (default)")
	setupOpenCodeCmd.Flags().BoolVar(&ocSetupProject, "project", false, "Setup for current project")
	setupOpenCodeCmd.Flags().StringVar(&ocSetupToken, "token", "", "Use explicit token instead of fetching from Costa")
	setupOpenCodeCmd.Flags().BoolVar(&ocSetupForce, "force", false, "Skip confirmation prompt (auto-yes)")
	setupOpenCodeCmd.Flags().BoolVar(&ocSetupDryRun, "dry-run", false, "Show what would change without writing")
	setupOpenCodeCmd.Flags().StringVar(&ocSetupBackupDir, "backup-dir", "", "Custom backup directory")
	setupOpenCodeCmd.Flags().BoolVar(&ocSetupRefreshTokenOnly, "refresh-token-only", false, "Only update the authentication token")
	setupOpenCodeCmd.Flags().BoolVar(&ocSetupRequireInstalled, "require-installed", false, "Fail if OpenCode CLI is not installed")
	setupOpenCodeCmd.Flags().StringVar(&ocSetupFormat, "format", "", "Output format (json)")
}

func runSetupOpenCode(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	isJSONMode := ocSetupFormat == "json"

	scope := integrations.ScopeUser
	if ocSetupProject {
		scope = integrations.ScopeProject
	}

	opts := buildOpenCodeApplyOpts(scope)
	integration := opencode.New()
	inputReader := bufio.NewReader(cmd.InOrStdin())

	// Get and validate status
	status, err := integration.Status(ctx, scope)
	if err != nil {
		return handleOpenCodeError(cmd, isJSONMode, "failed to check status: %w", err)
	}

	if err := showOpenCodeDetectionInfo(cmd, status, isJSONMode); err != nil {
		return err
	}

	// Plan changes
	planResult, err := planOpenCodeChanges(ctx, integration, opts)
	if err != nil {
		return handleOpenCodeError(cmd, isJSONMode, "%w", err)
	}

	// Handle no changes case
	if !planResult.Changed {
		return handleOpenCodeNoChanges(cmd, status, isJSONMode)
	}

	// Show planned changes
	showOpenCodePlannedChanges(cmd, planResult, isJSONMode)

	// Handle dry-run
	if ocSetupDryRun {
		return handleOpenCodeDryRun(cmd, planResult, status, isJSONMode)
	}

	// Confirm changes
	if !confirmOpenCodeChanges(cmd, inputReader, isJSONMode) {
		return nil
	}

	// Apply changes
	result, err := applyOpenCodeChanges(ctx, integration, opts)
	if err != nil {
		return handleOpenCodeError(cmd, isJSONMode, "%w", err)
	}

	// Output results
	return outputOpenCodeResults(cmd, result, status, isJSONMode)
}

func buildOpenCodeApplyOpts(scope integrations.Scope) integrations.ApplyOpts {
	return integrations.ApplyOpts{
		Scope:            scope,
		TokenOverride:    ocSetupToken,
		Force:            ocSetupForce,
		RefreshTokenOnly: ocSetupRefreshTokenOnly,
		DryRun:           ocSetupDryRun,
		BackupDir:        ocSetupBackupDir,
		RequireInstalled: ocSetupRequireInstalled,
	}
}

func showOpenCodeDetectionInfo(cmd *cobra.Command, status integrations.StatusResult, isJSONMode bool) error {
	if !isJSONMode {
		if status.Installed {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ OpenCode CLI detected: %s\n", status.Version)
		} else {
			if ocSetupRequireInstalled {
				return fmt.Errorf("opencode CLI not found; install it first: https://opencode.ai")
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠ OpenCode CLI not detected (will configure anyway)\n")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "📁 Config path: %s\n", status.ConfigPath)
	} else if ocSetupRequireInstalled && !status.Installed {
		return outputOpenCodeJSON(cmd, "error", map[string]any{"error": "opencode CLI not found"})
	}
	return nil
}

func planOpenCodeChanges(ctx context.Context, integration *opencode.OpenCode, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	planOpts := opts
	planOpts.DryRun = true
	return integration.Apply(ctx, planOpts)
}

func handleOpenCodeNoChanges(cmd *cobra.Command, status integrations.StatusResult, isJSONMode bool) error {
	if isJSONMode {
		return outputOpenCodeJSON(cmd, "success", map[string]any{
			"message":     "Already configured",
			"changed":     false,
			"config_path": status.ConfigPath,
		})
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✓ Already configured! No changes needed.")
	return nil
}

func showOpenCodePlannedChanges(cmd *cobra.Command, planResult integrations.ApplyResult, isJSONMode bool) {
	if !isJSONMode {
		fmt.Fprintln(cmd.OutOrStdout(), "\n📝 Changes to apply:")
		for _, change := range planResult.UpdatedKeys {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", change)
		}
	}
}

func handleOpenCodeDryRun(cmd *cobra.Command, planResult integrations.ApplyResult, status integrations.StatusResult, isJSONMode bool) error {
	if isJSONMode {
		return outputOpenCodeJSON(cmd, "success", map[string]any{
			"message":      "Dry run completed",
			"changed":      true,
			"updated_keys": planResult.UpdatedKeys,
			"config_path":  status.ConfigPath,
		})
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\n🔍 Dry run - no changes made")
	return nil
}

func confirmOpenCodeChanges(cmd *cobra.Command, inputReader *bufio.Reader, isJSONMode bool) bool {
	if ocSetupForce || isJSONMode {
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

func applyOpenCodeChanges(ctx context.Context, integration *opencode.OpenCode, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	writeOpts := opts
	writeOpts.DryRun = false
	return integration.Apply(ctx, writeOpts)
}

func outputOpenCodeResults(cmd *cobra.Command, result integrations.ApplyResult, status integrations.StatusResult, isJSONMode bool) error {
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
		return outputOpenCodeJSON(cmd, "success", data)
	}

	if result.BackupPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "💾 Backup created: %s\n", result.BackupPath)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✅ Successfully configured OpenCode for Costa!")
	return nil
}

func handleOpenCodeError(cmd *cobra.Command, isJSONMode bool, format string, err error) error {
	if isJSONMode {
		return outputOpenCodeJSON(cmd, "error", map[string]any{"error": err.Error()})
	}
	return fmt.Errorf(format, err)
}

// outputOpenCodeJSON outputs a JSON response with status and data for setup commands
func outputOpenCodeJSON(cmd *cobra.Command, status string, data map[string]any) error {
	output := map[string]any{
		"status": status,
		"data":   data,
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
