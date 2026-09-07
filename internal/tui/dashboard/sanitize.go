package dashboard

import "strings"

// stripControl removes control characters from a string that came from
// outside tracks: a tab becomes a space, and everything else below
// U+0020 — plus DEL and the C1 block — is dropped.
//
// The dashboard renders text git and gh hand it verbatim, and neither
// constrains what that text contains. A commit subject can carry a raw
// ESC, so can a path in a diff, and a review track checks out a branch
// somebody else wrote; a `git fetch` failure quotes whatever the remote
// sent back. Printed as-is, those bytes are not data to the terminal —
// they are commands, and they can repaint the frame, retitle the window,
// or on a terminal that answers OSC 52 reach the clipboard.
//
// The tab is a narrower problem with the same shape: `git diff
// --name-status` separates status from path with one, lipgloss measures
// it as a single cell, and the terminal expands it to the next tab stop
// — so a tab overflows the column it was measured for. A space keeps the
// separation and the arithmetic.
//
// Invalid UTF-8 comes out as one U+FFFD per bad byte, because that is
// what strings.Map does with it. So a latin-1 commit subject renders as
// replacement characters rather than raw bytes — which is the outcome we
// want here, since those bytes are what the terminal would have had to
// guess at.
//
// Deliberately not covered: bidi overrides (U+202E and the U+2066–2069
// isolates) and zero-width characters. They can misrepresent a diff path
// in the same spirit as an escape, but dropping them wholesale would
// mangle legitimate right-to-left text, and unlike an escape they cannot
// make the terminal *act*. Worth revisiting with a narrower rule.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		}
		return r
	}, s)
}
