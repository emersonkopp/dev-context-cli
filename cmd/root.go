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

Quick start:
  dctx config init   # configure the path to your local monorepo clone
  dctx install       # install all artifacts on this machine
  dctx sync          # pull/push changes to/from GitHub
  dctx status        # check what is installed and up-to-date
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
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)

	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(updateCmd)

	rootCmd.Version = Version
}
