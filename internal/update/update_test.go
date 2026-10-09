package update

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// TestLatestReleaseSendsUserAgent verifies the request includes a User-Agent,
// which the GitHub API requires (missing it yields 403).
func TestLatestReleaseSendsUserAgent(t *testing.T) {
	var gotUA, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","assets":[]}`))
	}))
	defer srv.Close()

	rel, err := fetchReleaseFrom(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUA == "" {
		t.Error("expected a User-Agent header to be sent")
	}
	if rel.TagName != "v1.2.0" {
		t.Errorf("expected tag v1.2.0, got %q", rel.TagName)
	}
	// No token set in this test environment → no Authorization header.
	if gotAuth != "" {
		t.Errorf("did not expect Authorization header without a token, got %q", gotAuth)
	}
}

// TestLatestReleaseUsesToken verifies a token from the env is sent as Bearer.
func TestLatestReleaseUsesToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","assets":[]}`))
	}))
	defer srv.Close()

	if _, err := fetchReleaseFrom(srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("expected Bearer token header, got %q", gotAuth)
	}
}

// TestRateLimitErrorMessage verifies a 403 with remaining=0 produces a clear,
// actionable message instead of a bare "403".
func TestRateLimitErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1790882184")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := fetchReleaseFrom(srv.URL)
	if err == nil {
		t.Fatal("expected an error on 403 rate limit")
	}
	msg := err.Error()
	if !strings.Contains(msg, "rate limit") {
		t.Errorf("expected a rate-limit message, got %q", msg)
	}
	if !strings.Contains(msg, "GITHUB_TOKEN") {
		t.Errorf("expected guidance to set a token, got %q", msg)
	}
}

// TestForbiddenNonRateLimit verifies a 403 without rate-limit headers reports a
// generic API error (not the rate-limit path).
func TestForbiddenNonRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := fetchReleaseFrom(srv.URL)
	if err == nil {
		t.Fatal("expected an error on 403")
	}
	if strings.Contains(err.Error(), "rate limit") {
		t.Errorf("did not expect rate-limit message without rate-limit headers, got %q", err.Error())
	}
}

// TestAssetURLMatchesDarwinNaming verifies the asset matcher accepts the
// goreleaser default naming (which uses "darwin", not "macos") on macOS.
// Regression test for: "no asset found for macos/arm64".
func TestAssetURLMatchesDarwinNaming(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-specific asset naming test")
	}
	rel := &Release{
		TagName: "v1.3.0",
		Assets: []Asset{
			{Name: "dctx_1.3.0_linux_amd64.tar.gz", BrowserDownloadURL: "http://x/linux-amd64"},
			{Name: "dctx_1.3.0_darwin_" + runtime.GOARCH + ".tar.gz", BrowserDownloadURL: "http://x/darwin"},
		},
	}
	url, err := AssetURL(rel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://x/darwin" {
		t.Errorf("expected the darwin asset, got %q", url)
	}
}

// TestAssetURLMatchesMacosNaming verifies the legacy "macos" spelling is still
// accepted (backward compatibility with older releases).
func TestAssetURLMatchesMacosNaming(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-specific asset naming test")
	}
	rel := &Release{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: "dctx_1.0.0_macos_" + runtime.GOARCH + ".tar.gz", BrowserDownloadURL: "http://x/macos"},
		},
	}
	url, err := AssetURL(rel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://x/macos" {
		t.Errorf("expected the macos asset, got %q", url)
	}
}
