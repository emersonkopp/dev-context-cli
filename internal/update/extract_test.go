package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

// makeTarGz builds an in-memory .tar.gz containing the given files.
func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestExtractBinaryPullsDctxFromTarGz verifies the real binary is extracted
// from the release archive (not the compressed tarball itself). Regression for
// 'Exec format error' after self-update.
func TestExtractBinaryPullsDctxFromTarGz(t *testing.T) {
	want := []byte("\x7fELF-fake-binary-bytes")
	archive := makeTarGz(t, map[string][]byte{
		"README.md": []byte("# docs"),
		"LICENSE":   []byte("MIT"),
		"dctx":      want,
	})

	got, err := extractBinary(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("extractBinary failed: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}
}

// TestExtractBinaryErrorsWhenMissing verifies a clear error when the archive
// has no dctx binary (so we never overwrite the running binary with garbage).
func TestExtractBinaryErrorsWhenMissing(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{"README.md": []byte("x")})
	if _, err := extractBinary(bytes.NewReader(archive)); err == nil {
		t.Error("expected error when dctx binary is absent from the archive")
	}
}
