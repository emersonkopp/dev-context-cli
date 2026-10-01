package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/emersonkopp/dev-context-cli/internal/config"
	"github.com/emersonkopp/dev-context-cli/providers/kiro"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Full first-time setup: clone repo, configure and install all artifacts",
	Long: `Bootstrap sets up everything on a new machine in one shot:

  1. Clones the dev-context monorepo if it is not already present locally.
  2. Saves ~/.dev-context/config.json pointing at the local clone.
  3. Installs all artifacts (steerings, settings, …) to their global destinations.

The only prompt you may see is if the default clone path (~/git/dev-context)
already exists as something else, or if you want to use a different location.
In all other cases the command runs non-interactively.`,
	RunE: runBootstrap,
}

func init() {
	bootstrapCmd.Flags().StringP("repo-path", "p", "", "Local path for the monorepo clone (default: ~/git/dev-context)")
	bootstrapCmd.Flags().BoolP("yes", "y", false, "Accept all defaults without prompting")
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	flagPath, _ := cmd.Flags().GetString("repo-path")

	// ── 1. Determine repo path ────────────────────────────────────────────────
	repoPath, err := resolveRepoPath(flagPath, yes)
	if err != nil {
		return err
	}

	// ── 2. Clone if the directory does not exist ──────────────────────────────
	if err := ensureCloned(repoPath, yes); err != nil {
		return err
	}

	// ── 3. Save config ────────────────────────────────────────────────────────
	color.Cyan("\n▸ Saving configuration…")

	existingCfg, loadErr := config.Load()
	if loadErr == nil && existingCfg.RepoPath == repoPath {
		color.White("  –  config already up-to-date (%s)", repoPath)
	} else {
		cfg := &config.Config{
			RepoPath: repoPath,
			RepoURL:  config.DefaultRepoURL,
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		p, _ := config.Path()
		color.Green("  ✓  config saved → %s", p)
	}

	// ── 4. Install all artifacts ──────────────────────────────────────────────
	color.Cyan("\n▸ Installing artifacts…")

	providers := []interface {
		Name() string
		Install(string) error
	}{
		&kiro.Provider{},
	}

	anyError := false
	for _, p := range providers {
		color.Cyan("\n  [%s]", p.Name())
		if err := p.Install(repoPath); err != nil {
			color.Red("  ✗  %s install failed: %v", p.Name(), err)
			anyError = true
		}
	}

	fmt.Println()
	if anyError {
		return fmt.Errorf("bootstrap completed with errors — check output above")
	}

	color.Green("✓  Bootstrap complete! Everything is installed and ready.")
	color.White("\n   Useful commands:")
	color.White("     dctx status   — check artifact state")
	color.White("     dctx sync     — pull/push monorepo changes")
	color.White("     dctx update   — update dctx itself")
	return nil
}

// resolveRepoPath returns the path to use for the local clone.
// Priority: --repo-path flag > existing config > default path.
// Only prompts when none of the above apply and -y is not set.
func resolveRepoPath(flagPath string, yes bool) (string, error) {
	if flagPath != "" {
		abs, err := filepath.Abs(flagPath)
		if err != nil {
			return "", fmt.Errorf("invalid --repo-path: %w", err)
		}
		return abs, nil
	}

	// If config already exists and points to a valid directory, reuse it.
	if cfg, err := config.Load(); err == nil && cfg.RepoPath != "" {
		if _, err := os.Stat(cfg.RepoPath); err == nil {
			color.White("  Using existing repo path from config: %s", cfg.RepoPath)
			return cfg.RepoPath, nil
		}
	}

	defaultPath := defaultRepoPath()

	if yes {
		return defaultPath, nil
	}

	// Only prompt if the default path does not already exist as a git repo.
	if isGitRepo(defaultPath) {
		return defaultPath, nil
	}

	// If the default path exists but is NOT a git repo, we need to ask.
	if _, err := os.Stat(defaultPath); err == nil {
		color.Yellow("  ⚠  %s exists but is not a git repository.", defaultPath)
	}

	var chosen string
	if err := survey.AskOne(&survey.Input{
		Message: "Where should the dev-context monorepo be cloned?",
		Default: defaultPath,
		Help:    "Leave blank to use the default path.",
	}, &chosen, survey.WithValidator(survey.Required)); err != nil {
		return "", fmt.Errorf("prompt cancelled: %w", err)
	}

	abs, err := filepath.Abs(chosen)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	return abs, nil
}

// ensureCloned clones the monorepo into repoPath if it is not already there.
func ensureCloned(repoPath string, yes bool) error {
	color.Cyan("\n▸ Checking monorepo at %s…", repoPath)

	if isGitRepo(repoPath) {
		color.White("  –  already cloned, skipping")

		// Pull latest changes silently.
		color.Cyan("  Pulling latest changes…")
		pullCmd := exec.Command("git", "pull", "--ff-only", "origin")
		pullCmd.Dir = repoPath
		out, err := pullCmd.CombinedOutput()
		if err != nil {
			// Non-fatal: if pull fails (e.g. diverged), bootstrap continues
			// and the user can run dctx sync later.
			color.Yellow("  ⚠  Could not pull latest changes: %s", string(out))
			color.Yellow("     Run 'dctx sync' later to reconcile.")
		} else {
			color.Green("  ✓  repo is up-to-date")
		}
		return nil
	}

	// Directory exists but is not a git repo — bail to avoid surprises.
	if _, err := os.Stat(repoPath); err == nil {
		return fmt.Errorf(
			"directory %s already exists but is not a git repository.\n"+
				"Remove it or choose a different path with --repo-path",
			repoPath,
		)
	}

	// Clone.
	// Use -c url.<https>.insteadOf overrides to force HTTPS regardless of any
	// global gitconfig rules (e.g. url.ssh://git@github.com/.insteadOf=https://github.com/)
	// that would rewrite the URL and break cloning on machines without SSH keys.
	cloneURL := config.DefaultRepoURL
	color.Cyan("  Cloning %s into %s…", cloneURL, repoPath)
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	cloneCmd := exec.Command(
		"git",
		"-c", "url.https://github.com/.insteadOf=git@github.com:",
		"-c", "url.https://github.com/.insteadOf=ssh://git@github.com/",
		"clone",
		cloneURL,
		repoPath,
	)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("cloning repository: %w", err)
	}

	color.Green("  ✓  cloned successfully")
	return nil
}

// isGitRepo returns true if the path contains a .git directory.
func isGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return !errors.Is(err, os.ErrNotExist)
}
