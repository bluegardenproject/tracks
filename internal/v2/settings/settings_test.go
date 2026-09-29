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

func TestDefaultEngine(t *testing.T) {
	var e Engines
	if id := e.DefaultID(); id != "" {
		t.Errorf("no engines: default %q", id)
	}
	e.Cursor = &Engine{}
	if id := e.DefaultID(); id != "cursor" {
		t.Errorf("only Cursor: default %q", id)
	}
	e.Claude = &Engine{}
	if id := e.DefaultID(); id != "claude" {
		t.Errorf("both: default %q, want claude", id)
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

func TestRunsOn(t *testing.T) {
	var s Settings
	if e, m := s.RunsOn("work"); e != "" || m != "" {
		t.Errorf("no engines: %q, %q", e, m)
	}
	s.Engines.Cursor = &Engine{Model: "gpt-5"}
	if e, m := s.RunsOn("work"); e != "cursor" || m != "gpt-5" {
		t.Errorf("only Cursor: %q, %q", e, m)
	}
	s.Engines.Claude = &Engine{Model: "opus"}
	if e, m := s.RunsOn("work"); e != "claude" || m != "opus" {
		t.Errorf("unset with Claude: %q, %q", e, m)
	}
	s.Tracks.Set("ask", &TrackType{Engine: "cursor"})
	s.Tracks.Set("plan", &TrackType{Engine: "claude", Model: "sonnet"})
	if e, m := s.RunsOn("ask"); e != "cursor" || m != "gpt-5" {
		t.Errorf("ask on Cursor's default: %q, %q", e, m)
	}
	if e, m := s.RunsOn("plan"); e != "claude" || m != "sonnet" {
		t.Errorf("plan on sonnet: %q, %q", e, m)
	}
	s.Engines.Cursor = nil
	if e, m := s.RunsOn("ask"); e != "cursor" || m != "" {
		t.Errorf("ask on Cursor, removed: %q, %q; want cursor all the same", e, m)
	}
}

func TestTracksKeepWhatThisBuildDoesntKnow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	newer := "tracks:\n  work:\n    engine: claude\n    effort: high # newer\n  triage: {engine: cursor}\n"
	if err := os.WriteFile(path, []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.Tracks.Get("work") == nil || s.Tracks.Get("work").Engine != "claude" {
		t.Fatalf("Load: %+v, %v", s.Tracks, err)
	}
	s.Tracks.Set("work", &TrackType{Engine: "claude", Model: "opus"})
	s.Tracks.Set("doc", &TrackType{Engine: "cursor"})
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"model: opus", "effort: high # newer", "triage: {engine: cursor}", "doc:\n        engine: cursor"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("saved file lacks %q:\n%s", want, data)
		}
	}
	s.Tracks.Set("work", nil)
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "work") || !strings.Contains(string(data), "triage") {
		t.Errorf("unsetting Work should keep the rest:\n%s", data)
	}
}
