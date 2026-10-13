package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/emersonkopp/dev-context-cli/internal/config"
	"github.com/emersonkopp/dev-context-cli/providers/kiro"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove artifacts dctx installed (and optionally its state and binary)",
	Long: `Removes the artifacts dctx installed into each AI tool's global locations,
using dctx's own manifest. It is conservative by design:

  • Only files dctx installed are removed — organization-local skills and any
    files dctx never installed are left untouched.
  • Merged files that may hold your own keys (e.g. ~/.kiro/settings/cli.json)
    are kept, not deleted.
  • The cloned monorepo is never touched.

By default, dctx keeps its state in ~/.dev-context (config + manifest) so a
later reinstall picks up exactly where you left off. Use --purge to wipe it.

Flags:
  --purge    also delete ~/.dev-context (config + manifest) — full clean slate
  --binary   also delete the dctx binary itself
  --yes      do not prompt for confirmation`,
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().Bool("purge", false, "Also delete ~/.dev-context (config + manifest)")
	uninstallCmd.Flags().Bool("binary", false, "Also delete the dctx binary itself")
	uninstallCmd.Flags().BoolP("yes", "y", false, "Do not prompt for confirmation")
}

func runUninstall(cmd *cobra.Command, args []string) error {
	purge, _ := cmd.Flags().GetBool("purge")
	removeBinary, _ := cmd.Flags().GetBool("binary")
	yes, _ := cmd.Flags().GetBool("yes")

	// ── Confirmation ──────────────────────────────────────────────────────────
	if !yes {
		msg := "Remove all artifacts dctx installed?"
		if purge {
			msg = "Remove all artifacts AND wipe ~/.dev-context (config + manifest)?"
		}
		var ok bool
		if err := survey.AskOne(&survey.Confirm{Message: msg, Default: false}, &ok); err != nil {
			return fmt.Errorf("prompt cancelled: %w", err)
		}
		if !ok {
			color.Yellow("Aborted — nothing was removed.")
			return nil
		}
	}

	// ── 1. Remove provider-managed artifacts ──────────────────────────────────
	color.Cyan("▸ Removing installed artifacts…")

	providers := []interface {
		Name() string
		Uninstall() error
	}{
		&kiro.Provider{},
	}

	anyError := false
	for _, p := range providers {
		color.Cyan("\n  [%s]", p.Name())
		if err := p.Uninstall(); err != nil {
			color.Red("  ✗  %s uninstall failed: %v", p.Name(), err)
			anyError = true
		}
	}

	// ── 2. Optionally purge state ─────────────────────────────────────────────
	if purge {
		color.Cyan("\n▸ Purging dctx state…")
		if err := config.RemoveState(); err != nil {
			color.Red("  ✗  could not remove state: %v", err)
			anyError = true
		} else {
			dir, _ := config.Dir()
			color.Green("  ✓  removed %s", dir)
		}
	} else {
		color.White("\n   State kept in ~/.dev-context (use --purge to wipe).")
		color.White("   The cloned monorepo was not touched.")
	}

	// ── 3. Optionally remove the binary itself ────────────────────────────────
	if removeBinary {
		color.Cyan("\n▸ Removing the dctx binary…")
		if err := removeSelf(); err != nil {
			color.Red("  ✗  could not remove binary: %v", err)
			color.Yellow("     Remove it manually: rm \"$(command -v dctx)\"")
			anyError = true
		}
	}

	fmt.Println()
	if anyError {
		return fmt.Errorf("uninstall completed with errors — check output above")
	}
	color.Green("✓  Uninstall complete.")
	return nil
}

// removeSelf deletes the currently running dctx executable. The running process
// keeps executing from the already-loaded image, so deleting the file is safe
// on Linux/macOS. Resolves symlinks so the real file (not a link) is removed.
func removeSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe // fall back to the unresolved path
	}
	if err := os.Remove(resolved); err != nil {
		return err
	}
	color.Green("  ✓  removed %s", resolved)
	return nil
}
