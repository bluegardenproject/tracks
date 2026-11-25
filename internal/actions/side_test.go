package actions

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestSidePromptsMatchGolden(t *testing.T) {
	golden.Check(t, "testdata/side-conflicts.golden", SideConflicts("acme/api#231", "main"))
	golden.Check(t, "testdata/side-ci-failed.golden", SideCIFailed("acme/api#231", "0123abc", "lint, test"))
	for _, engine := range []string{"claude", "cursor"} {
		text, ok := SideThreads(engine, "acme/api#231", "https://github.com/acme/api/pull/231", 3)
		if !ok {
			t.Fatalf("no threads prompt for %s", engine)
		}
		golden.Check(t, "testdata/side-threads."+engine+".golden", text)
	}
	if one, _ := SideThreads("claude", "acme/api#231", "https://github.com/acme/api/pull/231", 1); !strings.HasPrefix(one, "1 review thread on your PR acme/api#231 waits for an answer.") {
		t.Errorf("one thread: %q", one)
	}
	if _, ok := SideThreads("codex", "acme/api#231", "https://github.com/acme/api/pull/231", 3); ok {
		t.Error("an engine without a prompt got one")
	}
	golden.Check(t, "testdata/ask-main.golden", AskMain("side-000123", "relay-cli", "3 review threads on PR acme/api#231 wait for an answer."))
}
