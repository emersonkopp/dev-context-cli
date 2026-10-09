// Package tools discovers and inspects installable tools declared in the
// monorepo under tools/<name>/tool.json. It never executes anything: callers
// (the cmd layer) decide when to run a tool's installer, explicitly.
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Tool is a single installable tool declared by a tool.json manifest.
type Tool struct {
	// Name is the tool identifier (should match its directory name).
	Name string `json:"name"`
	// Description is a short human-readable summary.
	Description string `json:"description"`
	// Install is the installer script path, relative to the tool directory.
	Install string `json:"install"`
	// InstalledMarker is a path relative to $HOME whose existence indicates
	// the tool is installed on this machine.
	InstalledMarker string `json:"installed_marker"`

	// dir is the absolute path to the tool directory (not serialized).
	dir string
}

// Discover scans repoPath/tools/*/tool.json and returns the valid manifests.
// A missing tools/ directory yields an empty list (not an error). Invalid or
// unreadable manifests are skipped so one bad tool cannot break the command.
func Discover(repoPath string) ([]Tool, error) {
	toolsDir := filepath.Join(repoPath, "tools")
	entries, err := os.ReadDir(toolsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading tools dir: %w", err)
	}

	var out []Tool
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(toolsDir, e.Name())
		manifestPath := filepath.Join(dir, "tool.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // no manifest or unreadable: not a managed tool
		}
		var tool Tool
		if err := json.Unmarshal(data, &tool); err != nil {
			continue // invalid manifest: skip, don't fail the whole command
		}
		if tool.Name == "" {
			tool.Name = e.Name()
		}
		tool.dir = dir
		out = append(out, tool)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Find returns the tool with the given name, or an error if not found.
func Find(repoPath, name string) (Tool, error) {
	list, err := Discover(repoPath)
	if err != nil {
		return Tool{}, err
	}
	for _, tool := range list {
		if tool.Name == name {
			return tool, nil
		}
	}
	return Tool{}, fmt.Errorf("tool %q not found in %s/tools", name, repoPath)
}

// IsInstalled reports whether the tool's installed marker exists under $HOME.
// If no marker is declared, it conservatively returns false.
func (t Tool) IsInstalled() bool {
	if t.InstalledMarker == "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, t.InstalledMarker))
	return err == nil
}

// Dir returns the absolute tool directory.
func (t Tool) Dir() string { return t.dir }

// InstallScriptPath resolves the installer path and guarantees it stays inside
// the tool directory (rejects path traversal in the manifest).
func (t Tool) InstallScriptPath() (string, error) {
	if t.Install == "" {
		return "", fmt.Errorf("tool %q declares no install script", t.Name)
	}
	if t.dir == "" {
		return "", fmt.Errorf("tool %q has no resolved directory", t.Name)
	}
	resolved := filepath.Clean(filepath.Join(t.dir, t.Install))
	// Must remain within the tool directory.
	rel, err := filepath.Rel(t.dir, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("install path %q escapes the tool directory", t.Install)
	}
	return resolved, nil
}
