package cmd

import (
	"fmt"

	"github.com/emersonkopp/dev-context-cli/internal/update"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update dctx to the latest version",
	Long: `Checks GitHub Releases for a newer version of dctx.
If a newer version is found, downloads and replaces the current binary in-place.

The binary is always downloaded for the current OS and architecture.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		color.Cyan("▸ Checking for updates…")
		color.White("  Current version: %s", Version)

		rel, err := update.LatestRelease()
		if err != nil {
			return fmt.Errorf("fetching release info: %w", err)
		}
		color.White("  Latest release:  %s", rel.TagName)

		if !update.NeedsUpdate(Version, rel.TagName) {
			color.Green("  ✓  Already up-to-date.")
			return nil
		}

		color.Yellow("  New version available: %s → %s", Version, rel.TagName)

		assetURL, err := update.AssetURL(rel)
		if err != nil {
			return fmt.Errorf("finding asset: %w", err)
		}
		color.Cyan("  Downloading from %s…", assetURL)

		if err := update.Apply(assetURL); err != nil {
			return fmt.Errorf("applying update: %w", err)
		}

		color.Green("  ✓  Updated to %s. Restart dctx to use the new version.", rel.TagName)
		return nil
	},
}
