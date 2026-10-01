package cursor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallRule(t *testing.T) {
	home := t.TempDir()
	path, ok, err := InstallRule(home)
	if err != nil || !ok || path != filepath.Join(home, ".cursor", "rules", "tracks-v2.mdc") {
		t.Fatalf("InstallRule = %s, %v, %v", path, ok, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"alwaysApply: true", "both `TRACKS_ID` and `TRACKS_NEW_APP` are set",
		"`tracks terminal`", "`tracks review`", "`tracks add-repo <repo>`", "`tracks promote`", "TRACKS_PR_URL=<url>"} {
		if !strings.Contains(got, want) {
			t.Errorf("the rule lacks %q", want)
		}
	}

	if err := os.WriteFile(path, []byte("the user's own\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := InstallRule(home); err != nil || ok {
		t.Errorf("over the user's file: %v, %v; want it left", ok, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "the user's own\n" {
		t.Error("the user's rule was overwritten")
	}
}
