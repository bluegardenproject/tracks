package notify

import "testing"

func TestQuoteEscapesDoubleQuotes(t *testing.T) {
	got := quote(`he said "hi"`)
	want := `"he said \"hi\""`
	if got != want {
		t.Errorf("quote: got %q want %q", got, want)
	}
}

// The escaping has to survive a backslash immediately before a quote.
// Escaping the quote alone turned `\"` into `\\"` — a literal backslash
// and then the closing quote — so everything after it in a track slug
// was parsed by osascript as AppleScript rather than shown as text.
func TestQuoteEscapesBackslashBeforeQuote(t *testing.T) {
	got := quote(`a\" & (do shell script "touch /tmp/pwned") & "`)
	want := `"a\\\" & (do shell script \"touch /tmp/pwned\") & \""`
	if got != want {
		t.Errorf("quote:\n got %s\nwant %s", got, want)
	}
}

// AppleScript has no multi-line string literal, so a raw newline is a
// syntax error and the notification silently never arrives.
func TestQuoteEscapesNewlines(t *testing.T) {
	got := quote("first\nsecond\rthird")
	want := `"first\nsecond\rthird"`
	if got != want {
		t.Errorf("quote: got %s want %s", got, want)
	}
}

// Every backslash in the result must be an escape we put there: an odd
// run of backslashes anywhere would mean something downstream is
// consuming the character after it.
func TestQuoteProducesBalancedEscapes(t *testing.T) {
	for _, in := range []string{
		`plain`, `back\slash`, `quote"`, `both\"`, `trailing\`, `\\\`, "nl\n",
	} {
		got := quote(in)
		if len(got) < 2 || got[0] != '"' || got[len(got)-1] != '"' {
			t.Fatalf("quote(%q) = %s, want it wrapped in double quotes", in, got)
		}
		body := got[1 : len(got)-1]
		// Walk the body; every backslash must be followed by a
		// character it is escaping, and never fall off the end.
		for i := 0; i < len(body); i++ {
			if body[i] != '\\' {
				if body[i] == '"' {
					t.Errorf("quote(%q) = %s: unescaped quote at %d", in, got, i)
				}
				continue
			}
			if i+1 >= len(body) {
				t.Errorf("quote(%q) = %s: trailing backslash escapes the closing quote", in, got)
				break
			}
			i++ // skip the escaped character
		}
	}
}

func TestSendNilNotifierIsNoop(t *testing.T) {
	var n *Notifier
	// Should not panic.
	n.Send("title", "body")
}

func TestSendBothChannelsDisabledIsNoop(t *testing.T) {
	n := New(Channel{})
	// Should not panic and should not contact /dev/tty or osascript.
	// We have no way to assert non-execution here, but it must not
	// crash on any platform.
	n.Send("title", "body")
}
