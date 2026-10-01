package kiro

import (
	"os"
	"path/filepath"
	"testing"
)

// setupEnv creates an isolated HOME and a minimal monorepo, returning the repo path.
// It points os.UserHomeDir() at a temp dir via t.Setenv("HOME", ...).
func setupEnv(t *testing.T) (repoPath string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	repo := filepath.Join(tmp, "repo")
	for _, d := range []string{
		filepath.Join(repo, "kiro", "steering"),
		filepath.Join(repo, "kiro", "skills", "validacao-seguranca"),
		filepath.Join(repo, "kiro", "settings"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(repo, "kiro", "steering", "00-pilares.md"), "regra A")
	write(t, filepath.Join(repo, "kiro", "skills", "validacao-seguranca", "SKILL.md"),
		"---\nname: validacao-seguranca\ndescription: x\n---\ncorpo")
	write(t, filepath.Join(repo, "kiro", "settings", "cli.json"), `{"chat.enableAutoAgentUpgrade": true}`)
	return repo
}

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

// TestInstallInstallsSteeringsAndSkills verifies steerings and skills are copied.
func TestInstallInstallsSteeringsAndSkills(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	mustExist(t, filepath.Join(home, ".kiro", "steering", "00-pilares.md"))
	mustExist(t, filepath.Join(home, ".kiro", "skills", "validacao-seguranca", "SKILL.md"))
	mustExist(t, filepath.Join(home, ".kiro", "settings", "cli.json"))
}

// TestInstallPrunesRenamedSkillButKeepsLocalOnes verifies that a skill renamed
// in the repo is removed from the destination on the next install, while a
// skill that dctx never installed (organization-local) is preserved.
func TestInstallPrunesRenamedSkillButKeepsLocalOnes(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	// Pre-existing organization-local skill not managed by dctx.
	localSkill := filepath.Join(home, ".kiro", "skills", "flex-backend-senior", "SKILL.md")
	write(t, localSkill, "skill local da organizacao")

	// First install seeds the manifest.
	if err := p.Install(repo); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	mustExist(t, filepath.Join(home, ".kiro", "skills", "validacao-seguranca", "SKILL.md"))

	// Rename the skill in the repo.
	oldDir := filepath.Join(repo, "kiro", "skills", "validacao-seguranca")
	newDir := filepath.Join(repo, "kiro", "skills", "validacao-sec")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}

	// Second install should install the new name and prune the old one.
	if err := p.Install(repo); err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	mustExist(t, filepath.Join(home, ".kiro", "skills", "validacao-sec", "SKILL.md"))
	mustNotExist(t, filepath.Join(home, ".kiro", "skills", "validacao-seguranca", "SKILL.md"))
	// Empty orphaned directory should be cleaned up.
	mustNotExist(t, filepath.Join(home, ".kiro", "skills", "validacao-seguranca"))
	// Organization-local skill must be untouched.
	mustExist(t, localSkill)
}

// TestInstallIsIdempotent verifies running install twice without repo changes
// does not remove anything and keeps the manifest consistent.
func TestInstallIsIdempotent(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	if err := p.Install(repo); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	if err := p.Install(repo); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	mustExist(t, filepath.Join(home, ".kiro", "steering", "00-pilares.md"))
	mustExist(t, filepath.Join(home, ".kiro", "skills", "validacao-seguranca", "SKILL.md"))
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
			t.Errorf("artifact %s should be installed and up-to-date after install", s.Name)
		}
	}
}

// TestStatusCliJSONUpToDateWithExtraLocalKeys verifies the merge-aware check:
// a cli.json that already contains all source keys (plus extra local keys and
// different formatting) is reported as up-to-date, not outdated.
func TestStatusCliJSONUpToDateWithExtraLocalKeys(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	// Pre-seed a destination cli.json with the source key already set plus an
	// extra local key, in compact (different) formatting.
	dst := filepath.Join(home, ".kiro", "settings", "cli.json")
	write(t, dst, `{"chat.enableAutoAgentUpgrade":true,"local.only":"keep-me"}`)

	statuses, err := p.Status(repo)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	var found bool
	for _, s := range statuses {
		if filepath.Base(s.DstPath) == "cli.json" {
			found = true
			if !s.Installed || !s.UpToDate {
				t.Errorf("cli.json should be up-to-date when all source keys are present (installed=%v upToDate=%v)",
					s.Installed, s.UpToDate)
			}
		}
	}
	if !found {
		t.Fatal("cli.json artifact not found in status")
	}
}

// TestStatusCliJSONOutdatedWhenKeyMissing verifies the merge-aware check still
// flags a cli.json that is missing a source key (or has a different value).
func TestStatusCliJSONOutdatedWhenKeyMissing(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	// Destination has only an unrelated local key — source key is absent.
	dst := filepath.Join(home, ".kiro", "settings", "cli.json")
	write(t, dst, `{"local.only":"x"}`)

	statuses, err := p.Status(repo)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	for _, s := range statuses {
		if filepath.Base(s.DstPath) == "cli.json" {
			if !s.Installed {
				t.Error("cli.json should be reported as installed")
			}
			if s.UpToDate {
				t.Error("cli.json should be outdated when a source key is missing")
			}
		}
	}
}
