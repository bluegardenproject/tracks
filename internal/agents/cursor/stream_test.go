package cursor

import (
	"os"
	"strings"
	"testing"
)

// testdata/stream.jsonl is a real turn of Cursor's agent CLI
// (2026.11.19), its paths made /w.
func TestStream(t *testing.T) {
	b, err := os.ReadFile("testdata/stream.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var s Stream
	var shown strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		shown.WriteString(s.Feed(line))
	}
	const answer = "Add is wrong: it returns `a - b` (subtraction), so it should be `return a + b`."
	if !s.Done || s.Failed || s.Result != answer {
		t.Errorf("done %v, failed %v, result %q", s.Done, s.Failed, s.Result)
	}
	if got := shown.String(); got != "→ read x.go\n"+answer+"\n" {
		t.Errorf("shown:\n%s", got)
	}
}

func TestStreamFails(t *testing.T) {
	var s Stream
	if got := s.Feed("Warning: something the CLI prints"); got != "Warning: something the CLI prints\n" {
		t.Errorf("a line that isn't an event: %q", got)
	}
	s.Feed(`{"type":"result","subtype":"error","is_error":true,"result":"Authentication required"}`)
	if !s.Done || !s.Failed {
		t.Error("a failed turn wasn't seen")
	}
	long := `{"type":"tool_call","subtype":"started","tool_call":{"grepToolCall":{"args":{"pattern":"` + strings.Repeat("x", 300) + `"}}}}`
	if got := s.Feed(long); !strings.HasPrefix(got, "→ grep xxx") || len([]rune(got)) > 120 {
		t.Errorf("a long tool call: %q", got)
	}
}
