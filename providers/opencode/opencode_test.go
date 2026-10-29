package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupEnv creates an isolated HOME and a minimal monorepo, returning the repo
// path. It points os.UserHomeDir() at a temp dir via t.Setenv("HOME", ...).
func setupEnv(t *testing.T) (repoPath string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	repo := filepath.Join(home, "git", "dev-context") // canonical clone location
	for _, d := range []string{
		filepath.Join(repo, "opencode", "skills", "dev-context-manager"),
		filepath.Join(repo, "opencode", "config"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(repo, "opencode", "skills", "dev-context-manager", "SKILL.md"),
		"---\nname: dev-context-manager\ndescription: variante opencode\n---\ncorpo")
	write(t, filepath.Join(repo, "opencode", "config", "opencode.reference.jsonc"), referenceJSONC)
	return repo
}

// referenceJSONC mirrors opencode/config/opencode.reference.jsonc of the
// monorepo: comments (// and /* */), trailing commas, URLs inside strings.
const referenceJSONC = `// dev-context — config de referência para o OpenCode V2
{
  "$schema": "https://opencode.ai/config.json",

  // Ordem importa: em ID igual, a fonte registrada depois vence.
  "skills": [
    "~/git/dev-context/kiro/skills",      // reuso das 14 agnósticas
    "~/git/dev-context/opencode/skills",  // variantes
  ],

  // Estratégia A: glob ~ em instructions; nada é copiado.
  "instructions": [
    "~/git/dev-context/kiro/steering/*.md",
  ],

  "mcp": {
    "servers": {
      "ai-router": {
        "type": "local",
        "command": ["{env:HOME}/.local/share/kiro-ai-router/app/.venv/bin/python", "-m", "kiro_ai_router.server"],
        /* block comment */ "env": {},
      },
    },
  },
}
`

// mustExist / mustNotExist / write mirror the kiro provider tests.
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to exist, got: %v", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s to NOT exist", path)
	}
}

// TestInstallCopiesSkillsAndMergesConfig verifies the two artifact kinds.
func TestInstallCopiesSkillsAndMergesConfig(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	mustExist(t, filepath.Join(home, ".config", "opencode", "skills", "dev-context-manager", "SKILL.md"))

	raw, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatalf("reading merged config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("merged config is not valid JSON: %v\n%s", err, raw)
	}
	if cfg["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("$schema missing/wrong: %v", cfg["$schema"])
	}
	skills := toStrings(t, cfg["skills"])
	// Managed entries must be expanded (plan: "grava expandido"), in order.
	wantSkills := []string{
		filepath.Join(repo, "kiro", "skills"),
		filepath.Join(repo, "opencode", "skills"),
	}
	if len(skills) != len(wantSkills) {
		t.Fatalf("skills = %v, want %v", skills, wantSkills)
	}
	for i, w := range wantSkills {
		if skills[i] != w {
			t.Errorf("skills[%d] = %q, want %q", i, skills[i], w)
		}
	}
	instructions := toStrings(t, cfg["instructions"])
	if len(instructions) != 1 || instructions[0] != filepath.Join(repo, "kiro", "steering", "*.md") {
		t.Errorf("instructions = %v", instructions)
	}
	mcp, ok := cfg["mcp"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcp missing: %v", cfg["mcp"])
	}
	servers := mcp["servers"].(map[string]interface{})
	router, ok := servers["ai-router"].(map[string]interface{})
	if !ok {
		t.Fatalf("ai-router missing: %v", servers)
	}
	// {env:HOME} must be preserved — OpenCode expands it itself.
	cmd := toStrings(t, router["command"])
	if cmd[0] != "{env:HOME}/.local/share/kiro-ai-router/app/.venv/bin/python" {
		t.Errorf("ai-router command = %v", cmd)
	}
}

// TestInstallMergePreservesThirdPartyEntries verifies that keys and entries the
// user had before (other tools' skills, other MCP servers, colors, models etc.)
// are untouched, and that an existing ai-router entry is not overwritten.
func TestInstallMergePreservesThirdPartyEntries(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	dst := filepath.Join(home, ".config", "opencode", "opencode.json")
	write(t, dst, `{
  "theme": "custom-dark",
  "model": "xai/grok-code",
  "skills": ["/third-party/skills", "~/git/dev-context/kiro/skills"],
  "mcp": { "servers": { "other-server": { "type": "local", "command": ["/bin/true"] } } }
}`)

	p := &Provider{}
	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	raw, _ := os.ReadFile(dst)
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("merged config invalid: %v\n%s", err, raw)
	}
	if cfg["theme"] != "custom-dark" || cfg["model"] != "xai/grok-code" {
		t.Errorf("third-party top-level keys lost: %v", cfg)
	}
	skills := toStrings(t, cfg["skills"])
	if len(skills) != 3 || skills[0] != "/third-party/skills" {
		t.Errorf("skills = %v — third-party entry must come first and be kept", skills)
	}
	// The duplicate tilde-form managed entry is replaced by the expanded one.
	for _, s := range skills[1:] {
		if strings.HasPrefix(s, "~") || strings.Contains(s, "git/dev-context") && !strings.HasPrefix(s, filepath.Join(home, "git", "dev-context")) {
			t.Errorf("unexpected managed entry form: %q", s)
		}
	}
	servers := cfg["mcp"].(map[string]interface{})["servers"].(map[string]interface{})
	if _, ok := servers["other-server"]; !ok {
		t.Errorf("other-server lost: %v", servers)
	}
	router, ok := servers["ai-router"].(map[string]interface{})
	if !ok {
		t.Fatal("ai-router should have been added alongside other-server")
	}
	if cmd := toStrings(t, router["command"]); cmd[0] != "{env:HOME}/.local/share/kiro-ai-router/app/.venv/bin/python" {
		t.Errorf("ai-router = %v", router)
	}

	// Re-install must not overwrite the user's custom ai-router command.
	write(t, dst, `{
  "mcp": { "servers": { "ai-router": { "type": "local", "command": ["/custom/python"] } } }
}`)
	if err := p.Install(repo); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	raw, _ = os.ReadFile(dst)
	var cfg2 map[string]interface{}
	if err := json.Unmarshal(raw, &cfg2); err != nil {
		t.Fatalf("reinstall config invalid: %v", err)
	}
	servers2 := cfg2["mcp"].(map[string]interface{})["servers"].(map[string]interface{})
	if cmd := toStrings(t, servers2["ai-router"].(map[string]interface{})["command"]); cmd[0] != "/custom/python" {
		t.Errorf("existing ai-router entry was overwritten: %v", cmd)
	}
	// Managed arrays must still be (re)created even when mcp was user-owned.
	if skills := toStrings(t, cfg2["skills"]); len(skills) != 2 {
		t.Errorf("skills after reinstall = %v", skills)
	}
}

// TestInstallOnJSONCDestination verifies merging into an existing
// opencode.jsonc (comments tolerated on read; file type is preserved).
func TestInstallOnJSONCDestination(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	dst := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	write(t, dst, `{
  // user's local comment
  "theme": "night",
}`)

	p := &Provider{}
	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	mustNotExist(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	raw, _ := os.ReadFile(dst)
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("merged .jsonc not valid JSON: %v\n%s", err, raw)
	}
	if cfg["theme"] != "night" {
		t.Errorf("theme lost: %v", cfg)
	}
	skills := toStrings(t, cfg["skills"])
	if len(skills) != 2 {
		t.Errorf("skills = %v", skills)
	}
}

// TestInstallInCustomRepoPath verifies the managed paths are rebased when the
// monorepo is cloned outside ~/git/dev-context.
func TestInstallInCustomRepoPath(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	os.MkdirAll(home, 0o755)
	t.Setenv("HOME", home)

	repo := filepath.Join(tmp, "clones", "dev-context")
	write(t, filepath.Join(repo, "opencode", "skills", "dev-context-manager", "SKILL.md"), "x")
	write(t, filepath.Join(repo, "opencode", "config", "opencode.reference.jsonc"), referenceJSONC)

	p := &Provider{}
	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	skills := toStrings(t, cfg["skills"])
	want := []string{filepath.Join(repo, "kiro", "skills"), filepath.Join(repo, "opencode", "skills")}
	if len(skills) != 2 || skills[0] != want[0] || skills[1] != want[1] {
		t.Errorf("skills = %v, want %v (rebase failed)", skills, want)
	}
	instructions := toStrings(t, cfg["instructions"])
	if len(instructions) != 1 || instructions[0] != filepath.Join(repo, "kiro", "steering", "*.md") {
		t.Errorf("instructions = %v", instructions)
	}
}

// TestInstallPrunesRenamedSkillButKeepsLocalOnes mirrors the kiro orphan model.
func TestInstallPrunesRenamedSkillButKeepsLocalOnes(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	// Pre-existing organization-local skill not managed by dctx.
	localSkill := filepath.Join(home, ".config", "opencode", "skills", "org-local", "SKILL.md")
	write(t, localSkill, "skill local da organização")

	if err := p.Install(repo); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	mustExist(t, filepath.Join(home, ".config", "opencode", "skills", "dev-context-manager", "SKILL.md"))

	// Rename the skill in the repo.
	oldDir := filepath.Join(repo, "opencode", "skills", "dev-context-manager")
	newDir := filepath.Join(repo, "opencode", "skills", "dev-context-oc")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}

	if err := p.Install(repo); err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	mustExist(t, filepath.Join(home, ".config", "opencode", "skills", "dev-context-oc", "SKILL.md"))
	mustNotExist(t, filepath.Join(home, ".config", "opencode", "skills", "dev-context-manager", "SKILL.md"))
	// Empty orphaned directory should be cleaned up.
	mustNotExist(t, filepath.Join(home, ".config", "opencode", "skills", "dev-context-manager"))
	// Organization-local skill must be untouched.
	mustExist(t, localSkill)
}

// TestInstallIsIdempotent verifies running install twice without repo changes
// does not remove anything, does not duplicate entries and keeps the manifest.
func TestInstallIsIdempotent(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	if err := p.Install(repo); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Install(repo); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("config changed on re-install:\nfirst:  %s\nsecond: %s", first, second)
	}
	var cfg map[string]interface{}
	json.Unmarshal(first, &cfg)
	skills := toStrings(t, cfg["skills"])
	if len(skills) != 2 {
		t.Errorf("skills duplicated: %v", skills)
	}
}

// TestStatusReportsInstalledState verifies Status reflects installed artifacts.
func TestStatusReportsInstalledState(t *testing.T) {
	repo := setupEnv(t)
	p := &Provider{}

	statuses, err := p.Status(repo)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("expected at least one artifact in status")
	}
	if len(statuses) != 2 {
		t.Errorf("expected skills + config artifacts, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.Installed {
			t.Errorf("artifact %s should not be installed before install", s.Name)
		}
	}

	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	statuses, err = p.Status(repo)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	for _, s := range statuses {
		if !s.Installed || !s.UpToDate {
			t.Errorf("artifact %s should be installed and up-to-date after install (installed=%v upToDate=%v)",
				s.Name, s.Installed, s.UpToDate)
		}
	}
}

// TestStatusOutdatedWhenEntriesMissing verifies the merge-aware check flags a
// config whose managed entries were partially removed by the user.
func TestStatusOutdatedWhenEntriesMissing(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	dst := filepath.Join(home, ".config", "opencode", "opencode.json")
	write(t, dst, `{"$schema":"x","instructions":[],"mcp":{"servers":{"ai-router":{"type":"local"}}}}`)

	statuses, err := p.Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if strings.HasSuffix(s.Name, ".reference.jsonc") {
			if !s.Installed {
				t.Error("config should be reported as installed")
			}
			if s.UpToDate {
				t.Error("config should be outdated when managed skills/instructions are missing")
			}
		} else {
			if s.Installed {
				t.Error("skill should be reported as not installed")
			}
		}
	}
}

// TestStatusUpToDateWithManualTildeSetup verifies the satisfaction check
// accepts a manual setup written in ~ form (validated live in the spike).
func TestStatusUpToDateWithManualTildeSetup(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	dst := filepath.Join(home, ".config", "opencode", "opencode.json")
	write(t, dst, `{
  "$schema": "https://opencode.ai/config.json",
  "skills": ["~/git/dev-context/kiro/skills", "~/git/dev-context/opencode/skills"],
  "instructions": ["~/git/dev-context/kiro/steering/*.md"],
  "mcp": {"servers": {"ai-router": {"type": "local", "command": ["{env:HOME}/.local/share/kiro-ai-router/app/.venv/bin/python", "-m", "kiro_ai_router.server"]}}}
}`)
	// Copy the skills manually, as the README manual setup does.
	skillsSrc := filepath.Join(repo, "opencode", "skills", "dev-context-manager")
	skillsDst := filepath.Join(home, ".config", "opencode", "skills", "dev-context-manager")
	os.MkdirAll(skillsDst, 0o755)
	data, _ := os.ReadFile(filepath.Join(skillsSrc, "SKILL.md"))
	os.WriteFile(filepath.Join(skillsDst, "SKILL.md"), data, 0o644)

	p := &Provider{}
	statuses, err := p.Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if !s.Installed || !s.UpToDate {
			t.Errorf("%s should be up-to-date with manual ~-form setup (installed=%v upToDate=%v)",
				s.Name, s.Installed, s.UpToDate)
		}
	}
}

// TestStatusUpToDateWithEnvHOMEForm verifies the {env:HOME} path form the
// monorepo sometimes documents is recognized as the same managed path.
func TestStatusUpToDateWithEnvHOMEForm(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	dst := filepath.Join(home, ".config", "opencode", "opencode.json")
	write(t, dst, `{"$schema":"x","skills": ["{env:HOME}/git/dev-context/kiro/skills", "{env:HOME}/git/dev-context/opencode/skills"], "instructions": ["{env:HOME}/git/dev-context/kiro/steering/*.md"], "mcp": {"servers": {"ai-router": {"type":"local"}}}}`)

	p := &Provider{}
	statuses, err := p.Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if strings.HasSuffix(s.Name, ".reference.jsonc") && !s.UpToDate {
			t.Error("config with {env:HOME}-form entries should be up-to-date")
		}
	}
}

// TestStatusOutdatedWhenSchemaMissing verifies that a config without the
// reference's $schema is flagged outdated — dctx would merge it in (the
// manual setup documented in the monorepo README omits it, so 'dctx install'
// canonicalizes those configs).
func TestStatusOutdatedWhenSchemaMissing(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	dst := filepath.Join(home, ".config", "opencode", "opencode.json")
	write(t, dst, `{"skills": ["~/git/dev-context/kiro/skills", "~/git/dev-context/opencode/skills"]}`)

	p := &Provider{}
	statuses, err := p.Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if strings.HasSuffix(s.Name, ".reference.jsonc") && s.UpToDate {
			t.Error("config without $schema should be flagged outdated (dctx merges it)")
		}
	}
}

// TestUninstallRemovesSkillsButKeepsConfig mirrors the kiro policy: merged
// files that may hold user keys are never deleted.
func TestUninstallRemovesSkillsButKeepsConfig(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	cfg := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := p.Uninstall(); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	mustNotExist(t, filepath.Join(home, ".config", "opencode", "skills", "dev-context-manager"))
	mustExist(t, cfg) // merged config kept
	// A second uninstall is a clean no-op.
	if err := p.Uninstall(); err != nil {
		t.Errorf("second uninstall failed: %v", err)
	}
}

// TestStripJSONC verifies comment/trailing-comma tolerance, including the
// "//" inside the $schema URL which must NOT be treated as a comment.
func TestStripJSONC(t *testing.T) {
	out := stripJSONC([]byte(referenceJSONC))
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("stripJSONC output invalid: %v\n%s", err, out)
	}
	if parsed["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("$schema corrupted by comment stripping: %v", parsed["$schema"])
	}
	if _, ok := parsed["mcp"].(map[string]interface{})["servers"].(map[string]interface{})["ai-router"].(map[string]interface{})["env"]; !ok {
		t.Error("ai-router env lost")
	}
}

// TestMergeIntoLeavesDestinationAloneWhenSatisfied verifies the satisfaction
// path: a third-party-only destination gains key-less changes only when needed.
func TestMergeIntoLeavesDestinationAloneWhenSatisfied(t *testing.T) {
	repo := setupEnv(t)

	ref, err := readReference(repo)
	if err != nil {
		t.Fatal(err)
	}
	_ = ref
	// Destination already fully satisfied (canonical, expanded, all entries):
	// merging again must report no change.
	valid, _ := json.Marshal(map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"skills": []string{
			filepath.Join(repo, "kiro", "skills"),
			filepath.Join(repo, "opencode", "skills"),
		},
		"instructions": []string{filepath.Join(repo, "kiro", "steering", "*.md")},
		"mcp":          referenceMCP(t, repo),
	})
	merged, changed, err := mergeInto(valid, repo)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Errorf("already satisfied merge reported a change: %v", merged)
	}
}

func referenceMCP(t *testing.T, repo string) map[string]interface{} {
	t.Helper()
	ref, err := readReference(repo)
	if err != nil {
		t.Fatal(err)
	}
	mcp, _ := ref["mcp"].(map[string]interface{})
	return mcp
}

// toStrings converts a decoded JSON array to []string, failing the test on
// unexpected shapes.
func toStrings(t *testing.T, v interface{}) []string {
	t.Helper()
	arr, ok := v.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T: %v", v, v)
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		s, ok := e.(string)
		if !ok {
			t.Fatalf("expected string entry, got %T: %v", e, e)
		}
		out = append(out, s)
	}
	return out
}
