package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShortModel(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5":             "opus-5",
		"claude-sonnet-5":           "sonnet-5",
		"claude-haiku-4-5-20251001": "haiku-4-5",
		"claude-opus-4-8":           "opus-4-8",
		"claude-fable-5":            "fable-5",
		"  claude-opus-5  ":         "opus-5",
		"":                          "",
		"something-unrecognised":    "something-unrecognised",
		"claude-future-9-20991231":  "future-9",
		// Gateway-hosted ids shorten to the same thing as the direct id.
		"anthropic.claude-opus-4-20250514-v1:0": "opus-4",
		"claude-opus-4@20250514":                "opus-4",
	}
	for in, want := range cases {
		if got := ShortModel(in); got != want {
			t.Errorf("ShortModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeCWD(t *testing.T) {
	got := sanitizeCWD("/Users/x/.local/state/tracks/worktrees/abc/repo")
	want := "-Users-x--local-state-tracks-worktrees-abc-repo"
	if got != want {
		t.Errorf("sanitizeCWD = %q, want %q", got, want)
	}
}

func TestLocateRejectsTraversalAndHandlesGlobChars(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	proj := filepath.Join(cfg, "projects", "-tmp-repo-[1]")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(proj, "sess-1.jsonl")
	if err := os.WriteFile(want, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(cfg, "secret.jsonl")
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := Locate("sess-1", ""); len(got) != 1 || got[0] != want {
		t.Errorf("Locate(sess-1) = %v, want [%s]", got, want)
	}
	for _, bad := range []string{"../secret", "../../secret", "*", "sess-?", "[s]ess-1"} {
		if got := Locate(bad, ""); len(got) != 0 {
			t.Errorf("Locate(%q) = %v, want empty", bad, got)
		}
	}
	// Symlinked .jsonl must be skipped in both branches.
	link := filepath.Join(proj, "link.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if got := Locate("link", ""); len(got) != 0 {
		t.Errorf("Locate(symlink sessionID) = %v, want empty", got)
	}
	// Fallback by cwd containing glob metacharacters [1] skips the symlink.
	if got := Locate("", "/tmp/repo/[1]"); len(got) != 1 || got[0] != want {
		t.Errorf("Locate(cwd with brackets) = %v, want [%s]", got, want)
	}
}
