package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/agents"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name, yaml string
		says       string // "" for a manifest that parses
	}{
		{"minimal", "protocol: 1\nname: p\ncomponents:\n  - {kind: track, name: t, base: ask, brief: b.md}\n", ""},
		{"minor as a number", "protocol: 1.0\nname: p\ncomponents:\n  - {kind: track, name: t, base: ask, brief: b.md}\n", ""},
		{"minor as text", "protocol: \"1.0\"\nname: p\ncomponents:\n  - {kind: track, name: t, base: ask, brief: b.md}\n", ""},
		{"empty", "", "the file is empty"},
		{"not yaml", "protocol: [1\n", "isn't valid YAML"},
		{"unknown field", "protocol: 1\nname: p\nversion: 2\n", "line 3: unknown field version"},
		{"typo in a component", "protocol: 1\nname: p\ncomponents:\n  - {kind: train, name: t, permission: read}\n", "unknown field permission"},
		{"wrong type", "protocol: 1\nname: p\nengines: claude\n", "should be a list"},
		{"bad protocol", "protocol: one\nname: p\n", `protocol "one" isn't a version`},
		{"newer major", "protocol: 2\nname: p\ncomponents:\n  - {kind: track, name: t, base: ask, brief: b.md}\n", "speaks protocol 1 only"},
		{"several problems", "protocol: 1\nname: P\ncomponents: []\n", "has 2 problems"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Parse([]byte(tt.yaml))
			if tt.says == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var ps Problems
			if !errors.As(err, &ps) {
				t.Fatalf("err %v isn't Problems", err)
			}
			if !strings.Contains(err.Error(), tt.says) {
				t.Errorf("%q doesn't say %q", err, tt.says)
			}
		})
	}
}

func TestLoadFixture(t *testing.T) {
	m, warnings, err := Load("testdata/acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) > 0 || m.Name != "acme-planning" || len(m.Components) != 2 {
		t.Fatalf("warnings %q, manifest %+v", warnings, m)
	}
	if m.Settings[1].Default != true || m.Components[1].Relay == nil || !m.Components[1].Relay.Reads {
		t.Errorf("manifest %+v", m)
	}
}

func TestLoadFiles(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		setup func(dir string) error
		says  string
	}{
		{"missing brief", func(dir string) error { return os.Remove(filepath.Join(dir, "b.md")) }, "b.md isn't in the plugin's folder"},
		{"brief links out", func(dir string) error {
			if err := os.Remove(filepath.Join(dir, "b.md")); err != nil {
				return err
			}
			return os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "b.md"))
		}, "leads out of the plugin's folder"},
		{"brief is a folder", func(dir string) error {
			if err := os.Remove(filepath.Join(dir, "b.md")); err != nil {
				return err
			}
			return os.Mkdir(filepath.Join(dir, "b.md"), 0o700)
		}, "should be a file"},
		{"no manifest", func(dir string) error { return os.Remove(filepath.Join(dir, File)) }, "no plugin.yaml"},
		{"manifest is a link", func(dir string) error {
			data, err := os.ReadFile(filepath.Join(dir, File))
			if err != nil {
				return err
			}
			target := filepath.Join(outside, "plugin.yaml")
			if err := os.WriteFile(target, data, 0o600); err != nil {
				return err
			}
			if err := os.Remove(filepath.Join(dir, File)); err != nil {
				return err
			}
			return os.Symlink(target, filepath.Join(dir, File))
		}, "isn't a plain file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			manifest := "protocol: 1\nname: p\ncomponents:\n  - {kind: track, name: t, base: ask, brief: b.md}\n"
			for name, body := range map[string]string{File: manifest, "b.md": "brief"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := tt.setup(dir); err != nil {
				t.Fatal(err)
			}
			_, _, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), tt.says) {
				t.Errorf("err %v, want %q", err, tt.says)
			}
		})
	}
}

func TestEnginesAreTheRunners(t *testing.T) {
	var ids []string
	for _, e := range agents.Runners() {
		ids = append(ids, e.ID)
	}
	if !slices.Equal(Engines, ids) {
		t.Errorf("plugin engines %v, runners %v", Engines, ids)
	}
}
