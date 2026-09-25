package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	edited := Default().With(BgBase, Value{Dark: "#123456", Light: "#abcdef"})
	if Default().Value(BgBase) == edited.Value(BgBase) {
		t.Fatal("With changed the original theme's value, or didn't change the copy")
	}
	path := filepath.Join(t.TempDir(), "theme.yaml")
	if err := edited.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range All {
		if loaded.Value(token) != edited.Value(token) {
			t.Errorf("%s = %v after loading, want %v", token, loaded.Value(token), edited.Value(token))
		}
	}
}

func TestLoadWithoutFileIsDefault(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil || got.Value(Accent) != Default().Value(Accent) {
		t.Errorf("Load of a missing file = %v, %v; want the built-in theme", got.Value(Accent), err)
	}
}

func TestLoadOfBrokenFileIsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.yaml")
	if err := os.WriteFile(path, []byte("name: broken\ntokens:\n  accent: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err == nil || got.Value(Accent) != Default().Value(Accent) {
		t.Errorf("Load of a broken file = %v, %v; want the built-in theme and an error", got.Value(Accent), err)
	}
}

func TestLoadFillsNewTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.yaml")
	old := "name: mine\ntokens:\n  accent: { dark: \"#123456\", light: \"#abcdef\" }\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value(Accent) != (Value{Dark: "#123456", Light: "#abcdef"}) || got.Value(TableBgSelected) != Default().Value(TableBgSelected) {
		t.Errorf("Load of a theme without newer tokens: accent %v, table.bg.selected %v; want its accent and the built-in rest",
			got.Value(Accent), got.Value(TableBgSelected))
	}
}
