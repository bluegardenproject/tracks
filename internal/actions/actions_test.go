package actions

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestPromptsMatchGolden(t *testing.T) {
	for _, id := range IDs {
		for _, engine := range []string{"claude", "cursor"} {
			text, ok := Prompt(id, engine)
			if !ok {
				t.Fatalf("%s has no prompt for %s", id, engine)
			}
			golden.Check(t, "testdata/"+id+"."+engine+".golden", text)
		}
	}
	if _, ok := Prompt(PRComments, "codex"); ok {
		t.Error("an engine without a prompt got one")
	}
}

func TestIDsFitTmuxRanges(t *testing.T) {
	for _, id := range IDs {
		if len(id) > 15 {
			t.Errorf("%s is longer than tmux's 15 bytes for a range", id)
		}
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
