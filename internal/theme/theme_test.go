package theme

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestBuiltInsAreComplete(t *testing.T) {
	got := BuiltIns()
	if len(got) != 2 || got[0].ID != DefaultID || got[0].DisplayName != "Default" || got[1].ID != "default_light" {
		t.Fatalf("built-ins %v; want Default, then Default Light", got)
	}
	for _, b := range got {
		for _, token := range All {
			if b.Value(token) == "" {
				t.Errorf("%s: token %s has no value", b.ID, token)
			}
		}
	}
}

func TestParseRejects(t *testing.T) {
	complete := func() string {
		var b strings.Builder
		b.WriteString("display_name: t\ntokens:\n")
		for _, token := range All {
			b.WriteString("  " + string(token) + `: "#000000"` + "\n")
		}
		return b.String()
	}()
	if _, err := Parse([]byte(complete)); err != nil {
		t.Fatalf("complete theme rejected: %v", err)
	}
	tests := map[string]string{
		"missing token": strings.Replace(complete, "  text.accent:", "  # text.accent:", 1),
		"unknown token": complete + `  text.shiny: "#000000"` + "\n",
		"not hex":       strings.Replace(complete, `text.accent: "#000000"`, `text.accent: "12"`, 1),
		"short hex":     strings.Replace(complete, `text.accent: "#000000"`, `text.accent: "#000"`, 1),
		"old format":    strings.Replace(complete, `text.accent: "#000000"`, `text.accent: { dark: "#000000", light: "#ffffff" }`, 1),
		"not yaml":      "tokens: [",
	}
	for name, data := range tests {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Colours come only from tokens, so no colour value may appear in v2
// code outside this package. Tests may use values as data.
func TestNoColourLiteralsOutsideTheme(t *testing.T) {
	literal := regexp.MustCompile(`"#[0-9a-fA-F]{3,8}"|lipgloss\.Color\("|\\x1b\[[34]8;|\\033\[[34]8;|\b[fb]g=(#|colou?r\d)`)
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	self, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path == self {
			return filepath.SkipDir
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if m := literal.FindString(line); m != "" {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d: colour literal %s; use a theme token", rel, i+1, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
