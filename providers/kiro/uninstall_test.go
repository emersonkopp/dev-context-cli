package kiro

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUninstallRemovesManagedArtifactsOnly verifies that Uninstall removes the
// files dctx installed (recorded in the manifest) but leaves organization-local
// files it never installed untouched. It also clears the manifest.
func TestUninstallRemovesManagedArtifactsOnly(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	// Organization-local skill dctx never installed.
	localSkill := filepath.Join(home, ".kiro", "skills", "flex-backend-senior", "SKILL.md")
	write(t, localSkill, "skill local")

	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	steering := filepath.Join(home, ".kiro", "steering", "00-pilares.md")
	managedSkill := filepath.Join(home, ".kiro", "skills", "validacao-seguranca", "SKILL.md")
	mustExist(t, steering)
	mustExist(t, managedSkill)

	if err := p.Uninstall(); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	// Managed artifacts removed.
	mustNotExist(t, steering)
	mustNotExist(t, managedSkill)
	// Empty managed skill directory cleaned up.
	mustNotExist(t, filepath.Join(home, ".kiro", "skills", "validacao-seguranca"))
	// Organization-local skill preserved.
	mustExist(t, localSkill)

	// Manifest cleared so a later install starts clean.
	mp, _ := manifestPath()
	mustNotExist(t, mp)
}

// TestReinstallAfterUninstallRecoversState simulates the real "remove then
// reinstall" flow: after a full uninstall (manifest gone), a fresh install must
// reinstall every artifact and rebuild a correct manifest — with no leftover
// orphan entries from the previous life.
func TestReinstallAfterUninstallRecoversState(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	if err := p.Install(repo); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	if err := p.Uninstall(); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	// Manifest is gone — this is the "clean slate" state.
	mp, _ := manifestPath()
	mustNotExist(t, mp)

	// Reinstall (as 'dctx bootstrap'/'dctx install' would do on reinstall).
	if err := p.Install(repo); err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}

	// Everything is back.
	mustExist(t, filepath.Join(home, ".kiro", "steering", "00-pilares.md"))
	mustExist(t, filepath.Join(home, ".kiro", "skills", "validacao-seguranca", "SKILL.md"))
	mustExist(t, filepath.Join(home, ".kiro", "settings", "cli.json"))

	// Manifest rebuilt and consistent with the current install (no orphans):
	// a no-op install right after must not remove anything.
	if err := p.Install(repo); err != nil {
		t.Fatalf("idempotent reinstall failed: %v", err)
	}
	mustExist(t, filepath.Join(home, ".kiro", "steering", "00-pilares.md"))
	mustExist(t, mp)
}

// TestInstallDoesNotDependOnPreexistingState verifies install works with no
// prior ~/.dev-context at all (as right after a manual full removal), creating
// the manifest fresh.
func TestInstallDoesNotDependOnPreexistingState(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	// Ensure no state dir exists yet.
	mustNotExist(t, filepath.Join(home, ".dev-context"))

	if err := p.Install(repo); err != nil {
		t.Fatalf("install from clean slate failed: %v", err)
	}
	mp, _ := manifestPath()
	mustExist(t, mp)
	mustExist(t, filepath.Join(home, ".kiro", "steering", "00-pilares.md"))
}

// TestUninstallWithoutManifestIsNoop verifies Uninstall does not error when
// there is nothing recorded (e.g. dctx was never run or manifest already gone).
func TestUninstallWithoutManifestIsNoop(t *testing.T) {
	setupEnv(t)
	p := &Provider{}
	if err := p.Uninstall(); err != nil {
		t.Fatalf("uninstall with no manifest should be a no-op, got: %v", err)
	}
}

// TestUninstallDoesNotDeleteCliJSON verifies cli.json is NOT deleted on
// uninstall: it is a merged file that may hold the user's own local keys, so
// removing it would be destructive. dctx stops managing it but leaves it.
func TestUninstallDoesNotDeleteCliJSON(t *testing.T) {
	repo := setupEnv(t)
	home, _ := os.UserHomeDir()
	p := &Provider{}

	if err := p.Install(repo); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	cli := filepath.Join(home, ".kiro", "settings", "cli.json")
	mustExist(t, cli)

	if err := p.Uninstall(); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	// cli.json must survive — it may contain the user's local keys.
	mustExist(t, cli)
}
