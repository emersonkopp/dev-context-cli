package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/emersonkopp/dev-context-cli/internal/config"
	internalgit "github.com/emersonkopp/dev-context-cli/internal/git"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync the local monorepo with the remote (pull + push)",
	Long: `Synchronises the local dev-context monorepo with its GitHub remote.

Behaviour:
  1. Fetches the latest changes from origin.
  2. If there are no divergences, fast-forwards and exits.
  3. If both sides have changes, asks how to resolve:
       • prefer-remote  — resets local to origin (local changes are discarded)
       • prefer-local   — force-pushes local state to origin
       • merge          — opens your editor to resolve conflicts manually

Any uncommitted local changes are committed automatically before syncing
(with an auto-generated message).`,
	RunE: runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("%w\n\nRun 'dctx config init' to set up the CLI first.", err)
	}

	dir := cfg.RepoPath

	// 1. Make sure there is a git repo here.
	if _, err := os.Stat(fmt.Sprintf("%s/.git", dir)); os.IsNotExist(err) {
		return fmt.Errorf("no git repository found at %s", dir)
	}

	branch, err := internalgit.LocalBranch(dir)
	if err != nil {
		return fmt.Errorf("getting branch: %w", err)
	}
	color.Cyan("▸ Syncing branch '%s' in %s", branch, dir)

	// 2. Auto-commit any local changes.
	dirty, err := internalgit.HasUncommittedChanges(dir)
	if err != nil {
		return fmt.Errorf("checking working tree: %w", err)
	}
	if dirty {
		color.Yellow("  Local changes detected — committing before sync…")
		if err := internalgit.AddAll(dir); err != nil {
			return fmt.Errorf("staging files: %w", err)
		}
		msg := fmt.Sprintf("chore: auto-commit before sync (%s)", time.Now().Format("2006-01-02 15:04:05"))
		if err := internalgit.Commit(dir, msg); err != nil {
			return fmt.Errorf("committing: %w", err)
		}
		color.Green("  ✓  committed local changes")
	}

	// 3. Fetch.
	color.Cyan("  Fetching from origin…")
	if err := internalgit.Fetch(dir); err != nil {
		return fmt.Errorf("fetching: %w", err)
	}

	localAhead, err := internalgit.LocalAhead(dir, branch)
	if err != nil {
		return fmt.Errorf("checking local ahead: %w", err)
	}
	remoteAhead, err := internalgit.RemoteAhead(dir, branch)
	if err != nil {
		return fmt.Errorf("checking remote ahead: %w", err)
	}

	color.White("  local ahead: %d  |  remote ahead: %d", localAhead, remoteAhead)

	switch {
	case localAhead == 0 && remoteAhead == 0:
		color.Green("  ✓  Already up-to-date.")
		return nil

	case localAhead == 0 && remoteAhead > 0:
		// Only remote has changes — fast-forward.
		color.Cyan("  Fast-forwarding from origin…")
		if err := internalgit.Pull(dir); err != nil {
			return fmt.Errorf("pulling: %w", err)
		}
		color.Green("  ✓  Pulled %d commit(s) from origin.", remoteAhead)
		return nil

	case localAhead > 0 && remoteAhead == 0:
		// Only local has changes — push.
		color.Cyan("  Pushing %d commit(s) to origin…", localAhead)
		if err := internalgit.Push(dir, branch); err != nil {
			return fmt.Errorf("pushing: %w", err)
		}
		color.Green("  ✓  Pushed successfully.")
		return nil

	default:
		// Both sides diverged — ask the user.
		return handleDiverged(dir, branch, localAhead, remoteAhead)
	}
}

// handleDiverged presents an interactive menu when both sides have commits.
func handleDiverged(dir, branch string, localAhead, remoteAhead int) error {
	color.Yellow("\n  ⚠  Diverged: local has %d commit(s), remote has %d commit(s).", localAhead, remoteAhead)

	var choice string
	prompt := &survey.Select{
		Message: "How do you want to resolve the divergence?",
		Options: []string{
			"merge          — rebase local on top of remote (recommended)",
			"prefer-remote  — discard local commits, reset to remote",
			"prefer-local   — overwrite remote with local (force push)",
			"abort          — do nothing, resolve manually later",
		},
	}
	if err := survey.AskOne(prompt, &choice); err != nil {
		return fmt.Errorf("prompt cancelled: %w", err)
	}

	switch {
	case startsWith(choice, "merge"):
		return doMerge(dir, branch)
	case startsWith(choice, "prefer-remote"):
		return doPreferRemote(dir, branch)
	case startsWith(choice, "prefer-local"):
		return doPreferLocal(dir, branch)
	default:
		color.Yellow("  Aborted. No changes made.")
		return nil
	}
}

func doMerge(dir, branch string) error {
	color.Cyan("  Rebasing local commits on top of remote…")
	if err := internalgit.PullRebase(dir); err != nil {
		// Check for conflicts.
		conflicted, _ := internalgit.HasConflicts(dir)
		if conflicted {
			files, _ := internalgit.ConflictedFiles(dir)
			color.Red("  ✗  Merge conflicts in:")
			for _, f := range files {
				color.Red("       %s", f)
			}
			color.Yellow("\n  Resolve conflicts, then run:")
			color.White("    git -C %s rebase --continue", dir)
			color.White("    dctx sync")
			return fmt.Errorf("merge conflicts — resolve manually and re-run 'dctx sync'")
		}
		return fmt.Errorf("rebase failed: %w", err)
	}

	color.Cyan("  Pushing rebased commits…")
	if err := internalgit.Push(dir, branch); err != nil {
		return fmt.Errorf("pushing after rebase: %w", err)
	}
	color.Green("  ✓  Synced via rebase.")
	return nil
}

func doPreferRemote(dir, branch string) error {
	var confirm bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "This will DISCARD your local commits. Are you sure?",
		Default: false,
	}, &confirm); err != nil || !confirm {
		color.Yellow("  Aborted.")
		return nil
	}
	if err := internalgit.ResetHard(dir, branch); err != nil {
		return fmt.Errorf("resetting to remote: %w", err)
	}
	color.Green("  ✓  Reset to remote. Local commits discarded.")
	return nil
}

func doPreferLocal(dir, branch string) error {
	var confirm bool
	if err := survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("This will OVERWRITE the remote '%s' branch. Are you sure?", branch),
		Default: false,
	}, &confirm); err != nil || !confirm {
		color.Yellow("  Aborted.")
		return nil
	}

	// Use git directly for force push to keep internal/git clean.
	cmd := exec.Command("git", "push", "--force-with-lease", "origin", branch)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("force push failed: %w", err)
	}
	color.Green("  ✓  Force-pushed local state to remote.")
	return nil
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
