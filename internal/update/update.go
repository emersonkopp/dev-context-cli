// Package update implements self-update via GitHub Releases.
package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/inconshreveable/go-update"
)

const (
	githubAPI  = "https://api.github.com/repos/emersonkopp/dev-context-cli/releases/latest"
	binaryName = "dctx"
)

// Release holds the fields we care about from the GitHub Releases API.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is a single downloadable file in a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// LatestRelease fetches the latest release info from GitHub.
func LatestRelease() (*Release, error) {
	return fetchReleaseFrom(githubAPI)
}

// fetchReleaseFrom fetches release info from the given URL. Extracted from
// LatestRelease so it can be exercised against a test server.
func fetchReleaseFrom(url string) (*Release, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// The GitHub API requires a User-Agent header; requests without it are
	// rejected with 403.
	req.Header.Set("User-Agent", "dctx")
	// Authenticate when a token is available to raise the rate limit from 60
	// to 5000 requests/hour. Falls back to anonymous when no token is set.
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, newAPIError(resp)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parsing release info: %w", err)
	}
	return &rel, nil
}

// githubToken returns a GitHub token from the environment, if present.
func githubToken() string {
	for _, env := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	return ""
}

// newAPIError builds a helpful error for a non-200 GitHub API response,
// detecting the common rate-limit case to guide the user.
func newAPIError(resp *http.Response) error {
	if resp.StatusCode == http.StatusForbidden &&
		resp.Header.Get("X-RateLimit-Remaining") == "0" {
		msg := "GitHub API rate limit exceeded"
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
				msg += fmt.Sprintf(" (resets at %s)", time.Unix(ts, 0).Format(time.Kitchen))
			}
		}
		return fmt.Errorf("%s. Set GITHUB_TOKEN or GH_TOKEN to raise the limit "+
			"(60→5000/h), or retry later", msg)
	}
	return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
}

// NeedsUpdate returns true when latestTag is newer than currentVersion.
// Both are expected in "vX.Y.Z" format.
func NeedsUpdate(currentVersion, latestTag string) bool {
	return normalise(latestTag) != normalise(currentVersion)
}

func normalise(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// AssetURL returns the download URL for the asset matching the current OS/arch.
func AssetURL(rel *Release) (string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go arch names to the names used in goreleaser asset filenames.
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
	}
	// Accept multiple OS spellings per platform. goreleaser's default
	// name_template uses "darwin" for macOS, but older releases (and some
	// configs) used "macos"; match either so updates never break on naming.
	osAliases := map[string][]string{
		"linux":  {"linux"},
		"darwin": {"darwin", "macos"},
	}

	osNames, ok := osAliases[goos]
	if !ok {
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}
	archStr, ok := archMap[goarch]
	if !ok {
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	// Expected filename pattern: dctx_<version>_<os>_<arch>
	// e.g. dctx_1.0.0_linux_amd64 or dctx_1.3.0_darwin_arm64
	for _, asset := range rel.Assets {
		name := strings.ToLower(asset.Name)
		if !strings.Contains(name, archStr) {
			continue
		}
		for _, osStr := range osNames {
			if strings.Contains(name, osStr) {
				return asset.BrowserDownloadURL, nil
			}
		}
	}
	return "", fmt.Errorf("no asset found for %s/%s in release %s", goos, archStr, rel.TagName)
}

// Apply downloads the binary at url and replaces the currently running executable.
// version is the release tag (e.g. "v1.4.0") used to locate the checksums file;
// when non-empty, the downloaded archive is verified against the published
// SHA256 checksum before anything is applied, so a corrupted or truncated
// download can never overwrite the running binary.
func Apply(url, version string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating download request: %w", err)
	}
	req.Header.Set("User-Agent", "dctx")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	// Read the whole archive into memory so we can verify its integrity before
	// touching the running binary. Release archives are a few MB — safe to buffer.
	archiveBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading download: %w", err)
	}

	// Verify the archive against the published SHA256 checksum. A mismatch means
	// the download is corrupted/truncated; we abort before replacing anything.
	if version != "" {
		if err := verifyDownload(archiveBytes, url, version); err != nil {
			return err
		}
	}

	// Extract the real binary from the verified .tar.gz release archive.
	// (Applying the raw tarball would corrupt the binary — "Exec format error".)
	binaryBytes, err := extractBinary(bytes.NewReader(archiveBytes))
	if err != nil {
		return fmt.Errorf("extracting binary from archive: %w", err)
	}

	// go-update writes to a temp file in the target directory and renames it
	// over the current binary atomically, so an interrupted apply never leaves
	// a half-written (corrupted) executable in place.
	return update.Apply(bytes.NewReader(binaryBytes), update.Options{})
}

// verifyDownload fetches the release checksums file and verifies that
// archiveBytes matches the published SHA256 for the asset named in assetURL.
func verifyDownload(archiveBytes []byte, assetURL, version string) error {
	checksumURL, err := checksumURLFromAsset(assetURL, version)
	if err != nil {
		return fmt.Errorf("locating checksums file: %w", err)
	}
	contents, err := fetchChecksums(checksumURL)
	if err != nil {
		return fmt.Errorf("verifying download: %w", err)
	}
	assetName := path.Base(assetURL)
	wantHex, err := checksumForAsset(contents, assetName)
	if err != nil {
		return fmt.Errorf("verifying download: %w", err)
	}
	return verifyChecksum(archiveBytes, wantHex)
}

// extractBinary reads a gzip-compressed tar stream (a goreleaser release
// archive) and returns the bytes of the "dctx" executable inside it.
func extractBinary(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		// Match the binary by base name so a leading path does not matter.
		if filepath.Base(hdr.Name) == binaryName {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("reading %s from archive: %w", binaryName, err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("binary %q not found in release archive", binaryName)
}
