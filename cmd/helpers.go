package cmd

import (
	"os"
)

// selfBinary returns the absolute path to the running `tracks`
// binary. Used to construct commands that re-invoke `tracks` (e.g.
// `tracks dashboard` running inside a tmux window, or the menu popup
// bound to the tmux prefix key).
func selfBinary() (string, error) {
	return os.Executable()
}
