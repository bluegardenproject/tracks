package update

import (
	"context"
	"net/http"
	"net/http/httptest"
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
