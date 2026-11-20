package cursor

import (
	"encoding/json"
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

// testdata/stream-segments.jsonl is a real turn with text before, between
// and after two tool calls: Cursor repeats each segment once.
func TestStreamSegments(t *testing.T) {
	b, err := os.ReadFile("testdata/stream-segments.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var s Stream
	var shown strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		shown.WriteString(s.Feed(line))
	}
	const verdict = "- **`x.go` has a bug:** `Add` returns `a - b` when it should return `a + b`.\n" +
		"- **`y.go` is correct:** `Mul` returns `a * b` as it should."
	want := "I'll start by checking `x.go`.\n→ read x.go\nNext I'll check `y.go`.\n→ read y.go\n" + verdict + "\n"
	if shown.String() != want {
		t.Errorf("shown:\n%s\nwant:\n%s", shown.String(), want)
	}
	if s.Result != verdict {
		t.Errorf("result %q; want the last segment", s.Result)
	}
}

func TestStreamPieces(t *testing.T) {
	piece := func(text string) string {
		quoted, _ := json.Marshal(text)
		return `{"type":"assistant","message":{"content":[{"type":"text","text":` + string(quoted) + `}]}}`
	}
	var s Stream
	var shown strings.Builder
	for _, p := range []string{"*", "*", "bold*", "*"} {
		shown.WriteString(s.Feed(piece(p)))
	}
	if shown.String() != "**bold**" {
		t.Errorf("shown %q; a piece equal to the text so far, followed by more, was dropped", shown.String())
	}
	if got := (&Stream{}).Feed(piece("red \x1b[31mtext\x1b]0;title\x07 \u202e")); got != "red [31mtext]0;title " {
		t.Errorf("controls reached the pane: %q", got)
	}
	root := Stream{cwd: "/"}
	if got := root.relative("read /a/b.go"); got != "read /a/b.go" {
		t.Errorf("a turn in / showed %q", got)
	}
	if got := (&Stream{}).Feed(strings.Repeat("x", maxOther+1)); !strings.Contains(got, "too long") {
		t.Errorf("a long line that isn't an event was shown: %d bytes", len(got))
	}
}
