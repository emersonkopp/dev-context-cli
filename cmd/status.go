package cmd

import (
	"fmt"

	"github.com/emersonkopp/dev-context-cli/internal/config"
	"github.com/emersonkopp/dev-context-cli/internal/provider"
	"github.com/emersonkopp/dev-context-cli/providers/kiro"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show installation status of all managed artifacts",
	Long:  `Lists every artifact managed by dctx and reports whether it is installed and up-to-date.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("%w\n\nRun 'dctx config init' to set up the CLI first.", err)
		}

		providers := []interface {
			Name() string
			Status(string) ([]provider.ArtifactStatus, error)
		}{
			&kiro.Provider{},
		}

		for _, p := range providers {
			color.Cyan("\n▸ %s", p.Name())
			statuses, err := p.Status(cfg.RepoPath)
			if err != nil {
				color.Red("  ✗  failed to get status: %v", err)
				continue
			}
			if len(statuses) == 0 {
				color.White("  (no artifacts found)")
				continue
			}
			for _, s := range statuses {
				printArtifactStatus(s)
			}
		}
		fmt.Println()
		return nil
	},
}

func printArtifactStatus(s provider.ArtifactStatus) {
	switch {
	case !s.Installed:
		color.Red("  ✗  not installed  %s", s.Name)
		color.White("                   → %s", s.DstPath)
	case !s.UpToDate:
		color.Yellow("  ⚠  outdated       %s", s.Name)
		color.White("                   → %s", s.DstPath)
	default:
		color.Green("  ✓  up-to-date     %s", s.Name)
	}
}
