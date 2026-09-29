package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("history:\n  auto_archive: true\n  unsaved: keep\n  after: 30d # from a newer build\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || !s.History.AutoArchive || !s.History.KeepUnsaved() {
		t.Fatalf("Load: %+v, %v", s.History, err)
	}

	s.History = History{Unsaved: UnsavedSkip}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if got := string(data); strings.Contains(got, "auto_archive") || !strings.Contains(got, "unsaved: skip") || !strings.Contains(got, "after: 30d") {
		t.Errorf("saved:\n%s\nwant auto_archive gone, unsaved: skip and the unknown key kept", got)
	}
	if s, _ := Load(path); s.History.AutoArchive || s.History.KeepUnsaved() {
		t.Errorf("after Save: %+v", s.History)
	}
}
