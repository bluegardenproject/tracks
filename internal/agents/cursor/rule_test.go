package cursor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallRule(t *testing.T) {
	home := t.TempDir()
	if skipped, err := InstallRules(home); err != nil || len(skipped) > 0 {
		t.Fatalf("InstallRules = %q, %v", skipped, err)
	}
	path := filepath.Join(home, ".cursor", "rules", "tracks.mdc")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"alwaysApply: true", "only when the `TRACKS_ID` environment variable is set",
		"`tracks terminal`", "`tracks review`", "`tracks add-repo <repo>`", "`tracks promote`", "TRACKS_PR_URL=<url>",
		"`~/.cursor/rules/tracks-pr-comments.mdc`"} {
		if !strings.Contains(got, want) {
			t.Errorf("the rule lacks %q", want)
		}
	}
	if strings.Contains(got, "TRACKS_NEW_APP") {
		t.Error("the rule still asks for TRACKS_NEW_APP")
	}

	if err := os.WriteFile(path, []byte("the user's own\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if skipped, err := InstallRules(home); err != nil || len(skipped) != 1 || skipped[0] != path {
		t.Errorf("over the user's file: %q, %v; want it left", skipped, err)
	}
	if b, err := os.ReadFile(filepath.Join(home, ".cursor", "rules", "tracks-pr-comments.mdc")); err != nil ||
		!strings.Contains(string(b), "alwaysApply: false") {
		t.Errorf("the PR comments rule: %v; want it written, loaded only when asked for", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "the user's own\n" {
		t.Error("the user's rule was overwritten")
	}
}
