package tracksview

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/ui/addtrack"
	"github.com/bluegardenproject/tracks/internal/v2/ui/quickaccess"
	"github.com/bluegardenproject/tracks/internal/v2/ui/tracksfilter"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// keyHelp is a key and what it does. The hints and the Keys section
// read the same lists.
type keyHelp struct{ key, help string }

var (
	tabKeys          = []keyHelp{{"Tab", "next tab"}, {"Shift+Tab", "previous tab"}}
	stationKeys      = []keyHelp{{"↑/↓", "select"}, {"Enter", "open"}}
	endedStationKeys = []keyHelp{{"↑/↓", "select"}, {"Enter", "resume"}}
	archivedKeys     = []keyHelp{{"↑/↓", "select"}, {"Enter", "unarchive"}}
	clearFilterKeys  = []keyHelp{{"x", "clear filter"}}
	selectKeys       = []keyHelp{{"↑/↓", "select"}}
	emptyStationKeys = []keyHelp{{"Enter", "add a new track"}}
	repoListKeys     = []keyHelp{{"↑/↓", "select"}, {"Enter", "edit"}, {"n", "new"}}
	repoFormKeys     = []keyHelp{{"Tab", "next field"}, {"Shift+Tab", "previous field"}, {"Space", "toggle"}, {"Ctrl+C/V", "copy, paste"}, {"Esc", "back to the list"}}
	settingsListKeys = []keyHelp{{"↑/↓", "section"}, {"Enter", "open"}}
	themeFieldKeys   = []keyHelp{{"Enter", "pick a theme"}, {"Esc", "back"}}
	pickerKeys       = []keyHelp{{"↑/↓", "select"}, {"Enter", "choose"}, {"Esc", "close"}}
	creatorKeys      = []keyHelp{{"Tab", "next"}, {"↑/↓", "token"}, {"Enter", "next or press"}, {"Ctrl+C/V", "copy, paste"}, {"Esc", "back"}}
	scrollKeys       = []keyHelp{{"↑/↓", "scroll"}, {"Esc", "back"}}
	engineListKeys   = []keyHelp{{"↑/↓", "scroll"}, {"Enter", "to the controls"}}
	engineKeys       = []keyHelp{{"Tab", "next"}, {"Shift+Tab", "previous"}, {"Enter", "press"}, {"Esc", "back"}}
	typeKeys         = []keyHelp{{"←/→", "track type"}, {"↑/↓", "field"}, {"Enter", "choose"}, {"Esc", "back"}}
	modelPickerKeys  = []keyHelp{{"Type", "to filter"}, {"↑/↓", "select"}, {"Enter", "choose"}, {"Esc", "close"}}
)

// questionKeys answer q.
func questionKeys(q question) []keyHelp {
	switch {
	case q.kind == askEnd:
		return []keyHelp{{"y/Enter", "end track"}, {"n/Esc", "cancel"}}
	case q.kind == askRecreate:
		return []keyHelp{{"y", "re-create"}, {"n/Esc/Enter", "cancel"}}
	case q.kind == askArchive && len(q.lines) > 0:
		return []keyHelp{{"y", "remove and archive"}, {"n/Esc/Enter", "cancel"}}
	case q.kind == askArchive:
		return []keyHelp{{"y/Enter", "archive"}, {"n/Esc", "cancel"}}
	case q.kind == askDerail && len(q.lines) > 0:
		return []keyHelp{{"y", "derail anyway"}, {"n/Esc/Enter", "cancel"}}
	case q.kind == askDerail:
		return []keyHelp{{"y", "derail"}, {"n/Esc/Enter", "cancel"}}
	case len(q.lines) > 0:
		return []keyHelp{{"y", "remove anyway"}, {"n/Esc/Enter", "cancel"}}
	}
	return []keyHelp{{"y/Enter", "remove"}, {"n/Esc", "cancel"}}
}

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
	station = append(station, keyHelp{"Enter", "resume an ended track"})
	for _, a := range slices.Concat(openActions, endedActions[:4], archivedActions[:1]) {
		station = append(station, keyHelp{a.key, strings.ToLower(a.label[:1]) + a.label[1:]})
	}
	station = append(station, keyHelp{"x", "clear the filter"})
	return []keyGroup{
		{"Tracks window", tabKeys},
		{"Station", station},
		{"Repositories", append(append([]keyHelp{}, repoListKeys...), repoFormKeys...)},
		{"Engines", append(append([]keyHelp{}, engineListKeys...), engineKeys...)},
		{"Model picker", modelPickerKeys},
		{"Settings", append(append([]keyHelp{}, settingsListKeys...), themeFieldKeys...)},
		{"Theme picker", pickerKeys},
		{"Theme Creator", creatorKeys},
		{"Every window", append(prefixKeys(), keyHelp{"Click", "a track in the footer to switch to it"})},
		{"Quick Access", fromWidget(quickaccess.Keys)},
		{"New track", fromWidget(addtrack.Keys)},
		{"Tracks filter", fromWidget(tracksfilter.Keys)},
	}
}

func fromWidget(keys []widget.KeyHelp) []keyHelp {
	out := make([]keyHelp, len(keys))
	for i, k := range keys {
		out[i] = keyHelp{k.Key, k.Help}
	}
	return out
}

func joinKeys(key, text lipgloss.Style, keys []keyHelp) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = key.Render(k.key) + text.Render(" "+k.help)
	}
	return strings.Join(parts, text.Render(" · "))
}
