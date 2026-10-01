// Package cmd contains all CLI commands.
package cmd

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "dctx",
	Short: "dev-context CLI — manage your AI tool artifacts across machines",
	Long: `dctx manages the dev-context monorepo artifacts (steerings, settings, prompts,
agents) and installs them into the correct global locations for each AI tool.

First time on a new machine:
  curl -fsSL https://raw.githubusercontent.com/emersonkopp/dev-context-cli/main/install.sh | bash

That's it. The script installs dctx and runs 'dctx bootstrap' automatically.

Other useful commands:
  dctx status        # check what is installed and up-to-date
  dctx sync          # pull/push changes to/from GitHub
  dctx update        # update dctx itself to the latest release`,
	SilenceUsage: true,
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		color.Red("%v", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(bootstrapCmd)

	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)

	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(updateCmd)

	rootCmd.Version = Version
}
