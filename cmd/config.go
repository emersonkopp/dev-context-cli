package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/emersonkopp/dev-context-cli/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage dctx configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the dctx configuration interactively",
	Long: `Creates ~/.dev-context/config.json with the path to your local
dev-context monorepo clone. Run this once on each machine.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if config already exists.
		existing, err := config.Load()
		if err == nil {
			color.Yellow("  Config already exists at %s", mustConfigPath())
			var overwrite bool
			if err := survey.AskOne(&survey.Confirm{
				Message: "Overwrite existing config?",
				Default: false,
			}, &overwrite); err != nil || !overwrite {
				color.White("  Keeping existing config.")
				return nil
			}
			_ = existing
		}

		var repoPath string
		if err := survey.AskOne(&survey.Input{
			Message: "Path to your local dev-context monorepo clone:",
			Default: defaultRepoPath(),
		}, &repoPath, survey.WithValidator(survey.Required)); err != nil {
			return fmt.Errorf("prompt cancelled: %w", err)
		}

		// Validate path exists.
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			color.Yellow("  ⚠  Path does not exist yet: %s", repoPath)
			color.Yellow("     You can still save the config and clone the repo later.")
		}

		cfg, err := config.Init(repoPath)
		if err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		p, _ := config.Path()
		color.Green("  ✓  Config saved to %s", p)
		color.White("     repo_path: %s", cfg.RepoPath)
		color.White("     repo_url:  %s", cfg.RepoURL)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("%w\n\nRun 'dctx config init' to set up the CLI first.", err)
		}

		p, _ := config.Path()
		color.Cyan("Config file: %s\n", p)

		out, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(out))
		return nil
	},
}

func mustConfigPath() string {
	p, _ := config.Path()
	return p
}

func defaultRepoPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "~/git/dev-context"
	}
	return home + "/git/dev-context"
}
