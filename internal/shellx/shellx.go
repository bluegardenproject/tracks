// Package shellx quotes strings for inclusion in a /bin/sh command
// line.
//
// It exists because three copies of this logic had drifted into two
// behaviours — always-quote in the spawn and service paths,
// quote-only-when-needed in the CLI's tmux command builder. Both are
// correct and both are wanted; what was missing was a name for the
// difference. Callers now pick one deliberately instead of inheriting
// whichever copy their package happened to hold.
package shellx

import "strings"

// shellSpecial are the characters that force quoting: whitespace, the
// quote characters themselves, and the metacharacters /bin/sh acts on
// — including the glob characters, which expand against the current
// directory rather than being passed through.
const shellSpecial = " \t\n\"'`$&|;()<>\\*?[]{}~"

// Quote wraps s in single quotes, escaping any it contains. The result
// is always quoted, even when it needn't be.
//
// Use this for anything assembled into a command line programmatically
// — argv elements, paths, prompts — where being unconditionally safe
// matters more than being readable. An empty string becomes `”`,
// which is a real empty argument rather than nothing at all.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// QuoteIfNeeded quotes s only when it contains a character the shell
// would act on — whitespace, quotes, metacharacters, or a glob — and
// returns it untouched otherwise.
//
// Use this for command lines a human reads — the strings tracks hands
// to tmux, which show up in window listings and in the menu. Quoting a
// plain binary path there adds noise without adding safety. The
// escaping, when it does apply, is identical to Quote.
func QuoteIfNeeded(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, shellSpecial) {
		return Quote(s)
	}
	return s
}
