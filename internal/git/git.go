// Package git wraps git operations needed by the sync command.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// run executes a git command inside dir and returns combined output.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// HasUncommittedChanges returns true when the working tree is dirty.
func HasUncommittedChanges(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// LocalBranch returns the current branch name.
func LocalBranch(dir string) (string, error) {
	return run(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// Fetch fetches from origin without merging.
func Fetch(dir string) error {
	_, err := run(dir, "fetch", "origin")
	return err
}

// LocalAhead returns the number of commits local is ahead of origin/<branch>.
func LocalAhead(dir, branch string) (int, error) {
	out, err := run(dir, "rev-list", "--count", fmt.Sprintf("origin/%s..HEAD", branch))
	if err != nil {
		return 0, err
	}
	var n int
	fmt.Sscanf(out, "%d", &n)
	return n, nil
}

// RemoteAhead returns the number of commits origin/<branch> is ahead of local.
func RemoteAhead(dir, branch string) (int, error) {
	out, err := run(dir, "rev-list", "--count", fmt.Sprintf("HEAD..origin/%s", branch))
	if err != nil {
		return 0, err
	}
	var n int
	fmt.Sscanf(out, "%d", &n)
	return n, nil
}

// Pull performs a fast-forward merge from origin.
func Pull(dir string) error {
	_, err := run(dir, "pull", "--ff-only", "origin")
	return err
}

// PullRebase rebases local commits on top of origin.
func PullRebase(dir string) error {
	_, err := run(dir, "pull", "--rebase", "origin")
	return err
}

// ResetHard resets the working tree to origin/<branch>, discarding local changes.
func ResetHard(dir, branch string) error {
	_, err := run(dir, "reset", "--hard", fmt.Sprintf("origin/%s", branch))
	return err
}

// AddAll stages all tracked and untracked files.
func AddAll(dir string) error {
	_, err := run(dir, "add", "-A")
	return err
}

// Commit creates a commit with the given message.
func Commit(dir, message string) error {
	_, err := run(dir, "commit", "-m", message)
	return err
}

// Push pushes the current branch to origin.
func Push(dir, branch string) error {
	_, err := run(dir, "push", "origin", branch)
	return err
}

// HasConflicts returns true when the repo is in a conflicted state.
func HasConflicts(dir string) (bool, error) {
	out, err := run(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// ConflictedFiles returns the list of files with merge conflicts.
func ConflictedFiles(dir string) ([]string, error) {
	out, err := run(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// LastCommitHash returns the short hash of the latest commit.
func LastCommitHash(dir string) (string, error) {
	return run(dir, "rev-parse", "--short", "HEAD")
}

// RemoteURL returns the URL of the origin remote.
func RemoteURL(dir string) (string, error) {
	return run(dir, "remote", "get-url", "origin")
}
