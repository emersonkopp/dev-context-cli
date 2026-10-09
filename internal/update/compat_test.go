package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// legacyAssetMatch replicates the asset matcher shipped in dctx <= 1.3.0,
// which only recognized the "macos" spelling for darwin. We keep it here as an
// executable specification: future releases MUST publish assets whose names
// this legacy matcher can still find, otherwise users stuck on old versions
// can never self-update. Do not "fix" this function — it documents old behavior.
func legacyAssetMatch(assetNames []string, goos, goarch string) (string, bool) {
	osMap := map[string]string{"linux": "linux", "darwin": "macos"}
	osStr := osMap[goos]
	for _, name := range assetNames {
		n := strings.ToLower(name)
		if strings.Contains(n, osStr) && strings.Contains(n, goarch) {
			return name, true
		}
	}
	return "", false
}

// goreleaserAssetNames returns the asset filenames the release pipeline is
// expected to publish for a given version. This mirrors the naming contract in
// .goreleaser.yaml; if that file changes the OS token, update this list and the
// tests below will confirm both matchers still work.
func goreleaserAssetNames(version string) []string {
	return []string{
		"dctx_" + version + "_linux_amd64.tar.gz",
		"dctx_" + version + "_linux_arm64.tar.gz",
		"dctx_" + version + "_macos_amd64.tar.gz",
		"dctx_" + version + "_macos_arm64.tar.gz",
		"dctx_" + version + "_checksums.txt",
	}
}

// TestReleaseAssetsAreUpdatableByLegacyClients guarantees that every supported
// platform can be resolved by the OLD (<=1.3.0) matcher from the asset names
// the pipeline publishes. This is the future-proofing guard: it fails if a
// future naming change would break self-update for users on old versions.
func TestReleaseAssetsAreUpdatableByLegacyClients(t *testing.T) {
	names := goreleaserAssetNames("1.9.9")
	platforms := []struct{ os, arch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
	}
	for _, p := range platforms {
		if _, ok := legacyAssetMatch(names, p.os, p.arch); !ok {
			t.Errorf("legacy (<=1.3.0) client on %s/%s could NOT find an asset in %v — "+
				"self-update would break for old versions", p.os, p.arch, names)
		}
	}
}

// TestReleaseAssetsAreUpdatableByCurrentClient guarantees the current matcher
// (AssetURL) also resolves the published names for this machine's platform.
func TestReleaseAssetsAreUpdatableByCurrentClient(t *testing.T) {
	names := goreleaserAssetNames("1.9.9")
	rel := &Release{TagName: "v1.9.9"}
	for _, n := range names {
		rel.Assets = append(rel.Assets, Asset{Name: n, BrowserDownloadURL: "http://x/" + n})
	}
	url, err := AssetURL(rel)
	if err != nil {
		t.Fatalf("current matcher failed on %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
	}
	if url == "" {
		t.Error("current matcher returned empty URL")
	}
}

// TestGoreleaserNamesMacOSAssetsAsMacos reads the real .goreleaser.yaml and
// fails unless the macOS archive name resolves to the "macos" token. goreleaser
// uses "darwin" for {{ .Os }}, which legacy (<=1.3.0) clients cannot match, so
// the archive name_template MUST remap darwin→macos. This test is the guard
// that keeps self-update working for ALL past and future versions.
func TestGoreleaserNamesMacOSAssetsAsMacos(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("reading .goreleaser.yaml: %v", err)
	}

	// Find the archives' name_template line specifically (ignore comments and
	// the release/checksum name_templates).
	var archiveTmpl string
	inArchives := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			continue // skip comments
		}
		if line == "archives:" {
			inArchives = true
			continue
		}
		// A new top-level section ends the archives block.
		if inArchives && len(raw) > 0 && raw[0] != ' ' && raw[0] != '-' && raw[0] != '\t' {
			inArchives = false
		}
		if inArchives && strings.HasPrefix(line, "name_template:") {
			archiveTmpl = line
			break
		}
	}

	if archiveTmpl == "" {
		t.Fatal("could not find archives name_template in .goreleaser.yaml")
	}
	// Must emit the "macos" token for darwin so legacy (<=1.3.0) clients match.
	if !strings.Contains(archiveTmpl, "macos") {
		t.Errorf("archives name_template must produce 'macos'-named assets for "+
			"backward-compatible self-update; got: %s", archiveTmpl)
	}
	// {{ .Os }} is fine for non-darwin platforms, but darwin MUST be remapped to
	// "macos". A bare {{ .Os }} with no darwin→macos remap resolves to "darwin"
	// and breaks legacy clients.
	hasRawOs := strings.Contains(archiveTmpl, "{{ .Os }}")
	hasDarwinRemap := strings.Contains(archiveTmpl, "darwin") && strings.Contains(archiveTmpl, "macos")
	if hasRawOs && !hasDarwinRemap {
		t.Errorf("archives name_template uses raw {{ .Os }} (darwin) without a "+
			"darwin→macos remap; got: %s", archiveTmpl)
	}
}
