package update

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestApplyRejectsCorruptedDownload is the end-to-end regression test for the
// reported bug: a corrupted download must NOT be applied. We serve an archive
// whose bytes do not match the published checksum and assert Apply fails with a
// checksum error before touching any binary.
func TestApplyRejectsCorruptedDownload(t *testing.T) {
	// The "real" archive the checksum is computed over.
	realArchive := makeTarGz(t, map[string][]byte{"dctx": []byte("\x7fELF-good")})
	// What the server actually returns — truncated/corrupted.
	corrupted := realArchive[:len(realArchive)-5]

	mux := http.NewServeMux()
	var srvURL string
	mux.HandleFunc("/releases/download/v9.9.9/dctx_9.9.9_test_test.tar.gz",
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(corrupted)
		})
	mux.HandleFunc("/releases/download/v9.9.9/dctx_9.9.9_checksums.txt",
		func(w http.ResponseWriter, r *http.Request) {
			// Checksum of the REAL archive, not the corrupted bytes served.
			fmt.Fprintf(w, "%s  dctx_9.9.9_test_test.tar.gz\n", sha256HexBytes(realArchive))
		})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	assetURL := srvURL + "/releases/download/v9.9.9/dctx_9.9.9_test_test.tar.gz"
	err := Apply(assetURL, "v9.9.9")
	if err == nil {
		t.Fatal("expected Apply to reject a corrupted download, but it succeeded")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Errorf("expected a checksum error, got %q", err.Error())
	}
}
