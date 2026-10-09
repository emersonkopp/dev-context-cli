package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupRepo creates an isolated HOME and a monorepo with a tools/ dir holding
// one valid tool manifest. Returns the repo path.
func setupRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	repo := filepath.Join(tmp, "repo")
	manifest := `{
  "name": "kiro-ai-router",
  "description": "Local-first MCP router",
  "install": "scripts/install.sh",
  "installed_marker": ".local/share/kiro-ai-router/app/.venv"
}`
	write(t, filepath.Join(repo, "tools", "kiro-ai-router", "tool.json"), manifest)
	write(t, filepath.Join(repo, "tools", "kiro-ai-router", "scripts", "install.sh"), "#!/usr/bin/env bash\necho ok\n")
	return repo
}

func TestDiscoverFindsTools(t *testing.T) {
	repo := setupRepo(t)
	list, err := Discover(repo)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(list))
	}
	tool := list[0]
	if tool.Name != "kiro-ai-router" {
		t.Errorf("name = %q, want kiro-ai-router", tool.Name)
	}
	if tool.Description != "Local-first MCP router" {
		t.Errorf("description = %q", tool.Description)
	}
	if tool.Install != "scripts/install.sh" {
		t.Errorf("install = %q", tool.Install)
	}
}

func TestDiscoverEmptyWhenNoToolsDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	list, err := Discover(filepath.Join(tmp, "repo-without-tools"))
	if err != nil {
		t.Fatalf("discover should not error on missing tools dir: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(list))
	}
}

func TestDiscoverSkipsInvalidManifest(t *testing.T) {
	repo := setupRepo(t)
	// Add a dir with a broken manifest — must be skipped, not fatal.
	write(t, filepath.Join(repo, "tools", "broken", "tool.json"), "{ not valid json")
	list, err := Discover(repo)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	for _, tool := range list {
		if tool.Name == "broken" {
			t.Error("broken tool should have been skipped")
		}
	}
}

func TestIsInstalledReflectsMarker(t *testing.T) {
	repo := setupRepo(t)
	home, _ := os.UserHomeDir()
	list, _ := Discover(repo)
	tool := list[0]

	if tool.IsInstalled() {
		t.Error("tool should not be installed before marker exists")
	}

	// Create the marker under HOME.
	marker := filepath.Join(home, ".local", "share", "kiro-ai-router", "app", ".venv")
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	if !tool.IsInstalled() {
		t.Error("tool should be installed once marker exists")
	}
}

func TestFindByNameReturnsError(t *testing.T) {
	repo := setupRepo(t)
	_, err := Find(repo, "does-not-exist")
	if err == nil {
		t.Error("expected error for unknown tool name")
	}
	tool, err := Find(repo, "kiro-ai-router")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.Name != "kiro-ai-router" {
		t.Errorf("got %q", tool.Name)
	}
}

// InstallScriptPath must resolve to an absolute path inside the tool dir and
// must reject manifests that try to escape the tool directory (path traversal).
func TestInstallScriptPathIsSafe(t *testing.T) {
	repo := setupRepo(t)
	tool, _ := Find(repo, "kiro-ai-router")
	p, err := tool.InstallScriptPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(repo, "tools", "kiro-ai-router", "scripts", "install.sh")
	if p != want {
		t.Errorf("path = %q, want %q", p, want)
	}

	// Traversal attempt must be rejected.
	write(t, filepath.Join(repo, "tools", "evil", "tool.json"),
		`{"name":"evil","install":"../../../../etc/evil.sh","installed_marker":"x"}`)
	evil, _ := Find(repo, "evil")
	if _, err := evil.InstallScriptPath(); err == nil {
		t.Error("expected path-traversal install path to be rejected")
	}
}
