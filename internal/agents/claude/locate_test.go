package claude

import "testing"

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
