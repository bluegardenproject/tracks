package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubPolicy(t *testing.T) {
	var none GitHub
	for event, want := range map[string]Policy{EventConflicts: PolicyAuto, EventCIFailed: PolicyAuto, EventReviewThreads: PolicyMain, "merged": PolicyMain} {
		if got := none.Policy(event); got != want {
			t.Errorf("default for %s = %q, want %q", event, got, want)
		}
	}
	set := GitHub{Conflicts: PolicyAsk, CIFailed: "sometimes", ReviewThreads: PolicyAuto}
	for event, want := range map[string]Policy{EventConflicts: PolicyAsk, EventCIFailed: PolicyAuto, EventReviewThreads: PolicyAuto} {
		if got := set.Policy(event); got != want {
			t.Errorf("%s set = %q, want %q: an unknown value is the default", event, got, want)
		}
	}

	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := Save(path, Settings{GitHub: GitHub{ReviewThreads: PolicyAsk}}); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.GitHub.Policy(EventReviewThreads) != PolicyAsk || s.GitHub.Policy(EventConflicts) != PolicyAuto {
		t.Errorf("loaded %+v, %v", s.GitHub, err)
	}
}

func TestGitHubAsksBeforeHumanReplies(t *testing.T) {
	on, off := true, false
	for _, c := range []struct {
		set  *bool
		want bool
	}{{nil, true}, {&on, true}, {&off, false}} {
		if got := (GitHub{AskBeforeHumanReplies: c.set}).AsksBeforeHumanReplies(); got != c.want {
			t.Errorf("set %v: asks %v, want %v", c.set, got, c.want)
		}
	}

	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := Save(path, Settings{GitHub: GitHub{AskBeforeHumanReplies: &off}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "ask_before_human_replies: false") {
		t.Errorf("settings.yaml reads %q, %v", b, err)
	}
	s, err := Load(path)
	if err != nil || s.GitHub.AsksBeforeHumanReplies() {
		t.Errorf("loaded %+v, %v: want off", s.GitHub, err)
	}
}
