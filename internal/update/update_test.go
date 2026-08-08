package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.5.0", "0.5.1", true},
		{"0.5.0", "0.6.0", true},
		{"0.5.0", "1.0.0", true},
		{"0.5.0", "0.5.0", false},
		{"0.5.1", "0.5.0", false},
		{"1.0.0", "0.9.9", false},
		{"v0.5.0", "v0.5.1", true},
		// `make build` stamps `git describe`: the commits-since-tag
		// suffix doesn't make it newer than the tag it describes.
		{"v0.5.0-3-gabc1234-dirty", "v0.5.0", false},
		{"v0.5.0-3-gabc1234", "v0.5.1", true},
		// An unidentifiable local build is still offered the release.
		{"dev", "0.5.0", true},
		// An unparseable release is never offered.
		{"0.5.0", "nightly", false},
		{"0.5.0", "", false},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestLatestFromPicksPlatformAsset(t *testing.T) {
	body := `{
	  "tag_name": "v1.2.3",
	  "html_url": "https://example.test/releases/v1.2.3",
	  "assets": [
	    {"name": "tracks-other-arch", "browser_download_url": "https://example.test/other"},
	    {"name": "` + AssetName() + `", "browser_download_url": "https://example.test/mine"}
	  ]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	rel, err := latestFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("latestFrom: %v", err)
	}
	if rel.Tag != "v1.2.3" || rel.Version != "1.2.3" {
		t.Errorf("got tag %q version %q", rel.Tag, rel.Version)
	}
	if rel.AssetURL != "https://example.test/mine" {
		t.Errorf("asset URL = %q, want the one matching %s", rel.AssetURL, AssetName())
	}
}

func TestLatestFromNoAssetForPlatform(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"tracks-plan9-mips","browser_download_url":"https://example.test/x"}]}`))
	}))
	defer srv.Close()

	rel, err := latestFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("latestFrom: %v", err)
	}
	if rel.AssetURL != "" {
		t.Errorf("AssetURL = %q, want empty", rel.AssetURL)
	}
}

func TestLatestFromErrors(t *testing.T) {
	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer notFound.Close()
	if _, err := latestFrom(context.Background(), notFound.URL); err == nil {
		t.Error("want an error for a 404 release endpoint")
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer empty.Close()
	if _, err := latestFrom(context.Background(), empty.URL); err == nil {
		t.Error("want an error when the payload has no tag")
	}
}

func TestApplyWithoutAsset(t *testing.T) {
	if _, err := Apply(context.Background(), Release{Tag: "v1.0.0"}); err == nil {
		t.Error("want an error when the release has no asset for this platform")
	}
}

// fakeInstall lays down a stand-in for the running binary and points
// selfPath at it, so Apply can be exercised end to end without touching
// the test process's own executable.
func fakeInstall(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "tracks")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho old\n"), 0o700); err != nil {
		t.Fatalf("writing fake install: %v", err)
	}
	orig := selfPath
	selfPath = func() (string, error) { return target, nil }
	t.Cleanup(func() { selfPath = orig })
	return target
}

// assetServer serves body as the release asset.
func assetServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestApplyReplacesBinary(t *testing.T) {
	target := fakeInstall(t)
	srv := assetServer(t, "#!/bin/sh\necho new\n")

	got, err := Apply(context.Background(), Release{Tag: "v1.2.3", AssetURL: srv.URL})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != target {
		t.Errorf("replaced %q, want %q", got, target)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading target: %v", err)
	}
	if !strings.Contains(string(content), "echo new") {
		t.Errorf("target still holds the old binary: %q", content)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("mode = %v, want the replaced binary's 0700", fi.Mode().Perm())
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), tmpPrefix+"*"))
	if len(leftovers) != 0 {
		t.Errorf("left temp files behind: %v", leftovers)
	}
}

func TestApplyKeepsTargetWhenDownloadDoesNotRun(t *testing.T) {
	target := fakeInstall(t)
	// A binary for another platform (or a truncated download) fails to
	// execute — the working install must survive.
	srv := assetServer(t, "not an executable")

	if _, err := Apply(context.Background(), Release{Tag: "v1.2.3", AssetURL: srv.URL}); err == nil {
		t.Fatal("want an error when the downloaded binary does not run")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading target: %v", err)
	}
	if !strings.Contains(string(content), "echo old") {
		t.Errorf("target was replaced by a broken download: %q", content)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), tmpPrefix+"*"))
	if len(leftovers) != 0 {
		t.Errorf("left temp files behind: %v", leftovers)
	}
}

func TestApplySweepsStaleTempFiles(t *testing.T) {
	target := fakeInstall(t)
	stale := filepath.Join(filepath.Dir(target), tmpPrefix+"killed")
	if err := os.WriteFile(stale, []byte("half a download"), 0o600); err != nil {
		t.Fatalf("writing stale temp file: %v", err)
	}
	srv := assetServer(t, "#!/bin/sh\nexit 0\n")

	if _, err := Apply(context.Background(), Release{Tag: "v1.2.3", AssetURL: srv.URL}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale temp file survived: %v", err)
	}
}
