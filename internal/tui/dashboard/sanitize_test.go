package dashboard

import (
	"strings"
	"testing"
)

func TestStripControl(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is untouched", "fix(update): verify the download", "fix(update): verify the download"},
		{"tab becomes a space", "M\tinternal/update/update.go", "M internal/update/update.go"},
		{"csi colour sequence loses its introducer", "subject \x1b[31mred\x1b[0m", "subject [31mred[0m"},
		{"osc window retitle loses both ends", "\x1b]0;retitled\x07subject", "]0;retitledsubject"},
		{"newline and carriage return go", "one\ntwo\rthree", "onetwothree"},
		{"del goes", "a\x7fb", "ab"},
		{"c1 csi goes", "a\u009bb", "ab"},
		{"non-ascii text survives", "café — naïve ✓", "café — naïve ✓"},
		{"empty stays empty", "", ""},
		// strings.Map decodes as it goes, so a bad byte becomes one
		// U+FFFD rather than passing through as a raw byte.
		{"invalid utf-8 becomes replacement chars", "a\xffb", "a\ufffdb"},
	}
	for _, c := range cases {
		if got := stripControl(c.in); got != c.want {
			t.Errorf("%s: stripControl(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// The point of the helper is that no escape introducer survives it, so
// nothing downstream can be talked into obeying one. A clipboard-write
// via OSC 52 is the worst realistic payload for a commit subject.
func TestStripControlLeavesNoEscapeIntroducer(t *testing.T) {
	in := "\x1b]52;c;cGF5bG9hZA==\x07 commit subject \x1b[2J\u009b6n"
	got := stripControl(in)
	for _, bad := range []string{"\x1b", "\x07", "\u009b"} {
		if strings.Contains(got, bad) {
			t.Errorf("stripControl left %q in %q", bad, got)
		}
	}
}
