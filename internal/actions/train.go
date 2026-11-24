package actions

import (
	"strings"

	"github.com/bluegardenproject/tracks/internal/track"
)

// HideTrainRange and EndTrainRange name the click ranges of a train's
// overlay header: Hide detaches the overlay, End train ends the train.
const (
	HideTrainRange = "train-hide"
	EndTrainRange  = "train-end"
)

// HideTrainVariant and EndTrainVariant are the colour variants of the
// header's buttons. End train has one of its own, outside the rotation.
const (
	HideTrainVariant = 1
	EndTrainVariant  = 5
)

// BadgeBg and BadgeText name the global options holding the colours of
// a badge of kind, such as track.BadgeInfo.
func BadgeBg(kind string) string   { return "@tracks_badge_" + kind + "_bg" }
func BadgeText(kind string) string { return "@tracks_badge_" + kind + "_text" }

// BadgeKinds are the badge kinds a status can take.
var BadgeKinds = []string{track.BadgeInfo, track.BadgeWarning, track.BadgeDanger}

// TrainHeader is the tmux format of the overlay header of train handle,
// running agent: its handle and agent, its badges from the session
// option badgeOption, and Hide and End train on the right.
func TrainHeader(handle, agent, badgeOption string) string {
	name := strings.ReplaceAll(handle+" · "+agent, "#", "##")
	return "#[align=left] " + name + "  #{E:" + badgeOption + "}" +
		"#[align=right]" + button(HideTrainRange, HideTrainVariant, " Hide ") + " " +
		button(EndTrainRange, EndTrainVariant, " End train ") + " "
}

// Badges is the tmux format of statuses as badges, side by side; a
// status without a label is left out.
func Badges(statuses ...track.Status) string {
	var parts []string
	for _, s := range statuses {
		if s.Label == "" {
			continue
		}
		parts = append(parts, style("bg", BadgeBg(s.Badge))+style("fg", BadgeText(s.Badge))+" "+
			strings.ReplaceAll(s.Label, "#", "##")+" #[default]")
	}
	return strings.Join(parts, " ")
}

// button is the tmux format of a button labelled label, its click range
// name, in colour variant v.
func button(name string, v int, label string) string {
	return "#[range=user|" + name + "]" + style("bg", VariantBg(v)) + style("fg", VariantText(v)) + label + "#[norange default]"
}
