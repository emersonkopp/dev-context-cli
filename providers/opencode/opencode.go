// Package opencode implements the Provider interface for the OpenCode V2 CLI tool.
//
// Strategy (per docs/plan-port-opencode.md of the dev-context monorepo):
//   - Skills variantes (opencode/skills/*/SKILL.md) are copied to
//     ~/.config/opencode/skills/<id>/SKILL.md, with orphan pruning driven by the
//     dctx manifest (same model as the kiro provider).
//   - Steerings are NOT copied (strategy A): they are reused directly from the
//     monorepo via the "instructions" array in the global OpenCode config.
//   - The reference config opencode/config/opencode.reference.jsonc (JSONC, with
//     comments) is merged into the global OpenCode config (~/.config/opencode/
//     opencode.json or .jsonc) idempotently, without overwriting third-party
//     entries: existing local keys are preserved, managed entries are deduped,
//     and an already-existing mcp.servers.ai-router entry is never overwritten.
//   - Monorepo paths inside skills/instructions are rebased to the real clone
//     path (from ~/.dev-context/config.json) and written expanded, so a
//     monorepo cloned outside ~/git/dev-context still works.
package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emersonkopp/dev-context-cli/internal/provider"
	"github.com/fatih/color"
)

// artifact maps a source path (relative to repoPath) to its destination.
type artifact struct {
	src string // relative to repoPath
	dst string // absolute path
}

// Provider implements provider.Provider for OpenCode.
type Provider struct{}

var _ provider.Provider = (*Provider)(nil)

// Name returns the tool name.
func (p *Provider) Name() string { return "OpenCode" }

// opencodeDir returns ~/.config/opencode. OpenCode reads its global config
// from there on every platform (XDG), including macOS — os.UserConfigDir()
// would be wrong (~/$HOME/Library/Application Support on macOS).
func opencodeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "opencode"), nil
}

// configDst returns the global OpenCode config file to merge into. An existing
// file is preferred (opencode.json, then opencode.jsonc); when neither exists,
// opencode.json is created.
func configDst(ocDir string) string {
	if _, err := os.Stat(filepath.Join(ocDir, "opencode.json")); err == nil {
		return filepath.Join(ocDir, "opencode.json")
	}
	if _, err := os.Stat(filepath.Join(ocDir, "opencode.jsonc")); err == nil {
		return filepath.Join(ocDir, "opencode.jsonc")
	}
	return filepath.Join(ocDir, "opencode.json")
}

// artifacts returns the full list of managed files for the given repoPath.
func (p *Provider) artifacts(repoPath string) ([]artifact, error) {
	ocDir, err := opencodeDir()
	if err != nil {
		return nil, err
	}

	var arts []artifact

	// Glob all skills dynamically. Each skill is a directory under
	// opencode/skills/ containing a SKILL.md. New skills added to the repo
	// are picked up automatically.
	skillsSrc := filepath.Join(repoPath, "opencode", "skills")
	skillFiles, err := filepath.Glob(filepath.Join(skillsSrc, "*", "SKILL.md"))
	if err != nil {
		return nil, fmt.Errorf("globbing skill files: %w", err)
	}
	for _, f := range skillFiles {
		rel, _ := filepath.Rel(repoPath, f)
		skillName := filepath.Base(filepath.Dir(f))
		arts = append(arts, artifact{
			src: rel,
			dst: filepath.Join(ocDir, "skills", skillName, "SKILL.md"),
		})
	}

	// Reference config (JSONC with comments) — merged, never plain-copied.
	refPath := filepath.Join(repoPath, "opencode", "config", "opencode.reference.jsonc")
	if _, err := os.Stat(refPath); err == nil {
		rel, _ := filepath.Rel(repoPath, refPath)
		arts = append(arts, artifact{
			src: rel,
			dst: configDst(ocDir),
		})
	}

	return arts, nil
}

// Install copies all OpenCode artifacts from the monorepo to their destinations.
// Steerings are not copied — they are reused via the merged "instructions"
// entry in the global config (strategy A of the port plan).
func (p *Provider) Install(repoPath string) error {
	arts, err := p.artifacts(repoPath)
	if err != nil {
		return err
	}

	for _, a := range arts {
		src := filepath.Join(repoPath, a.src)

		if err := os.MkdirAll(filepath.Dir(a.dst), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", a.dst, err)
		}

		// Reference config: merge idempotently; existing third-party keys
		// and entries are preserved. Other files: plain copy.
		if strings.HasSuffix(src, ".reference.jsonc") {
			changed, err := mergeReferenceConfig(repoPath, a.dst)
			if err != nil {
				return fmt.Errorf("merging %s: %w", src, err)
			}
			if changed {
				color.Green("  ✓  merged     %s", prettyPath(a.dst))
			} else {
				color.White("  –  up-to-date %s", prettyPath(a.dst))
			}
		} else {
			changed, err := copyFile(src, a.dst)
			if err != nil {
				return fmt.Errorf("copying %s: %w", src, err)
			}
			if changed {
				color.Green("  ✓  installed  %s", prettyPath(a.dst))
			} else {
				color.White("  –  up-to-date %s", prettyPath(a.dst))
			}
		}
	}

	// Remove artifacts previously installed by dctx that no longer exist in
	// the repo — same conservative model as the kiro provider.
	if err := p.pruneOrphans(arts); err != nil {
		return fmt.Errorf("pruning orphaned artifacts: %w", err)
	}

	return nil
}

// Uninstall removes every artifact dctx installed for OpenCode, as recorded in
// the manifest. It is deliberately conservative:
//   - Only paths in dctx's own manifest are touched; files dctx never installed
//     (e.g. organization-local skills) are preserved.
//   - The merged global config (opencode.json/jsonc) is NOT deleted: it may
//     contain the user's own keys. dctx simply stops managing it — the same
//     policy applied to ~/.kiro/settings/cli.json.
//
// After removing the artifacts it deletes the manifest so a later install
// starts from a clean slate. A missing manifest is a no-op (no error).
func (p *Provider) Uninstall() error {
	previous, err := readManifest()
	if err != nil {
		return err
	}
	if len(previous) == 0 {
		return nil // nothing recorded — nothing to remove
	}

	for _, dst := range previous {
		// Never delete the merged global config: it may hold the user's own keys.
		if isMergedConfig(dst) {
			color.White("  –  kept       %s (merged file, may hold your keys)", prettyPath(dst))
			continue
		}
		if _, statErr := os.Stat(dst); os.IsNotExist(statErr) {
			continue // already gone
		}
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("removing %s: %w", dst, err)
		}
		color.Yellow("  ⊖  removed    %s", prettyPath(dst))
		// Clean up a now-empty skill directory (~/.config/opencode/skills/<name>/).
		removeEmptyParent(dst)
	}

	// Drop the manifest so a future install starts clean.
	mp, err := manifestPath()
	if err != nil {
		return err
	}
	if err := os.Remove(mp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing manifest: %w", err)
	}
	return nil
}

// isMergedConfig reports whether path is the merged OpenCode global config.
func isMergedConfig(path string) bool {
	base := filepath.Base(path)
	return base == "opencode.json" || base == "opencode.jsonc"
}

// manifestPath returns the path to dctx's install manifest for OpenCode,
// stored under ~/.dev-context/opencode-manifest.json.
func manifestPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".dev-context", "opencode-manifest.json"), nil
}

// readManifest returns the list of destination paths dctx installed last time.
// A missing manifest yields an empty list (no error) — first run.
func readManifest() ([]string, error) {
	p, err := manifestPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	var dsts []string
	if err := json.Unmarshal(data, &dsts); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return dsts, nil
}

// writeManifest persists the destination paths dctx installed this run.
func writeManifest(dsts []string) error {
	p, err := manifestPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("creating manifest dir: %w", err)
	}
	data, err := json.MarshalIndent(dsts, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding manifest: %w", err)
	}
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// pruneOrphans removes destination files that dctx installed previously but
// that are no longer part of the current artifact set (renamed or deleted in
// the repo). It is intentionally conservative: it only ever removes paths
// recorded in dctx's own manifest. Files that dctx never installed — such as
// organization-specific local skills — are left untouched. After writing, it
// refreshes the manifest with the current destinations.
func (p *Provider) pruneOrphans(current []artifact) error {
	previous, err := readManifest()
	if err != nil {
		return err
	}

	currentSet := make(map[string]struct{}, len(current))
	for _, a := range current {
		currentSet[a.dst] = struct{}{}
	}

	for _, old := range previous {
		if _, stillManaged := currentSet[old]; stillManaged {
			continue
		}
		// Orphan: dctx installed it before, repo no longer provides it.
		if _, statErr := os.Stat(old); os.IsNotExist(statErr) {
			continue // already gone
		}
		if err := os.Remove(old); err != nil {
			return fmt.Errorf("removing orphan %s: %w", old, err)
		}
		color.Yellow("  ⊖  removed    %s (orphaned)", prettyPath(old))
		// If this left an empty skill directory, clean it up too.
		removeEmptyParent(old)
	}

	// Record the current state for the next run.
	dsts := make([]string, 0, len(current))
	for _, a := range current {
		dsts = append(dsts, a.dst)
	}
	return writeManifest(dsts)
}

// removeEmptyParent removes the parent directory of path if it became empty.
// Used to clean up skill directories (~/.config/opencode/skills/<name>/) after
// their SKILL.md is pruned. Silently ignores non-empty directories and errors.
func removeEmptyParent(path string) {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
}

// Status reports the installation state of all OpenCode artifacts.
func (p *Provider) Status(repoPath string) ([]provider.ArtifactStatus, error) {
	arts, err := p.artifacts(repoPath)
	if err != nil {
		return nil, err
	}

	var statuses []provider.ArtifactStatus
	for _, a := range arts {
		src := filepath.Join(repoPath, a.src)
		st := provider.ArtifactStatus{
			Name:    a.src,
			SrcPath: src,
			DstPath: a.dst,
		}

		if _, err := os.Stat(a.dst); os.IsNotExist(err) {
			st.Installed = false
			st.UpToDate = false
		} else {
			st.Installed = true
			// The reference config is installed via a merge that must not
			// overwrite third-party entries, so byte-equality is the wrong
			// check. It is up-to-date when every managed entry is present
			// in the destination (deduped, in the right order).
			if strings.HasSuffix(src, ".reference.jsonc") {
				satisfied, err := mergeSatisfied(repoPath, a.dst)
				st.UpToDate = err == nil && satisfied
			} else {
				equal, err := filesEqual(src, a.dst)
				st.UpToDate = err == nil && equal
			}
		}
		statuses = append(statuses, st)
	}
	return statuses, nil
}
