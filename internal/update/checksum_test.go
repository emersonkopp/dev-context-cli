package update

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// sha256Hex is a tiny helper mirroring what the real checksum file stores.
func sha256HexBytes(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// TestChecksumForAsset extracts the expected hash for a given asset name from
// the contents of a goreleaser-style checksums.txt file.
func TestChecksumForAsset(t *testing.T) {
	archive := []byte("fake-archive-bytes")
	other := []byte("another-archive")
	contents := strings.Join([]string{
		sha256HexBytes(other) + "  dctx_1.4.0_linux_amd64.tar.gz",
		sha256HexBytes(archive) + "  dctx_1.4.0_darwin_arm64.tar.gz",
		"", // trailing newline edge case
	}, "\n")

	got, err := checksumForAsset(contents, "dctx_1.4.0_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sha256HexBytes(archive) {
		t.Errorf("got %q, want %q", got, sha256HexBytes(archive))
	}
}

// TestChecksumForAssetMissing errors clearly when the asset is absent.
func TestChecksumForAssetMissing(t *testing.T) {
	contents := sha256HexBytes([]byte("x")) + "  dctx_1.4.0_linux_amd64.tar.gz\n"
	if _, err := checksumForAsset(contents, "dctx_1.4.0_darwin_arm64.tar.gz"); err == nil {
		t.Error("expected an error when the asset is not listed in checksums.txt")
	}
}

// TestVerifyChecksumPass accepts bytes whose hash matches the expected hex.
func TestVerifyChecksumPass(t *testing.T) {
	data := []byte("the real archive")
	if err := verifyChecksum(data, sha256HexBytes(data)); err != nil {
		t.Errorf("expected matching checksum to pass, got %v", err)
	}
}

// TestVerifyChecksumFail rejects bytes whose hash does not match. This is the
// core guard against a corrupted/truncated download being applied.
func TestVerifyChecksumFail(t *testing.T) {
	data := []byte("the real archive")
	corrupted := []byte("the real archiv")
	err := verifyChecksum(corrupted, sha256HexBytes(data))
	if err == nil {
		t.Fatal("expected a checksum mismatch error for corrupted data")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Errorf("expected a checksum-related error, got %q", err.Error())
	}
}

// TestVerifyChecksumCaseInsensitive accepts upper/lower-case hex and surrounding
// whitespace, since checksum files vary in formatting.
func TestVerifyChecksumCaseInsensitive(t *testing.T) {
	data := []byte("payload")
	want := strings.ToUpper(sha256HexBytes(data))
	if err := verifyChecksum(data, "  "+want+"  "); err != nil {
		t.Errorf("expected case-insensitive match to pass, got %v", err)
	}
}

// TestChecksumURLFromAsset derives the checksums.txt URL from an asset URL by
// swapping the filename, so verification works without an extra release lookup.
func TestChecksumURLFromAsset(t *testing.T) {
	assetURL := "https://github.com/emersonkopp/dev-context-cli/releases/download/v1.4.0/dctx_1.4.0_darwin_arm64.tar.gz"
	want := "https://github.com/emersonkopp/dev-context-cli/releases/download/v1.4.0/dctx_1.4.0_checksums.txt"
	got, err := checksumURLFromAsset(assetURL, "v1.4.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
