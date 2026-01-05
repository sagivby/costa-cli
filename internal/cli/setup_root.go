package cli

import (
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup integrations with Costa",
	Long:  `Setup and configure third-party tools to work with Costa.`,
}

func init() {
	setupCmd.AddCommand(setupClaudeCodeCmd)
	setupCmd.AddCommand(setupCodexCmd)
	setupCmd.AddCommand(setupKiloCmd)
	setupCmd.AddCommand(setupStatusCmd)
}
