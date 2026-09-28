package tracks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDocument(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "spec.md")
	deck := filepath.Join(dir, "deck.pptx")
	for _, f := range []string{doc, deck} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for path, want := range map[string]string{doc: doc, dir: dir, " " + doc + "\n": doc} {
		if got, err := ResolveDocument(path); err != nil || got != want {
			t.Errorf("ResolveDocument(%q) = %q, %v", path, got, err)
		}
	}
	for path, want := range map[string]string{
		deck:                       "PowerPoint files can't be read directly. Export it to PDF and pick that.",
		"":                         "Enter the document's path.",
		filepath.Join(dir, "gone"): "There's no file or folder at " + filepath.Join(dir, "gone") + ".",
	} {
		if _, err := ResolveDocument(path); err == nil || err.Error() != want {
			t.Errorf("ResolveDocument(%q) = %v, want %q", path, err, want)
		}
	}
}
