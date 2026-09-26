package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingFileIsDefaults(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "settings.yaml"))
	if err != nil || s != (Settings{}) {
		t.Errorf("Load of a missing file = %+v, %v; want the defaults", s, err)
	}
}

func TestSaveKeepsOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "settings.yaml")
	if err := Save(path, Settings{Theme: "default_light"}); err != nil {
		t.Fatal(err)
	}
	if s, err := Load(path); err != nil || s.Theme != "default_light" {
		t.Fatalf("after Save: %+v, %v", s, err)
	}

	newer := "# mine\ntheme: default_light\nnotifications: true # from a newer build\n"
	if err := os.WriteFile(path, []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Settings{Theme: "my_theme"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{"# mine", "theme: my_theme", "notifications: true # from a newer build"} {
		if !strings.Contains(got, want) {
			t.Errorf("saved file lacks %q:\n%s", want, got)
		}
	}
}

func TestBrokenFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("theme: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load of a broken file worked")
	}
	if err := Save(path, Settings{Theme: "tracks"}); err == nil {
		t.Error("Save over a broken file worked; it would lose what's in it")
	}
}
