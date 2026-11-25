package termtext

import "testing"

func TestMultiline(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"plain", "hello world\nline 2\tok", "hello world\nline 2\tok"},
		{"crlf and stray cr", "a\r\nb\rc\n", "a\nbc\n"},
		{"unicode line separators", "a\u2028b\u2029c", "a\nb\nc"},
		{"csi sgr and clear", "\x1b[31mFAIL\x1b[0m \x1b[2J\x1b[?25ltext", "FAIL text"},
		{"osc bel and st", "\x1b]0;title\x07ok\x1b]52;c;ZGF0YQ==\x1b\\done", "okdone"},
		{"dcs apc pm sos", "\x1bP1+q\x1b\\a\x1b_Gi=1\x1b\\b\x1b^priv\x1b\\c\x1bXsos\x1b\\d", "abcd"},
		{"two byte and nf esc", "\x1b(Bhello\x1bc\x1b7world\x1b8", "helloworld"},
		{"utf8 8bit c1 csi and osc", "a\u009b2Jb\u009d0;title\u009cc", "abc"},
		{"raw 8bit c1 csi and osc", "a\x9b31mb\x9d0;title\x07c\xffd", "abcd"},
		{"unterminated csi and osc", "ok\x1b[31", "ok"},
		{"unterminated osc", "ok\x1b]0;forever", "ok"},
		{"c0 controls", "a\x00\x01\x07\x08\x7fb", "ab"},
		{"bidi and zero width stripped", "a\u202eb\u2066c\u200bd\ufeffe\u200ef", "abcdef"},
		{"zwj emoji preserved", "red \x1b[31mtext\x1b]0;title\x07 \u202e👩\u200d💻", "red text 👩\u200d💻"},
	} {
		if got := Multiline(tc.in); got != tc.want {
			t.Errorf("%s: Multiline(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestSingleLine(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"plain", "Fix the bug", "Fix the bug"},
		{"newlines and tabs", "a\r\nb\nc\td\re", "a b c d e"},
		{"unicode line separators", "a\u2028b\u2029c", "a b c"},
		{"ansi and bidi", "\x1b[31mFix\x1b[0m \u202e\x1b]0;t\x07bug\u200b", "Fix bug"},
		{"zwj emoji preserved", "👩\u200d💻 dev", "👩\u200d💻 dev"},
	} {
		if got := SingleLine(tc.in); got != tc.want {
			t.Errorf("%s: SingleLine(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
