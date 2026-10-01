// Package kiro implements the Provider interface for the Kiro AI CLI tool.
package kiro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	return nil
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
			equal, err := filesEqual(src, a.dst)
			st.UpToDate = err == nil && equal
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
