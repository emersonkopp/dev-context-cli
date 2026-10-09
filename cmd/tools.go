package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/emersonkopp/dev-context-cli/internal/config"
	"github.com/emersonkopp/dev-context-cli/internal/tools"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Discover and install standalone tools shipped in the monorepo",
	Long: `Tools are software projects under the monorepo's tools/ directory (each with
its own installer), distinct from the declarative artifacts handled by
'dctx install'. 'dctx tools status' only inspects state; 'dctx tools install'
runs a tool's own installer after showing you exactly what will execute.`,
}

var toolsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "List tools in the monorepo and whether each is installed",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("%w\n\nRun 'dctx config init' to set up the CLI first.", err)
		}
		return printToolsStatus(cfg.RepoPath)
	},
}

var toolsInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Run a tool's own installer (shows a preview and asks for confirmation)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("%w\n\nRun 'dctx config init' to set up the CLI first.", err)
		}
		name := args[0]
		tool, err := tools.Find(cfg.RepoPath, name)
		if err != nil {
			return err
		}
		script, err := tool.InstallScriptPath()
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(script); statErr != nil {
			return fmt.Errorf("installer not found: %s", script)
		}

		// Preview: show exactly what will run before doing anything.
		color.Cyan("\n▸ Tool: %s", tool.Name)
		if tool.Description != "" {
			color.White("  %s", tool.Description)
		}
		color.White("  Installer: %s", script)
		color.White("  Working dir: %s", tool.Dir())
		if tool.IsInstalled() {
			color.Yellow("  Note: this tool already appears to be installed.")
		}
		color.Yellow("\n  This will EXECUTE the installer script above with bash.")

		var proceed bool
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("Run the installer for %q?", tool.Name),
			Default: false,
		}, &proceed); err != nil || !proceed {
			color.White("  Aborted. Nothing was executed.")
			return nil
		}

		color.Cyan("\n▸ Installing %s…", tool.Name)
		// #nosec G204 -- script path is validated to stay within the tool dir
		// (InstallScriptPath rejects traversal) and the user explicitly confirmed.
		c := exec.Command("bash", script)
		c.Dir = tool.Dir()
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		if err := c.Run(); err != nil {
			return fmt.Errorf("installer for %q failed: %w", tool.Name, err)
		}
		color.Green("\n  ✓  %s installer finished.", tool.Name)
		return nil
	},
}

// printToolsStatus lists discovered tools and their installed state. Read-only.
func printToolsStatus(repoPath string) error {
	list, err := tools.Discover(repoPath)
	if err != nil {
		return err
	}
	color.Cyan("\n▸ Tools")
	if len(list) == 0 {
		color.White("  (no tools found in %s)", filepath.Join(repoPath, "tools"))
		fmt.Println()
		return nil
	}
	for _, tool := range list {
		if tool.IsInstalled() {
			color.Green("  ✓  installed      %s", tool.Name)
		} else {
			color.White("  –  not installed  %s   (run: dctx tools install %s)", tool.Name, tool.Name)
		}
		if tool.Description != "" {
			color.White("                    %s", tool.Description)
		}
	}
	fmt.Println()
	return nil
}
