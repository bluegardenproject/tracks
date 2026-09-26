package tmux

import (
	"fmt"
	"strconv"
)

// Binding is a key behind the prefix (Ctrl+b) in every window of a
// Tracks session. The generated config binds it and the Settings tab
// lists it, so they can't disagree.
type Binding struct {
	Keys  []string // each bound the same way
	Label string   // how Keys are shown, when not the one key
	Help  string
	args  func(key string) string // what a key runs, after the Tracks command
}

// Shown is how the binding's keys are written.
func (b Binding) Shown() string {
	if b.Label != "" {
		return b.Label
	}
	return b.Keys[0]
}

// Bindings are the prefix keys, in the order they're listed.
var Bindings = func() []Binding {
	digits := make([]string, 9)
	for i := range digits {
		digits[i] = strconv.Itoa(i + 1)
	}
	return []Binding{
		{Keys: []string{"n"}, Help: "next track", args: nav("next")},
		{Keys: []string{"p"}, Help: "previous track", args: nav("prev")},
		{Keys: []string{"<"}, Help: "first track", args: nav("first")},
		{Keys: []string{">"}, Help: "last track", args: nav("last")},
		{Keys: digits, Label: "1…9", Help: "track by number", args: func(key string) string { return nav(key)(key) }},
		{Keys: []string{"t"}, Help: "add a terminal to the track", args: func(string) string {
			return "trackwin add-terminal '#{window_id}'"
		}},
	}
}()

// nav switches tracks; it lands on the track's agent pane.
func nav(move string) func(string) string {
	return func(string) string { return "trackwin nav " + move + " '#{window_index}'" }
}

// Binds are the config's bind-key lines for Bindings.
func (c Conf) Binds() []string {
	var lines []string
	for _, b := range Bindings {
		for _, key := range b.Keys {
			lines = append(lines, fmt.Sprintf("bind-key %s run-shell -b \"%s %s\"", key, c.Command, b.args(key)))
		}
	}
	return lines
}
