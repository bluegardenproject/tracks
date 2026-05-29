package cmd

import (
	"os"
	"strings"
)

// selfBinary returns the absolute path to the running `tracks`
// binary. Used to construct commands that re-invoke `tracks` (e.g.
// `tracks log <id>` running inside a tmux window).
func selfBinary() (string, error) {
	return os.Executable()
}

// shellQuote returns s suitable for inclusion in a shell command
// line. Only quotes when needed.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t\n\"'`$&|;()<>\\") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
