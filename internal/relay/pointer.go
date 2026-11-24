package relay

import (
	"fmt"
	"slices"
	"strings"
)

// Pointer is the line Tracks types into an agent's pane when msgs came
// for it: how many, from whom, and the command that reads them, never
// the messages themselves. It starts with a digit, so it can't switch
// an agent's input to a shell or slash command, and holds nothing but a
// count and handles.
func Pointer(msgs []Message) string {
	var from []string
	for _, m := range msgs {
		h := "@" + m.From
		if !slices.Contains(from, h) {
			from = append(from, h)
		}
	}
	count := "1 message"
	if len(msgs) != 1 {
		count = fmt.Sprintf("%d messages", len(msgs))
	}
	if len(from) == 1 {
		return count + " from " + from[0] + " on the relay: run `tracks relay read`."
	}
	return count + " on the relay, from " + and(from) + ": run `tracks relay read`."
}

// and lists items as people do: a, b and c.
func and(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
