// Package update implements self-update via GitHub Releases.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	osMap := map[string]string{
		"linux":  "linux",
		"darwin": "macos",
	}

	osStr, ok := osMap[goos]
	if !ok {
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}
	archStr, ok := archMap[goarch]
	if !ok {
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	// Expected filename pattern: dctx_<version>_<os>_<arch>
	// e.g. dctx_v1.0.0_linux_amd64 or dctx_v1.0.0_macos_arm64
	for _, asset := range rel.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, osStr) && strings.Contains(name, archStr) {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no asset found for %s/%s in release %s", osStr, archStr, rel.TagName)
}

// Apply downloads the binary at url and replaces the currently running executable.
func Apply(url string) error {
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

	// Write to a temp file first so we can verify before replacing.
	tmp, err := os.CreateTemp("", binaryName+"-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("writing download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Re-open for the updater.
	f, err := os.Open(tmp.Name())
	if err != nil {
		return err
	}
	defer f.Close()

	return update.Apply(f, update.Options{})
}
