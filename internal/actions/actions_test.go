package actions

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestPromptsMatchGolden(t *testing.T) {
	const url = "https://github.com/acme/api/pull/23"
	for _, engine := range []string{"claude", "cursor"} {
		text, ok := Prompt(engine, url)
		if !ok {
			t.Fatalf("no prompt for %s", engine)
		}
		if !strings.Contains(text, url) {
			t.Errorf("the %s prompt doesn't name the PR: %q", engine, text)
		}
		golden.Check(t, "testdata/pr-comments."+engine+".golden", text)
	}
	if _, ok := Prompt("codex", url); ok {
		t.Error("an engine without a prompt got one")
	}
}

func TestRange(t *testing.T) {
	if i, ok := ParseRange(Range(12)); !ok || i != 12 {
		t.Errorf("ParseRange(Range(12)) = %d, %v", i, ok)
	}
	for _, name := range []string{"", "window", "pr:", "pr:-1", "pr:x"} {
		if _, ok := ParseRange(name); ok {
			t.Errorf("ParseRange(%q) took it for a button", name)
		}
	}
	if len(Range(9999)) > 15 {
		t.Error("a range name is longer than tmux's 15 bytes")
	}
}

func TestLabel(t *testing.T) {
	for _, tc := range []struct {
		b     Button
		multi bool
		want  string
	}{
		{Button{Repo: "acme/tracks", Number: 23, Comments: Comments{Open: 4}}, false, "PR #23 · solve 4 comments"},
		{Button{Repo: "acme/tracks", Number: 23, Comments: Comments{Open: 1}}, false, "PR #23 · solve 1 comment"},
		{Button{Repo: "acme/tracks", Number: 12, Comments: Comments{Open: 1}}, true, "tracks#12 · solve 1 comment"},
		{Button{Repo: "acme/predictions-live-app", Number: 23, Comments: Comments{Open: 4}}, true, "predictions-li…#23 · solve 4 comments"},
		{Button{Repo: "acme/tracks", Number: 23, Comments: Comments{Open: 4, Sent: true}}, false, "PR #23 · sent to the agent"},
	} {
		if got := tc.b.Label(tc.multi); got != tc.want {
			t.Errorf("Label = %q, want %q", got, tc.want)
		}
	}
}

func TestRender(t *testing.T) {
	got := Render([]Button{
		{Index: 0, Repo: "a/b", Number: 3, Comments: Comments{Open: 2}},
		{Index: 2, Repo: "a/b", Number: 7, Comments: Comments{Open: 1, Sent: true}},
	}, false)
	if !strings.Contains(got, "#[range=user|pr:0]") || strings.Contains(got, "range=user|pr:2") {
		t.Errorf("only the unsent button is a range: %s", got)
	}
	if !strings.Contains(got, "PR ##3 · solve 2 comments") || !strings.Contains(got, "PR ##7 · sent to the agent") {
		t.Errorf("the labels aren't escaped for tmux: %s", got)
	}
	if Render(nil, false) != "" {
		t.Error("no buttons render something")
	}
}

func TestCommentsNext(t *testing.T) {
	for _, tc := range []struct {
		name string
		was  Comments
		open int
		want Comments
	}{
		{"new comments", Comments{}, 3, Comments{Open: 3}},
		{"answering", Comments{Open: 3, Sent: true}, 2, Comments{Open: 2, Sent: true}},
		{"unchanged", Comments{Open: 3, Sent: true}, 3, Comments{Open: 3, Sent: true}},
		{"more came in", Comments{Open: 2, Sent: true}, 4, Comments{Open: 4}},
		{"all answered", Comments{Open: 2, Sent: true}, 0, Comments{}},
	} {
		if got := tc.was.Next(tc.open); got != tc.want {
			t.Errorf("%s: %+v.Next(%d) = %+v, want %+v", tc.name, tc.was, tc.open, got, tc.want)
		}
	}
}

func TestRenderTakesVariantsInTurn(t *testing.T) {
	var buttons []Button
	for i := range 5 {
		buttons = append(buttons, Button{Index: i, Repo: "a/b", Number: i + 1, Comments: Comments{Open: 1}})
	}
	got := Render(buttons, false)
	for v := 1; v <= Variants; v++ {
		if !strings.Contains(got, VariantBg(v)) {
			t.Errorf("variant %d unused: %s", v, got)
		}
	}
	if strings.Count(got, "#{"+VariantBg(1)+"}") != 2 {
		t.Errorf("the fifth PR should start over at variant 1: %s", got)
	}
}
