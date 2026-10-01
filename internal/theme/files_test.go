package theme

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCreateSaveLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	edited := Default().With(BgBase, "#123456")
	if Default().Value(BgBase) == edited.Value(BgBase) {
		t.Fatal("With changed the original theme's value, or didn't change the copy")
	}
	if err := edited.Save(dir); err == nil {
		t.Error("saving a built-in worked; want Save as new")
	}
	mine, err := Create(dir, " My Theme ", edited)
	if err != nil {
		t.Fatal(err)
	}
	if mine.ID != "my_theme" || mine.DisplayName != "My Theme" || mine.BuiltIn {
		t.Fatalf("created %s %q built-in %v; want my_theme, My Theme, a file", mine.ID, mine.DisplayName, mine.BuiltIn)
	}
	if _, err := Create(dir, "my theme", edited); !errors.Is(err, ErrExists) {
		t.Errorf("creating my theme twice: %v; want ErrExists", err)
	}
	if _, err := Create(dir, "Default Light", edited); !errors.Is(err, ErrExists) {
		t.Errorf("creating a theme with a built-in's id: %v; want ErrExists", err)
	}

	if err := mine.With(TextAccent, "#abcdef").Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, "my_theme")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DisplayName != "My Theme" || loaded.Value(TextAccent) != "#abcdef" || loaded.Value(BgBase) != "#123456" {
		t.Errorf("loaded %q accent %s bg %s; want the saved values", loaded.DisplayName, loaded.Value(TextAccent), loaded.Value(BgBase))
	}
}

func TestLoadFallsBack(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken.yaml", "display_name: broken\ntokens:\n  text.accent: nope\n")
	for _, id := range []string{"missing", "broken", "../etc/passwd"} {
		if got, err := Load(dir, id); err == nil || got.ID != DefaultID {
			t.Errorf("Load(%s) = %s, %v; want Default and an error", id, got.ID, err)
		}
	}
	if got, err := Load(dir, ""); err != nil || got.ID != DefaultID {
		t.Errorf("Load of no choice = %s, %v; want Default", got.ID, err)
	}
	if got, err := Load(dir, "default_light"); err != nil || got.DisplayName != "Default Light" {
		t.Errorf("Load(default_light) = %q, %v", got.DisplayName, err)
	}
}

func TestLoadFillsNewTokens(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "old.yaml", "display_name: Old\ntokens:\n  text.accent: \"#123456\"\n")
	got, err := Load(dir, "old")
	if err != nil {
		t.Fatal(err)
	}
	if got.Value(TextAccent) != "#123456" || got.Value(TableBgSelected) != Default().Value(TableBgSelected) {
		t.Errorf("accent %s, table.bg.selected %s; want its accent and Default for the rest",
			got.Value(TextAccent), got.Value(TableBgSelected))
	}
}

func TestListShowsBuiltInsThenFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "zeta.yaml", "display_name: Alpha\ntokens: {}\n")
	write(t, dir, "beta.yaml", "display_name: Beta\ntokens:\n  text.accent: red\n")
	write(t, dir, "default.yaml", "display_name: Mine\ntokens: {}\n")
	write(t, dir, "notes.txt", "not a theme")
	write(t, dir, ".hidden.yaml", "display_name: Hidden\ntokens: {}\n")

	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range list {
		got = append(got, e.ID)
	}
	want := []string{"default", "default_light", "beta", "default", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("listed %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("listed %v; want %v (built-ins, then files by file name)", got, want)
		}
	}
	if list[4].Err != nil || list[4].DisplayName != "Alpha" || list[4].Path == "" {
		t.Errorf("zeta.yaml: %+v; want the valid theme Alpha", list[4].Err)
	}
	if list[2].Err == nil || list[3].Err == nil {
		t.Error("a bad value and a built-in's id should make files invalid")
	}

	if list, err := List(filepath.Join(dir, "missing")); err != nil || len(list) != 2 {
		t.Errorf("List of a missing folder = %d themes, %v; want the built-ins", len(list), err)
	}
}

func TestIDFrom(t *testing.T) {
	for name, want := range map[string]string{
		"My Theme": "my_theme", "  Solarized -- Dark!": "solarized_dark", "Über": "über", "!!!": "",
	} {
		if got := IDFrom(name); got != want {
			t.Errorf("IDFrom(%q) = %q, want %q", name, got, want)
		}
	}
}
