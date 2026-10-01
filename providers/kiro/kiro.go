// Package kiro implements the Provider interface for the Kiro AI CLI tool.
package kiro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/emersonkopp/dev-context-cli/internal/provider"
	"github.com/fatih/color"
)

// artifact maps a source path (relative to repoPath) to its destination.
type artifact struct {
	src string // relative to repoPath
	dst string // absolute path
}

// Provider implements provider.Provider for Kiro.
type Provider struct{}

var _ provider.Provider = (*Provider)(nil)

// Name returns the tool name.
func (p *Provider) Name() string { return "Kiro" }

// artifacts returns the full list of managed files for the given repoPath.
func (p *Provider) artifacts(repoPath string) ([]artifact, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	kiroDir := filepath.Join(home, ".kiro")

	// Glob all steering files dynamically — new files added to the repo are
	// picked up automatically without changing this code.
	steeringSrc := filepath.Join(repoPath, "kiro", "steering")
	steeringFiles, err := filepath.Glob(filepath.Join(steeringSrc, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("globbing steering files: %w", err)
	}

	var arts []artifact
	for _, f := range steeringFiles {
		rel, _ := filepath.Rel(repoPath, f)
		arts = append(arts, artifact{
			src: rel,
			dst: filepath.Join(kiroDir, "steering", filepath.Base(f)),
		})
	}

	// Glob all skills dynamically. Each skill is a directory under
	// kiro/skills/ containing a SKILL.md (and optionally other files).
	// New skills added to the repo are picked up automatically.
	skillsSrc := filepath.Join(repoPath, "kiro", "skills")
	skillFiles, err := filepath.Glob(filepath.Join(skillsSrc, "*", "SKILL.md"))
	if err != nil {
		return nil, fmt.Errorf("globbing skill files: %w", err)
	}
	for _, f := range skillFiles {
		rel, _ := filepath.Rel(repoPath, f)
		skillName := filepath.Base(filepath.Dir(f))
		arts = append(arts, artifact{
			src: rel,
			dst: filepath.Join(kiroDir, "skills", skillName, "SKILL.md"),
		})
	}

	// Settings
	arts = append(arts, artifact{
		src: filepath.Join("kiro", "settings", "cli.json"),
		dst: filepath.Join(kiroDir, "settings", "cli.json"),
	})

	return arts, nil
}

// Install copies all Kiro artifacts from the monorepo to their destinations.
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

		// cli.json: merge so existing keys (e.g. credentials) are preserved.
		// All other files: plain copy.
		if filepath.Base(src) == "cli.json" {
			if err := mergeJSONFiles(src, a.dst); err != nil {
				return fmt.Errorf("merging %s: %w", src, err)
			}
			color.Green("  ✓  merged     %s", prettyPath(a.dst))
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
	// the repo (e.g. renamed or deleted steerings/skills). This only touches
	// files recorded in dctx's own manifest — files not installed by dctx
	// (such as organization-specific local skills) are never removed.
	if err := p.pruneOrphans(arts); err != nil {
		return fmt.Errorf("pruning orphaned artifacts: %w", err)
	}

	return nil
}

// manifestPath returns the path to dctx's install manifest for Kiro,
// stored under ~/.dev-context/kiro-manifest.json.
func manifestPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".dev-context", "kiro-manifest.json"), nil
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
// Used to clean up skill directories (~/.kiro/skills/<name>/) after their
// SKILL.md is pruned. Silently ignores non-empty directories and errors.
func removeEmptyParent(path string) {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
}

// Status reports the installation state of all Kiro artifacts.
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
			// cli.json is installed via merge (re-indented, may keep extra
			// local keys), so byte-equality is the wrong check. It is
			// up-to-date when every source key is already present in the
			// destination with an equal value.
			if filepath.Base(a.dst) == "cli.json" {
				satisfied, err := jsonMergeSatisfied(src, a.dst)
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

// ── helpers ───────────────────────────────────────────────────────────────────

// copyFile copies src to dst. Returns true if the file was actually written.
func copyFile(src, dst string) (changed bool, err error) {
	srcData, err := os.ReadFile(src)
	if err != nil {
		return false, err
	}
	if dstData, err := os.ReadFile(dst); err == nil {
		if sha256hex(srcData) == sha256hex(dstData) {
			return false, nil // already identical
		}
	}
	return true, os.WriteFile(dst, srcData, 0o644)
}

// mergeJSONFiles adds keys from src into dst without overwriting existing keys.
func mergeJSONFiles(srcPath, dstPath string) error {
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading source: %w", err)
	}
	var srcMap map[string]interface{}
	if err := json.Unmarshal(srcData, &srcMap); err != nil {
		return fmt.Errorf("parsing source JSON: %w", err)
	}

	dstMap := make(map[string]interface{})
	if dstData, err := os.ReadFile(dstPath); err == nil {
		_ = json.Unmarshal(dstData, &dstMap)
	}

	for k, v := range srcMap {
		if _, exists := dstMap[k]; !exists {
			dstMap[k] = v
		}
	}

	out, err := json.MarshalIndent(dstMap, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding merged JSON: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dstPath, append(out, '\n'), 0o644)
}

func filesEqual(a, b string) (bool, error) {
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return sha256hex(da) == sha256hex(db), nil
}

// jsonMergeSatisfied reports whether every top-level key in the source JSON is
// already present in the destination JSON with a deeply-equal value. This is
// the correct "up-to-date" check for files installed via mergeJSONFiles: the
// destination may contain extra local keys (e.g. credentials) and be formatted
// differently, yet still be fully up-to-date with respect to the source.
func jsonMergeSatisfied(srcPath, dstPath string) (bool, error) {
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return false, err
	}
	dstData, err := os.ReadFile(dstPath)
	if err != nil {
		return false, err
	}
	var srcMap, dstMap map[string]interface{}
	if err := json.Unmarshal(srcData, &srcMap); err != nil {
		return false, fmt.Errorf("parsing source JSON: %w", err)
	}
	if err := json.Unmarshal(dstData, &dstMap); err != nil {
		return false, fmt.Errorf("parsing destination JSON: %w", err)
	}
	for k, want := range srcMap {
		got, ok := dstMap[k]
		if !ok || !reflect.DeepEqual(want, got) {
			return false, nil
		}
	}
	return true, nil
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// prettyPath shortens the home directory to ~ for display.
func prettyPath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" {
		if rel, err := filepath.Rel(home, path); err == nil {
			return "~/" + rel
		}
	}
	return path
}
