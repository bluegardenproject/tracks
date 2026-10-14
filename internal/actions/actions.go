// Package actions are the track actions: buttons on the footer's top
// row, each for the track whose window is shown, that send its agent a
// prompt. Today there's one per pull request with open review comments.
// It says what a button reads, what it sends and when a sent one may be
// sent again; the daemon and the footer do the rest.
package actions

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// rangePrefix starts the click range of a PR's button: "pr:<i>" for the
// track's i-th PR. tmux caps a range's name at 15 bytes, too short for
// a link.
const rangePrefix = "pr:"

// Range names the click range of the button for the track's i-th PR.
func Range(i int) string { return rangePrefix + strconv.Itoa(i) }

// ParseRange returns which of the track's PRs range is the button of.
func ParseRange(name string) (int, bool) {
	rest, ok := strings.CutPrefix(name, rangePrefix)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(rest)
	return i, err == nil && i >= 0
}

// ButtonsOption is the window option holding a track's buttons, a tmux
// format the footer expands. Its colours come from global options the
// theme sets: Muted, and each variant's VariantBg and VariantText.
const (
	ButtonsOption = "@tracks_buttons"
	Muted         = "@tracks_muted"
)

// Variants is how many colour variants buttons take in turn.
const Variants = 4

// VariantBg and VariantText name the global options holding variant v's
// colours, v from 1.
func VariantBg(v int) string   { return "@tracks_button" + strconv.Itoa(v) + "_bg" }
func VariantText(v int) string { return "@tracks_button" + strconv.Itoa(v) + "_text" }

//go:embed prompts/*.txt
var prompts embed.FS

// Prompt is the line typed into an agent of engine to have it work
// through the open comments on the PR at url; false when that engine has
// none. It names the skill or rule that holds the steps, so a session
// started before they were installed finds them.
func Prompt(engine, url string) (string, bool) {
	b, err := prompts.ReadFile("prompts/pr-comments." + engine + ".txt")
	if err != nil {
		return "", false
	}
	return strings.ReplaceAll(strings.TrimSpace(string(b)), "{pr}", url), true
}

// Comments is how a PR's open comments stand: Open threads wait on the
// agent, and Sent says the prompt to answer them went out.
type Comments struct {
	Open int
	Sent bool
}

// Next is c once a poll counted open threads. A sent prompt stays sent
// while the count falls, as the agent answers them, and may be sent
// again once new ones come in or none are left.
func (c Comments) Next(open int) Comments {
	return Comments{Open: open, Sent: c.Sent && open > 0 && open <= c.Open}
}

// Button is one PR's button.
type Button struct {
	Index  int    // the PR's place among the track's
	Repo   string // owner/name
	Number int
	Comments
}

// maxRepo is how much of a repo's name a button shows.
const maxRepo = 14

// Label is what b reads. multiRepo says the track has PRs in more than
// one repo, which puts the repo's name, cut short, before the number.
func (b Button) Label(multiRepo bool) string {
	pr := "PR #" + strconv.Itoa(b.Number)
	if multiRepo {
		_, name, _ := strings.Cut(b.Repo, "/")
		if utf8.RuneCountInString(name) > maxRepo {
			name = string([]rune(name)[:maxRepo]) + "…"
		}
		pr = name + "#" + strconv.Itoa(b.Number)
	}
	if b.Sent {
		return pr + " · sent to the agent"
	}
	noun := "comments"
	if b.Open == 1 {
		noun = "comment"
	}
	return fmt.Sprintf("%s · solve %d %s", pr, b.Open, noun)
}

// style sets attr to the colour in the global option named option.
func style(attr, option string) string { return "#[" + attr + "=#{" + option + "}]" }

// Render is the tmux format of buttons, side by side. They take the
// colour variants in turn, from the first, so no two neighbours share
// one. A sent one is muted text, not a range: there's nothing to click
// until new comments come in, and it takes no variant.
func Render(buttons []Button, multiRepo bool) string {
	var parts []string
	coloured := 0
	for _, b := range buttons {
		label := " " + strings.ReplaceAll(b.Label(multiRepo), "#", "##") + " "
		if b.Sent {
			parts = append(parts, style("fg", Muted)+label+"#[default]")
			continue
		}
		v := coloured%Variants + 1
		coloured++
		parts = append(parts, "#[range=user|"+Range(b.Index)+"]"+style("bg", VariantBg(v))+style("fg", VariantText(v))+
			label+"#[norange default]")
	}
	return strings.Join(parts, " ")
}
