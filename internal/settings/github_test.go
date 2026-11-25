package settings

import (
	"path/filepath"
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
