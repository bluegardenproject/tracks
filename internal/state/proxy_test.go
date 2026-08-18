package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProxyRoundtrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.PutProxy(ProxyBinding{PublicPort: 3000, UpstreamTrackID: "trk", UpstreamService: "dev"}); err != nil {
		t.Fatal(err)
	}
	if err := fs.PutProxy(ProxyBinding{PublicPort: 8081, BindAll: true}); err != nil {
		t.Fatal(err)
	}

	fs2, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := fs2.AllProxies()
	if len(got) != 2 {
		t.Fatalf("got %d bindings, want 2", len(got))
	}
	// Sorted by port: 3000 then 8081.
	if got[0].PublicPort != 3000 || got[0].UpstreamService != "dev" {
		t.Errorf("binding[0] = %+v, want :3000 → dev", got[0])
	}
	if got[1].PublicPort != 8081 || !got[1].BindAll {
		t.Errorf("binding[1] = %+v, want :8081 bindAll", got[1])
	}
}

func TestPutProxyRejectsZeroPort(t *testing.T) {
	fs, err := OpenFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.PutProxy(ProxyBinding{PublicPort: 0}); err == nil {
		t.Fatal("PutProxy with port 0: want error, got nil")
	}
}

func TestUpdateAndDeleteProxy(t *testing.T) {
	fs, err := OpenFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.PutProxy(ProxyBinding{PublicPort: 3000}); err != nil {
		t.Fatal(err)
	}
	_, ok, err := fs.UpdateProxy(3000, func(b *ProxyBinding) bool {
		b.UpstreamTrackID = "trk"
		b.UpstreamService = "dev"
		return true
	})
	if err != nil || !ok {
		t.Fatalf("UpdateProxy = ok %v, err %v", ok, err)
	}
	if got := fs.AllProxies(); got[0].UpstreamTrackID != "trk" {
		t.Errorf("upstream not persisted: %+v", got[0])
	}

	// Unknown port: not found, mutate not called.
	if _, ok, _ := fs.UpdateProxy(9999, func(*ProxyBinding) bool { t.Fatal("mutate called for unknown port"); return true }); ok {
		t.Error("UpdateProxy on unknown port reported ok")
	}

	removed, err := fs.DeleteProxy(3000)
	if err != nil || !removed {
		t.Fatalf("DeleteProxy = %v, err %v", removed, err)
	}
	if len(fs.AllProxies()) != 0 {
		t.Error("binding survived DeleteProxy")
	}
}

// A pre-v4 state file has no proxies key; it loads as an empty list, not an
// error, and is rewritten at the current schema version.
func TestLoadV3FileHasNoProxies(t *testing.T) {
	dir := t.TempDir()
	raw := `{"schema_version":3,"tracks":[{"id":"20260101-000000-aaaaaa","branch":"fix/x","status":"running","log_path":"","task_prompt":"","created_at":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatalf("loading a v3 file must not error: %v", err)
	}
	if len(fs.AllProxies()) != 0 {
		t.Errorf("v3 file yielded %d proxies, want 0", len(fs.AllProxies()))
	}
}
