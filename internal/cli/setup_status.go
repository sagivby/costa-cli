package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costa-app/costa-cli/internal/integrations"
	"github.com/costa-app/costa-cli/internal/integrations/claudecode"
	"github.com/costa-app/costa-cli/internal/integrations/codex"
	"github.com/costa-app/costa-cli/internal/integrations/kilo"
)

var (
	setupUser         bool
	setupProject      bool
	setupStatusFormat string
)

var setupStatusCmd = &cobra.Command{
	Use:   "status [app]",
	Short: "Check setup status",
	Long:  `Check if tools are installed and configured to use Costa. Run without arguments to check all apps.`,
	RunE:  runSetupStatus,
}

func init() {
	setupStatusCmd.Flags().BoolVar(&setupUser, "user", false, "Check user config (default)")
	setupStatusCmd.Flags().BoolVar(&setupProject, "project", false, "Check project config")
	setupStatusCmd.Flags().StringVar(&setupStatusFormat, "format", "", "Output format (json)")
}

func runSetupStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Determine scope
	scope := integrations.ScopeUser
	if setupProject {
		scope = integrations.ScopeProject
	}

	// If specific app requested
	if len(args) > 0 {
		appName := args[0]
		// Normalize aliases
		if appName == "claude" || appName == "claude code" {
			appName = "claude-code"
		}

		if appName == "claude-code" {
			return showClaudeCodeStatus(cmd, ctx, scope)
		}
		if appName == "codex" {
			return showCodexStatus(cmd, ctx, scope)
		}
		if appName == "kilo" || appName == "kilo-code" {
			return showKiloStatus(cmd, ctx, scope)
		}

		return fmt.Errorf("unknown app: %s", appName)
	}

	// Check Claude Code
	claudeStatus, err := claudecode.New().Status(ctx, scope)
	if err != nil && setupStatusFormat != "json" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error checking Claude Code: %v\n", err)
	}

	// Check Codex
	codexStatus, codexErr := codex.New().Status(ctx, scope)
	if codexErr != nil && setupStatusFormat != "json" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error checking Codex: %v\n", codexErr)
	}

	// Check Kilo
	kiloStatus, kiloErr := kilo.New().Status(ctx, scope)
	if kiloErr != nil && setupStatusFormat != "json" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error checking Kilo: %v\n", kiloErr)
	}

	// JSON output
	if setupStatusFormat == "json" {
		output := map[string]interface{}{
			"claude_code": map[string]interface{}{
				"installed":        claudeStatus.Installed,
				"version":          claudeStatus.Version,
				"config_exists":    claudeStatus.ConfigExists,
				"is_costa_enabled": claudeStatus.IsCosta,
			},
			"codex": map[string]interface{}{
				"config_exists":    codexStatus.ConfigExists,
				"is_costa_enabled": codexStatus.IsCosta,
			},
			"kilo": map[string]interface{}{
				"installed":        kiloStatus.Installed,
				"version":          kiloStatus.Version,
				"config_exists":    kiloStatus.ConfigExists,
				"is_costa_enabled": kiloStatus.IsCosta,
			},
		}
		if err != nil {
			output["error"] = err.Error()
		}
		if kiloErr != nil {
			output["kilo_error"] = kiloErr.Error()
		}
		data, jsonErr := json.Marshal(output)
		if jsonErr != nil {
			return jsonErr
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	// Human-readable output
	fmt.Fprintln(cmd.OutOrStdout(), "🔍 Costa Setup Status")

	if err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Claude Code:    %s\n", formatStatusIcon(claudeStatus.IsCosta))
		if claudeStatus.Installed {
			fmt.Fprintf(cmd.OutOrStdout(), "  Installed:    ✓ %s\n", claudeStatus.Version)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "  Installed:    ✗ Not found")
		}
		if claudeStatus.ConfigExists {
			if claudeStatus.IsCosta {
				fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ✓ Costa enabled")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ⚠ Partial setup")
			}
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ✗ Not configured")
		}
	}

	if codexErr == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Codex:          %s\n", formatStatusIcon(codexStatus.IsCosta))
		if codexStatus.ConfigExists {
			if codexStatus.IsCosta {
				fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ✓ Costa enabled")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ⚠ Partial setup")
			}
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ✗ Not configured")
		}
	}

	if kiloErr == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Kilo:           %s\n", formatStatusIcon(kiloStatus.IsCosta))
		if kiloStatus.Installed {
			fmt.Fprintf(cmd.OutOrStdout(), "  Installed:    ✓ %s\n", kiloStatus.Version)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "  Installed:    ✗ Not found")
		}
		if kiloStatus.ConfigExists {
			if kiloStatus.IsCosta {
				fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ✓ Costa enabled")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ⚠ Partial setup")
			}
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "  Configured:   ✗ Not configured")
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nRun 'costa setup status <app>' for details.\n")

	return nil
}

func showClaudeCodeStatus(cmd *cobra.Command, ctx context.Context, scope integrations.Scope) error {
	integration := claudecode.New()
	status, err := integration.Status(ctx, scope)
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	// JSON output
	if setupStatusFormat == "json" {
		output := map[string]interface{}{
			"installed":        status.Installed,
			"version":          status.Version,
			"scope":            string(status.Scope),
			"config_path":      status.ConfigPath,
			"config_exists":    status.ConfigExists,
			"is_costa_enabled": status.IsCosta,
		}
		if status.Model != "" {
			output["model"] = status.Model
		}
		if status.TokenRedacted != "" {
			output["token_redacted"] = status.TokenRedacted
		}
		if len(status.Missing) > 0 {
			output["missing"] = status.Missing
		}
		data, jsonErr := json.Marshal(output)
		if jsonErr != nil {
			return jsonErr
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	// Human-readable output
	fmt.Fprintln(cmd.OutOrStdout(), "🔍 Claude Code Setup Status")

	// Claude CLI
	if status.Installed {
		fmt.Fprintf(cmd.OutOrStdout(), "Claude CLI:     ✓ Installed (%s)\n", status.Version)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Claude CLI:     ✗ Not found")
	}

	// Config info
	fmt.Fprintf(cmd.OutOrStdout(), "Config scope:   %s\n", status.Scope)
	fmt.Fprintf(cmd.OutOrStdout(), "Config path:    %s\n", status.ConfigPath)

	// Config status
	if !status.ConfigExists {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ✗ Not configured")
		fmt.Fprintln(cmd.OutOrStdout(), "Run 'costa setup claude-code' to configure.")
		return nil
	}

	if status.IsCosta {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ✓ Configured for Costa")

		// Show current model
		if status.Model != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Model:          %s\n", status.Model)
		}

		// Check token presence (redacted)
		if status.TokenRedacted != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Token:          %s\n", status.TokenRedacted)
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ⚠ Partially configured")
		if len(status.Missing) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "\nMissing Costa settings:")
			for _, key := range status.Missing {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", key)
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), "\nRun 'costa setup claude-code' to fix.")
	}

	return nil
}

func showCodexStatus(cmd *cobra.Command, ctx context.Context, scope integrations.Scope) error {
	integration := codex.New()
	status, err := integration.Status(ctx, scope)
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	// JSON output
	if setupStatusFormat == "json" {
		output := map[string]interface{}{
			"scope":            string(status.Scope),
			"config_path":      status.ConfigPath,
			"config_exists":    status.ConfigExists,
			"is_costa_enabled": status.IsCosta,
		}
		if status.Model != "" {
			output["model"] = status.Model
		}
		data, jsonErr := json.Marshal(output)
		if jsonErr != nil {
			return jsonErr
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	// Human-readable output
	fmt.Fprintln(cmd.OutOrStdout(), "🔍 Codex Setup Status")

	// Config info
	fmt.Fprintf(cmd.OutOrStdout(), "Config scope:   %s\n", status.Scope)
	fmt.Fprintf(cmd.OutOrStdout(), "Config path:    %s\n", status.ConfigPath)

	// Config status
	if !status.ConfigExists {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ✗ Not configured")
		fmt.Fprintln(cmd.OutOrStdout(), "Run 'costa setup codex' to configure.")
		return nil
	}

	if status.IsCosta {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ✓ Configured for Costa")

		// Show current model
		if status.Model != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Model:          %s\n", status.Model)
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ⚠ Partially configured")
		fmt.Fprintln(cmd.OutOrStdout(), "\nRun 'costa setup codex' to fix.")
	}

	return nil
}

func showKiloStatus(cmd *cobra.Command, ctx context.Context, scope integrations.Scope) error {
	integration := kilo.New()
	status, err := integration.Status(ctx, scope)
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	// JSON output
	if setupStatusFormat == "json" {
		output := map[string]interface{}{
			"installed":        status.Installed,
			"version":          status.Version,
			"config_path":      status.ConfigPath,
			"config_exists":    status.ConfigExists,
			"is_costa_enabled": status.IsCosta,
		}
		if status.Model != "" {
			output["model"] = status.Model
		}
		if len(status.Missing) > 0 {
			output["missing"] = status.Missing
		}
		data, jsonErr := json.Marshal(output)
		if jsonErr != nil {
			return jsonErr
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	// Human-readable output
	fmt.Fprintln(cmd.OutOrStdout(), "🔍 Kilo Setup Status")

	// VS Code
	if status.Installed {
		fmt.Fprintf(cmd.OutOrStdout(), "VS Code:        ✓ Installed (%s)\n", status.Version)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "VS Code:        ✗ Not found")
	}

	// Database path
	fmt.Fprintf(cmd.OutOrStdout(), "Database path:  %s\n", status.ConfigPath)

	// Config status
	if !status.ConfigExists {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ✗ Not configured")
		fmt.Fprintln(cmd.OutOrStdout(), "Run 'costa setup kilo' to configure.")
		return nil
	}

	if status.IsCosta {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ✓ Configured for Costa")

		// Show current model
		if status.Model != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Model:          %s\n", status.Model)
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Config status:  ⚠ Partially configured")
		if len(status.Missing) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "\nMissing Costa settings:")
			for _, key := range status.Missing {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", key)
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), "\nRun 'costa setup kilo' to fix.")
	}

	return nil
}

func formatStatusIcon(isCosta bool) string {
	if isCosta {
		return "✓"
	}
	return "⚠"
}
