package cursor

import (
	"slices"
	"testing"
)

// The reviewer must be spawned with no permission flags: overriding a
// user's approvalMode is not ours to do, and a process that cannot
// execute cannot spawn another reviewer.
func TestReviewCommandPassesNoPermissionFlags(t *testing.T) {
	c := ReviewCommand("/bin/agent", "/w", "")
	for _, forbidden := range []string{"--force", "--yolo", "-f", "--trust", "--sandbox"} {
		if slices.Contains(c.Args, forbidden) {
			t.Errorf("reviewer args %v carry %s, overriding the user's permission config", c.Args, forbidden)
		}
	}
	if !slices.Equal(c.Args, []string{"-p", "--workspace", "/w"}) || c.Binary != "/bin/agent" {
		t.Errorf("command %+v, want /bin/agent -p --workspace /w", c)
	}
}

func TestReviewCommandRunsOnTheModelGiven(t *testing.T) {
	if c := ReviewCommand("", "/w", "gpt-5"); !slices.Equal(c.Args[len(c.Args)-2:], []string{"--model", "gpt-5"}) || c.Binary != "agent" {
		t.Errorf("command %+v, want agent with --model gpt-5", c)
	}
}
