package cmd

import (
	"fmt"

	"github.com/emersonkopp/dev-context-cli/internal/config"
	"github.com/emersonkopp/dev-context-cli/providers/kiro"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and configure all AI tool artifacts from the monorepo",
	Long: `Copies artifacts from the local dev-context monorepo to their global
destinations on this machine. Safe to run multiple times — files that are
already up-to-date are skipped.

Currently installs:
  • Kiro steerings  → ~/.kiro/steering/
  • Kiro settings   → ~/.kiro/settings/cli.json  (merged, preserves existing keys)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("%w\n\nRun 'dctx config init' to set up the CLI first.", err)
		}

		providers := []interface {
			Name() string
			Install(string) error
		}{
			&kiro.Provider{},
		}

		anyError := false
		for _, p := range providers {
			color.Cyan("\n▸ Installing %s artifacts…", p.Name())
			if err := p.Install(cfg.RepoPath); err != nil {
				color.Red("  ✗  %s install failed: %v", p.Name(), err)
				anyError = true
			}
		}

		fmt.Println()
		if anyError {
			return fmt.Errorf("one or more providers failed — check output above")
		}
		color.Green("All artifacts installed successfully.")
		return nil
	},
}
