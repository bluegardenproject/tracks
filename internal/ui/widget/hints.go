package widget

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// KeyHelp is a key and what it does, for hint rows and key lists.
type KeyHelp struct{ Key, Help string }

// Hints joins keys for a hint row, each key in key's style and its help
// in text's.
func Hints(key, text lipgloss.Style, keys []KeyHelp) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = key.Render(k.Key) + text.Render(" "+k.Help)
	}
	return strings.Join(parts, text.Render(" · "))
}
