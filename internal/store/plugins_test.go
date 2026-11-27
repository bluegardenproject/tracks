package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestPluginsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	p := Plugin{Name: "acme", Source: "https://example.com/acme.git", Ref: "v1", Pinned: "abc", Dir: "/p/acme/abc",
		Enabled: true, Approved: `{"components":[]}`}
	added, err := s.AddPlugin(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if added.ID == 0 || added.InstalledAt.IsZero() || added.Pinned != "abc" || !added.Enabled || added.Local {
		t.Fatalf("added %+v", added)
	}
	if _, err := s.AddPlugin(ctx, p); !errors.Is(err, ErrNameTaken) {
		t.Errorf("same name: %v", err)
	}
	added.Pinned, added.Dir, added.Approved = "def", "/p/acme/def", `{"components":[{}]}`
	updated, err := s.UpdatePlugin(ctx, added)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Pinned != "def" || updated.Dir != "/p/acme/def" || updated.Approved != added.Approved || updated.Ref != "v1" {
		t.Errorf("updated %+v", updated)
	}
	if err := s.SetPluginEnabled(ctx, "acme", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddPlugin(ctx, Plugin{Name: "local", Source: "/src/x", Local: true, Dir: "/p/local/local", Approved: "{}"}); err != nil {
		t.Fatal(err)
	}
	all, err := s.Plugins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Name != "acme" || all[0].Enabled || !all[1].Local {
		t.Errorf("plugins %+v", all)
	}
	if err := s.DeletePlugin(ctx, "acme"); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{
		s.DeletePlugin(ctx, "acme"),
		s.SetPluginEnabled(ctx, "acme", true),
		func() error { _, err := s.Plugin(ctx, "acme"); return err }(),
		func() error { _, err := s.UpdatePlugin(ctx, added); return err }(),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("gone: %v", err)
		}
	}
}
