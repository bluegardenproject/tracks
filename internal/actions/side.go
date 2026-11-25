package actions

import (
	"fmt"
	"strings"
)

// The prompts Tracks types into a side track's agent as its main
// track's GitHub policies say (see docs/design/trains.md, "Side tracks
// and GitHub's events"), and into the main track's to ask the user.
// Each is Tracks' own words, with the user's authority, as a button's.

// SideConflicts has a side track rebase its PR pr (owner/name#n) onto
// base and solve the conflicts.
func SideConflicts(pr, base string) string {
	return fill("side-conflicts", "{pr}", pr, "{base}", base)
}

// SideCIFailed has a side track look into the checks that failed on
// its PR pr at commit head, and fix them or say why they aren't its.
func SideCIFailed(pr, head, checks string) string {
	return fill("side-ci-failed", "{pr}", pr, "{head}", head, "{checks}", checks)
}

// SideThreads has a side track on engine work through the n review
// threads on its PR pr, at url, as the PR comments button does, ask as
// Prompt's; false when engine has no prompt for that.
func SideThreads(engine, pr, url string, n int, ask bool) (string, bool) {
	work, ok := Prompt(engine, url, ask)
	if !ok {
		return "", false
	}
	threads, wait := fmt.Sprintf("%d review threads", n), "wait"
	if n == 1 {
		threads, wait = "1 review thread", "waits"
	}
	return fill("side-threads", "{threads}", threads, "{pr}", pr, "{wait}", wait, "{work}", work), true
}

// AskMain has a main track ask the user what its side track, handle
// side and named name, does about fact.
func AskMain(side, name, fact string) string {
	return fill("ask-main", "{side}", side, "{name}", name, "{fact}", fact)
}

func fill(prompt string, pairs ...string) string {
	b, err := prompts.ReadFile("prompts/" + prompt + ".txt")
	if err != nil {
		panic(err) // embedded: there
	}
	return strings.NewReplacer(pairs...).Replace(strings.TrimSpace(string(b)))
}

// SideRestack has a side track stacked on the PR pr of the track named
// name, merged into base at commit head, rebase its own commits onto
// base.
func SideRestack(pr, name, base, head string) string {
	return fill("side-restack", "{pr}", pr, "{name}", name, "{base}", base, "{head}", head)
}
