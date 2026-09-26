package tracksview

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
)

// keyHelp is a key and what it does. The hints and the Keys section
// read the same lists.
type keyHelp struct{ key, help string }

var (
	tabKeys          = []keyHelp{{"Tab", "next tab"}, {"Shift+Tab", "previous tab"}}
	stationKeys      = []keyHelp{{"↑/↓", "select"}, {"Enter", "open"}}
	repoListKeys     = []keyHelp{{"↑/↓", "select"}, {"Enter", "edit"}, {"n", "new"}}
	repoFormKeys     = []keyHelp{{"Tab", "next field"}, {"Shift+Tab", "previous field"}, {"Space", "toggle"}, {"Ctrl+C/V", "copy, paste"}, {"Esc", "back to the list"}}
	settingsListKeys = []keyHelp{{"↑/↓", "section"}, {"Enter", "open"}}
	themeFieldKeys   = []keyHelp{{"Enter", "pick a theme"}, {"Esc", "back"}}
	pickerKeys       = []keyHelp{{"↑/↓", "select"}, {"Enter", "choose"}, {"Esc", "close"}}
	creatorKeys      = []keyHelp{{"Tab", "next"}, {"↑/↓", "token"}, {"Enter", "next or press"}, {"Ctrl+C/V", "copy, paste"}, {"Esc", "back"}}
	scrollKeys       = []keyHelp{{"↑/↓", "scroll"}, {"Esc", "back"}}
)

// prefixKeys are the keys every window of the session has, behind the
// tmux prefix.
func prefixKeys() []keyHelp {
	keys := make([]keyHelp, len(tmux.Bindings))
	for i, b := range tmux.Bindings {
		keys[i] = keyHelp{"Ctrl+b " + b.Shown(), b.Help}
	}
	return keys
}

// keyGroup is a titled list in the Keys section.
type keyGroup struct {
	title string
	keys  []keyHelp
}

func keyGroups() []keyGroup {
	station := append([]keyHelp{}, stationKeys...)
	for _, a := range actions {
		station = append(station, keyHelp{a.key, strings.ToLower(a.label[:1]) + a.label[1:]})
	}
	return []keyGroup{
		{"Tracks window", tabKeys},
		{"Station", station},
		{"Repositories", append(append([]keyHelp{}, repoListKeys...), repoFormKeys...)},
		{"Settings", append(append([]keyHelp{}, settingsListKeys...), themeFieldKeys...)},
		{"Theme picker", pickerKeys},
		{"Theme Creator", creatorKeys},
		{"Every window", append(prefixKeys(), keyHelp{"Click", "a track in the footer to switch to it"})},
	}
}

func joinKeys(key, text lipgloss.Style, keys []keyHelp) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = key.Render(k.key) + text.Render(" "+k.help)
	}
	return strings.Join(parts, text.Render(" · "))
}
