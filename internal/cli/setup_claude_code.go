package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/costa-app/costa-cli/internal/integrations"
	"github.com/costa-app/costa-cli/internal/integrations/claudecode"
)

var (
	ccSetupUser             bool
	ccSetupProject          bool
	ccSetupToken            string
	ccSetupForce            bool
	ccSetupDryRun           bool
	ccSetupBackupDir        string
	ccSetupRefreshTokenOnly bool
	ccSetupRequireInstalled bool
	ccSetupEnableStatusLine bool
	ccSetupSkipStatusLine   bool
	ccSetupFormat           string
)

var setupClaudeCodeCmd = &cobra.Command{
	Use:     "claude-code",
	Aliases: []string{"claude", "claude code"},
	Short:   "Setup Claude Code to use Costa",
	Long:    `Configure Claude Code (CLI and VS Code extension) to use Costa's API and token.`,
	RunE:    runSetupClaudeCode,
}

func init() {
	setupClaudeCodeCmd.Flags().BoolVar(&ccSetupUser, "user", false, "Setup for current user (default)")
	setupClaudeCodeCmd.Flags().BoolVar(&ccSetupProject, "project", false, "Setup for current project")
	setupClaudeCodeCmd.Flags().StringVar(&ccSetupToken, "token", "", "Use explicit token instead of fetching from Costa")
	setupClaudeCodeCmd.Flags().BoolVar(&ccSetupForce, "force", false, "Skip confirmation prompt (auto-yes)")
	setupClaudeCodeCmd.Flags().BoolVar(&ccSetupDryRun, "dry-run", false, "Show what would change without writing")
	setupClaudeCodeCmd.Flags().StringVar(&ccSetupBackupDir, "backup-dir", "", "Custom backup directory")
	setupClaudeCodeCmd.Flags().BoolVar(&ccSetupRefreshTokenOnly, "refresh-token-only", false, "Only update the authentication token")
	setupClaudeCodeCmd.Flags().BoolVar(&ccSetupRequireInstalled, "require-installed", false, "Fail if Claude CLI is not installed")
	setupClaudeCodeCmd.Flags().BoolVar(&ccSetupEnableStatusLine, "enable-statusline", false, "Enable Claude Code status line")
	setupClaudeCodeCmd.Flags().BoolVar(&ccSetupSkipStatusLine, "skip-statusline", false, "Skip statusline prompt")
	setupClaudeCodeCmd.Flags().StringVar(&ccSetupFormat, "format", "", "Output format (json)")
}

func runSetupClaudeCode(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	isJSONMode := ccSetupFormat == "json"

	scope := integrations.ScopeUser
	if ccSetupProject {
		scope = integrations.ScopeProject
	}

	opts := buildApplyOpts(scope)
	integration := claudecode.New()
	inputReader := bufio.NewReader(cmd.InOrStdin())

	// Get and validate status
	status, err := integration.Status(ctx, scope)
	if err != nil {
		return handleError(cmd, isJSONMode, "failed to check status: %w", err)
	}

	if err := showDetectionInfo(cmd, status, isJSONMode); err != nil {
		return err
	}

	// Plan changes
	planResult, err := planChanges(ctx, integration, opts)
	if err != nil {
		return handleError(cmd, isJSONMode, "%w", err)
	}

	// Handle no changes case
	if !planResult.Changed {
		return handleNoChanges(cmd, status, isJSONMode)
	}

	// Show planned changes
	showPlannedChanges(cmd, planResult, isJSONMode)

	// Handle dry-run
	if ccSetupDryRun {
		return handleDryRun(cmd, planResult, status, isJSONMode)
	}

	// Prompt for status line if needed
	planResult, opts, err = promptForStatusLine(cmd, inputReader, ctx, integration, planResult, opts, isJSONMode)
	if err != nil {
		return err
	}

	// Confirm changes
	if !confirmChanges(cmd, inputReader, isJSONMode) {
		return nil
	}

	// Apply changes
	result, err := applyChanges(ctx, integration, opts)
	if err != nil {
		return handleError(cmd, isJSONMode, "%w", err)
	}

	// Output results
	return outputResults(cmd, result, status, isJSONMode)
}

func buildApplyOpts(scope integrations.Scope) integrations.ApplyOpts {
	return integrations.ApplyOpts{
		Scope:            scope,
		TokenOverride:    ccSetupToken,
		Force:            ccSetupForce,
		RefreshTokenOnly: ccSetupRefreshTokenOnly,
		DryRun:           ccSetupDryRun,
		BackupDir:        ccSetupBackupDir,
		RequireInstalled: ccSetupRequireInstalled,
		EnableStatusLine: ccSetupEnableStatusLine,
		SkipStatusLine:   ccSetupSkipStatusLine,
	}
}

func showDetectionInfo(cmd *cobra.Command, status integrations.StatusResult, isJSONMode bool) error {
	if !isJSONMode {
		if status.Installed {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Claude CLI detected: %s\n", status.Version)
		} else {
			if ccSetupRequireInstalled {
				return fmt.Errorf("claude CLI not found; install it first: https://docs.claude.com/en/docs/claude-code/quickstart")
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠ Claude CLI not detected (will configure anyway)\n")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "📁 Config path: %s\n", status.ConfigPath)
	} else if ccSetupRequireInstalled && !status.Installed {
		return outputSetupJSON(cmd, "error", map[string]any{"error": "claude CLI not found"})
	}
	return nil
}

func planChanges(ctx context.Context, integration *claudecode.ClaudeCode, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	planOpts := opts
	planOpts.DryRun = true
	return integration.Apply(ctx, planOpts)
}

func handleNoChanges(cmd *cobra.Command, status integrations.StatusResult, isJSONMode bool) error {
	if isJSONMode {
		return outputSetupJSON(cmd, "success", map[string]any{
			"message":     "Already configured",
			"changed":     false,
			"config_path": status.ConfigPath,
		})
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✓ Already configured! No changes needed.")
	return nil
}

func showPlannedChanges(cmd *cobra.Command, planResult integrations.ApplyResult, isJSONMode bool) {
	if !isJSONMode {
		fmt.Fprintln(cmd.OutOrStdout(), "\n📝 Changes to apply:")
		for _, change := range planResult.UpdatedKeys {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", change)
		}
	}
}

func handleDryRun(cmd *cobra.Command, planResult integrations.ApplyResult, status integrations.StatusResult, isJSONMode bool) error {
	if isJSONMode {
		return outputSetupJSON(cmd, "success", map[string]any{
			"message":      "Dry run completed",
			"changed":      true,
			"updated_keys": planResult.UpdatedKeys,
			"config_path":  status.ConfigPath,
		})
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\n🔍 Dry run - no changes made")
	return nil
}

func promptForStatusLine(cmd *cobra.Command, inputReader *bufio.Reader, ctx context.Context, integration *claudecode.ClaudeCode, planResult integrations.ApplyResult, opts integrations.ApplyOpts, isJSONMode bool) (integrations.ApplyResult, integrations.ApplyOpts, error) {
	if ccSetupSkipStatusLine || ccSetupEnableStatusLine || ccSetupRefreshTokenOnly || ccSetupForce || isJSONMode {
		return planResult, opts, nil
	}
	fmt.Fprint(cmd.OutOrStdout(), "\n📊 Would you like to include the Costa status line in Claude Code?\n")
	fmt.Fprint(cmd.OutOrStdout(), "   This will show your points usage in the Claude Code status bar.\n")
	fmt.Fprint(cmd.OutOrStdout(), "   Include status line? [Y/n]: ")
	response, _ := inputReader.ReadString('\n')
	resp := strings.ToLower(strings.TrimSpace(response))

	if resp == "n" || resp == "no" {
		return planResult, opts, nil
	}

	// User accepted status line
	ccSetupEnableStatusLine = true
	opts.EnableStatusLine = true

	// Re-plan with statusLine enabled
	planOpts := opts
	planOpts.DryRun = true
	planOpts.EnableStatusLine = true
	newPlanResult, err := integration.Apply(ctx, planOpts)
	if err != nil {
		return planResult, opts, err
	}

	// Show updated changes
	fmt.Fprintln(cmd.OutOrStdout(), "\n📝 Updated changes to apply:")
	for _, change := range newPlanResult.UpdatedKeys {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", change)
	}

	return newPlanResult, opts, nil
}

func confirmChanges(cmd *cobra.Command, inputReader *bufio.Reader, isJSONMode bool) bool {
	if ccSetupForce || isJSONMode {
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

func applyChanges(ctx context.Context, integration *claudecode.ClaudeCode, opts integrations.ApplyOpts) (integrations.ApplyResult, error) {
	writeOpts := opts
	writeOpts.DryRun = false
	return integration.Apply(ctx, writeOpts)
}

func outputResults(cmd *cobra.Command, result integrations.ApplyResult, status integrations.StatusResult, isJSONMode bool) error {
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
		return outputSetupJSON(cmd, "success", data)
	}

	if result.BackupPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "💾 Backup created: %s\n", result.BackupPath)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✅ Successfully configured Claude Code for Costa!")
	return nil
}

func handleError(cmd *cobra.Command, isJSONMode bool, format string, err error) error {
	if isJSONMode {
		return outputSetupJSON(cmd, "error", map[string]any{"error": err.Error()})
	}
	return fmt.Errorf(format, err)
}

// outputSetupJSON outputs a JSON response with status and data for setup commands
func outputSetupJSON(cmd *cobra.Command, status string, data map[string]any) error {
	output := map[string]any{
		"status": status,
		"data":   data,
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
