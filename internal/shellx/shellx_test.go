package shellx

import (
	"os/exec"
	"strings"
	"testing"
)

// The two functions must agree on escaping and differ only on whether
// a safe string is left bare — that distinction is the whole reason
// both exist.
func TestQuoteAlwaysQuotesAndQuoteIfNeededDoesNot(t *testing.T) {
	cases := []struct {
		in              string
		want, wantMaybe string
	}{
		{"plain", "'plain'", "plain"},
		{"/usr/local/bin/tracks", "'/usr/local/bin/tracks'", "/usr/local/bin/tracks"},
		{"has space", "'has space'", "'has space'"},
		{"it's", `'it'\''s'`, `'it'\''s'`},
		{"a$b", `'a$b'`, `'a$b'`},
		{"", "''", "''"},
	}
	for _, c := range cases {
		if got := Quote(c.in); got != c.want {
			t.Errorf("Quote(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := QuoteIfNeeded(c.in); got != c.wantMaybe {
			t.Errorf("QuoteIfNeeded(%q) = %q, want %q", c.in, got, c.wantMaybe)
		}
	}
}

// Both forms have to survive a real shell, which is the property the
// string comparisons above are a proxy for. Anything that round-trips
// wrong here is a command-injection or a broken-argument bug.
func TestQuotingRoundTripsThroughARealShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	inputs := []string{
		"plain", "has space", "it's", `double"quote`, "a$b", "back`tick`",
		"semi;colon", "pipe|pipe", "amp&amp", "paren(s)", "redir>out",
		`back\slash`, "new\nline", "tab\there", "-dash", "*glob*", "~tilde",
	}
	for _, in := range inputs {
		for name, quoted := range map[string]string{"Quote": Quote(in), "QuoteIfNeeded": QuoteIfNeeded(in)} {
			out, err := exec.Command("sh", "-c", "printf %s "+quoted).Output()
			if err != nil {
				t.Errorf("%s(%q) produced an unrunnable command %s: %v", name, in, quoted, err)
				continue
			}
			if string(out) != in {
				t.Errorf("%s(%q) round-tripped through sh as %q", name, in, string(out))
			}
		}
	}
}

// An empty argument must survive as an argument, not vanish — the
// reason both functions special-case it.
func TestEmptyStaysAnArgument(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	for name, q := range map[string]string{"Quote": Quote(""), "QuoteIfNeeded": QuoteIfNeeded("")} {
		out, err := exec.Command("sh", "-c", "set -- "+q+"; echo $#").Output()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.TrimSpace(string(out)) != "1" {
			t.Errorf("%s(\"\") produced %s arguments, want 1", name, strings.TrimSpace(string(out)))
		}
	}
}
