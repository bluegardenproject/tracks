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

func TestEnginesKeepWhatThisBuildDoesntKnow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	newer := "theme: my_theme\nengines:\n  claude:\n    model: opus\n    effort: high # newer\n  codex: {}\n"
	if err := os.WriteFile(path, []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.Engines.Claude == nil || s.Engines.Claude.Model != "opus" || !s.Engines.Claude.AutoMode() {
		t.Fatalf("Load: %+v, %v; want Claude on opus in auto mode", s.Engines.Claude, err)
	}

	off := false
	s.Engines.Claude.Models = []string{"claude-opus-5-5"}
	s.Engines.Claude.Auto = &off
	s.Engines.Set("cursor", &Engine{})
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"theme: my_theme", "effort: high # newer", "codex: {}", "claude-opus-5-5", "auto: false", "cursor: {}"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("saved file lacks %q:\n%s", want, data)
		}
	}

	s.Engines.Set("claude", nil)
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "claude") || !strings.Contains(string(data), "codex: {}") {
		t.Errorf("removing Claude should keep the rest:\n%s", data)
	}

	s.Engines.Cursor.Model = "gpt-5"
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "{model") {
		t.Errorf("an engine that was {} should be written as a block once it has keys:\n%s", data)
	}
}

func TestRemovingTheLastEngineKeepsUnknownOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("engines:\n  claude:\n    effort: high\n  codex: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Engines.Set("claude", nil)
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "claude") || !strings.Contains(string(data), "codex: {}") {
		t.Errorf("removing the last engine should remove it, with its unknown keys, and keep codex:\n%s", data)
	}
}
