package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// verifyChecksum returns nil when the SHA256 of data equals wantHex (compared
// case-insensitively, ignoring surrounding whitespace). It is the core guard
// that prevents a corrupted or truncated download from being applied over the
// running binary.
func verifyChecksum(data []byte, wantHex string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	want := strings.ToLower(strings.TrimSpace(wantHex))
	if got != want {
		return fmt.Errorf("checksum mismatch: downloaded archive sha256=%s, expected %s "+
			"(download may be corrupted — not applying update)", got, want)
	}
	return nil
}

// checksumForAsset parses a goreleaser-style checksums.txt and returns the hex
// digest listed for assetName. Each line is "<hexsum>  <filename>".
func checksumForAsset(contents, assetName string) (string, error) {
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Last field is the filename; first is the digest. Match on base name
		// so any path prefix in the checksum file is tolerated.
		name := path.Base(fields[len(fields)-1])
		if name == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %q in checksums file", assetName)
}

// checksumURLFromAsset derives the checksums.txt download URL from an asset URL
// by replacing the asset filename with the goreleaser checksums filename
// ("dctx_<version>_checksums.txt"). version may be "v1.4.0" or "1.4.0".
func checksumURLFromAsset(assetURL, version string) (string, error) {
	u, err := url.Parse(assetURL)
	if err != nil {
		return "", fmt.Errorf("parsing asset URL: %w", err)
	}
	bare := strings.TrimPrefix(strings.TrimSpace(version), "v")
	checksumFile := fmt.Sprintf("dctx_%s_checksums.txt", bare)
	dir := path.Dir(u.Path)
	u.Path = path.Join(dir, checksumFile)
	return u.String(), nil
}

// fetchChecksums downloads the checksums.txt file at url and returns its text.
func fetchChecksums(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating checksums request: %w", err)
	}
	req.Header.Set("User-Agent", "dctx")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums download returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading checksums: %w", err)
	}
	return string(data), nil
}
