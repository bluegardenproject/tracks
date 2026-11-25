// Package termtext strips terminal escape sequences and control
// characters from untrusted text before it reaches a terminal.
package termtext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Multiline returns s without terminal escape sequences, control
// characters other than newlines and tabs, invalid UTF-8 bytes, or
// invisible format characters (such as BiDi overrides and zero-width
// spaces). CRLF line endings become LF, and U+200D (ZWJ) is kept so
// joined emoji stay intact.
func Multiline(s string) string {
	return sanitize(s, true)
}

// SingleLine returns s for a single terminal line: terminal escape
// sequences, invalid UTF-8 bytes, controls and invisible format
// characters are stripped, and line breaks or tabs become spaces.
// U+200D (ZWJ) is kept so joined emoji stay intact.
func SingleLine(s string) string {
	return sanitize(s, false)
}

func sanitize(s string, multiline bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			i = skipESC(s, i)
			continue
		}
		if s[i] == '\r' {
			if i+1 < len(s) && s[i+1] == '\n' {
				i += 2
				if multiline {
					b.WriteByte('\n')
				} else {
					b.WriteByte(' ')
				}
				continue
			}
			i++
			if !multiline {
				b.WriteByte(' ')
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte: drop raw 8-bit C1 escape sequences too.
			i = skipRawC1(s, i)
			continue
		}
		if r >= 0x80 && r <= 0x9f {
			i = skipC1Rune(s, i, size, r)
			continue
		}
		i += size
		switch {
		case r == '\n' || r == '\t':
			if multiline {
				b.WriteRune(r)
			} else {
				b.WriteByte(' ')
			}
		case r == '\u2028' || r == '\u2029':
			if multiline {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		case unicode.IsControl(r):
			// Drop all other C0/DEL/C1 controls.
		case r == '\u200d':
			// Keep ZWJ for emoji sequences.
			b.WriteRune(r)
		case unicode.Is(unicode.Cf, r):
			// Drop BiDi overrides/isolates, zero-width spaces, BOM, etc.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// skipESC skips an ESC-introduced terminal sequence starting at s[i].
func skipESC(s string, i int) int {
	i++ // consume ESC
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[': // CSI
		return skipCSI(s, i+1)
	case ']': // OSC (ends at BEL or ST)
		return skipString(s, i+1, true)
	case 'P', 'X', '^', '_': // DCS, SOS, PM, APC (ends at ST)
		return skipString(s, i+1, false)
	}
	// nF sequence: intermediate bytes 0x20..0x2f, then final byte 0x30..0x7e.
	if s[i] >= 0x20 && s[i] <= 0x2f {
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
			i++
		}
		if i < len(s) && s[i] >= 0x30 && s[i] <= 0x7e {
			i++
		}
		return i
	}
	// 2-byte Fe/Fp/Fs sequence: 0x30..0x7e.
	if s[i] >= 0x30 && s[i] <= 0x7e {
		return i + 1
	}
	return i
}

// skipC1Rune skips a UTF-8 C1 control rune at s[i:i+size], consuming
// any CSI/OSC/DCS/SOS/PM/APC payload that follows it.
func skipC1Rune(s string, i, size int, r rune) int {
	next := i + size
	switch r {
	case 0x9b: // CSI
		return skipCSI(s, next)
	case 0x9d: // OSC
		return skipString(s, next, true)
	case 0x90, 0x98, 0x9e, 0x9f: // DCS, SOS, PM, APC
		return skipString(s, next, false)
	}
	return next
}

// skipRawC1 drops an invalid UTF-8 byte at s[i], also skipping an 8-bit
// C1 escape payload when the byte is a C1 introducer.
func skipRawC1(s string, i int) int {
	switch s[i] {
	case 0x9b:
		return skipCSI(s, i+1)
	case 0x9d:
		return skipString(s, i+1, true)
	case 0x90, 0x98, 0x9e, 0x9f:
		return skipString(s, i+1, false)
	}
	return i + 1
}

// skipCSI consumes CSI parameter bytes (0x30..0x3f), intermediate bytes
// (0x20..0x2f), and final byte (0x40..0x7e) starting at s[i].
func skipCSI(s string, i int) int {
	for i < len(s) && s[i] >= 0x30 && s[i] <= 0x3f {
		i++
	}
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
		i++
	}
	if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
		i++
	}
	return i
}

// skipString consumes an OSC/DCS/SOS/PM/APC string payload starting at
// s[i] until ST (ESC \, U+009C, or raw 0x9c), BEL when allowBEL is set,
// or the end of s.
func skipString(s string, i int, allowBEL bool) int {
	for i < len(s) {
		if allowBEL && s[i] == '\x07' {
			return i + 1
		}
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
			return i + 2
		}
		if s[i] == 0x9c {
			return i + 1
		}
		if s[i] >= 0x80 {
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == 0x9c {
				return i + size
			}
			i += size
			continue
		}
		i++
	}
	return i
}
