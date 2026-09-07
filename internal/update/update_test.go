package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	    {"name": "` + AssetName() + `", "browser_download_url": "https://example.test/mine"},
	    {"name": "` + ChecksumsName + `", "browser_download_url": "https://example.test/sums"}
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
	if rel.ChecksumsURL != "https://example.test/sums" {
		t.Errorf("checksums URL = %q, want the one matching %s", rel.ChecksumsURL, ChecksumsName)
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

// fakeRelease serves body as this platform's release asset alongside a
// checksums file that vouches for it, and returns the Release naming
// both — the happy path Apply expects.
func fakeRelease(t *testing.T, body string) Release {
	t.Helper()
	return fakeReleaseWithSums(t, body, sha256Hex(body)+"  "+AssetName()+"\n")
}

// fakeReleaseWithSums spells the checksums file out, for the cases where
// it disagrees with the asset it is supposed to describe.
func fakeReleaseWithSums(t *testing.T, body, sums string) Release {
	t.Helper()
	allowLocalAssets(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/"+ChecksumsName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return Release{
		Tag:          "v1.2.3",
		AssetURL:     srv.URL + "/asset",
		ChecksumsURL: srv.URL + "/" + ChecksumsName,
	}
}

// allowLocalAssets relaxes the GitHub host allowlist for one test, since
// httptest serves plain HTTP on 127.0.0.1.
func allowLocalAssets(t *testing.T) {
	t.Helper()
	orig := verifyAssetURL
	verifyAssetURL = func(string) error { return nil }
	t.Cleanup(func() { verifyAssetURL = orig })
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestApplyReplacesBinary(t *testing.T) {
	target := fakeInstall(t)
	rel := fakeRelease(t, "#!/bin/sh\necho new\n")

	got, err := Apply(context.Background(), rel)
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
	// A binary for another platform fails to execute even though it is
	// the file the release vouches for — the working install must
	// survive.
	rel := fakeRelease(t, "not an executable")

	if _, err := Apply(context.Background(), rel); err == nil {
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
	rel := fakeRelease(t, "#!/bin/sh\nexit 0\n")

	if _, err := Apply(context.Background(), rel); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale temp file survived: %v", err)
	}
}

// TestApplyRejectsChecksumMismatch is the core of the integrity check: a
// download that isn't the file the release vouches for must never be
// executed or installed.
func TestApplyRejectsChecksumMismatch(t *testing.T) {
	target := fakeInstall(t)
	// The asset would leave a mark if it ever ran, which is what pins the
	// ordering: move the chmod+exec pair ahead of the digest compare and
	// this sentinel appears.
	sentinel := filepath.Join(t.TempDir(), "it-ran")
	sums := strings.Repeat("a", 64) + "  " + AssetName() + "\n"
	rel := fakeReleaseWithSums(t, "#!/bin/sh\ntouch "+sentinel+"\n", sums)

	_, err := Apply(context.Background(), rel)
	if err == nil {
		t.Fatal("want an error when the download does not match the published checksum")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error should name the checksum, got: %v", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("reading target: %v", readErr)
	}
	if !strings.Contains(string(content), "echo old") {
		t.Errorf("target was replaced by an unverified download: %q", content)
	}
	if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
		t.Errorf("the unverified download was executed: %v", statErr)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), tmpPrefix+"*"))
	if len(leftovers) != 0 {
		t.Errorf("left temp files behind: %v", leftovers)
	}
}

func TestApplyRefusesReleaseWithoutChecksums(t *testing.T) {
	fakeInstall(t)
	rel := Release{
		Tag:      "v1.2.3",
		AssetURL: "https://github.com/o/r/releases/download/v1.2.3/" + AssetName(),
	}
	_, err := Apply(context.Background(), rel)
	if err == nil {
		t.Fatal("want an error when the release publishes no checksums")
	}
	if !strings.Contains(err.Error(), ChecksumsName) {
		t.Errorf("error should name %s, got: %v", ChecksumsName, err)
	}
}

// A checksums file that simply omits our asset must not read as approval.
func TestApplyRejectsChecksumsMissingOurAsset(t *testing.T) {
	fakeInstall(t)
	sums := strings.Repeat("b", 64) + "  tracks-plan9-mips\n"
	rel := fakeReleaseWithSums(t, "#!/bin/sh\nexit 0\n", sums)

	if _, err := Apply(context.Background(), rel); err == nil {
		t.Fatalf("want an error when %s does not list %s", ChecksumsName, AssetName())
	}
}

func TestVerifyAssetURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool // true = accepted
	}{
		{"https://github.com/o/r/releases/download/v1/" + AssetName(), true},
		{"https://api.github.com/repos/o/r/releases/assets/1", true},
		{"http://github.com/o/r/releases/download/v1/x", false},
		{"https://github.com.evil.test/o/r/x", false},
		{"https://evil.test/" + AssetName(), false},
		{"https://githubXcom/o/r/x", false},
		{"not a url at all", false},
	}
	for _, c := range cases {
		err := verifyAssetURL(c.url)
		if (err == nil) != c.want {
			t.Errorf("verifyAssetURL(%q) error = %v, want accepted=%v", c.url, err, c.want)
		}
	}
}
