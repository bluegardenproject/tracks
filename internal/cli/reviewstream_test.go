package cli

import (
	"github.com/bluegardenproject/tracks/internal/agents/cursor"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFollowStream(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "report.out")
	in := `{"type":"tool_call","subtype":"started","tool_call":{"readToolCall":{"args":{"path":"a.go"}}}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Report.\nREVIEW OUTCOME: pass"}]}}
{"type":"result","subtype":"success","result":"Report.\nREVIEW OUTCOME: pass"}
`
	var shown strings.Builder
	if err := followStream(strings.NewReader(in), &shown, out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "Report.\nREVIEW OUTCOME: pass" {
		t.Errorf("kept %q, want the result", b)
	}
	if !strings.Contains(shown.String(), "→ read a.go") || !strings.Contains(shown.String(), "Report.") {
		t.Errorf("shown %q", shown.String())
	}

	defer func(n int) { maxStreamLine = n }(maxStreamLine)
	maxStreamLine = 4096
	for name, in := range map[string]string{
		"failed":     `{"type":"result","is_error":true,"result":"no"}`,
		"unfinished": `{"type":"assistant","message":{"content":[{"type":"text","text":"Half"}]}}`,
		"too long":   strings.Repeat("x", maxStreamLine+10),
	} {
		// An earlier turn's text is there: it mustn't pass for this one's.
		if err := os.WriteFile(out, []byte("the last answer"), 0o600); err != nil {
			t.Fatal(err)
		}
		var shown strings.Builder
		if err := followStream(strings.NewReader(in), &shown, out); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Errorf("%s: a turn without a result left text to hand over", name)
		}
		if shown.Len() > 200 {
			t.Errorf("%s: %d bytes shown", name, shown.Len())
		}
	}
}

func TestTurnText(t *testing.T) {
	report := "Report.\nREVIEW OUTCOME: pass"
	for _, c := range []struct {
		s    cursor.Stream
		want string
	}{
		{cursor.Stream{Segments: []string{"I'll look.", report}}, report},
		{cursor.Stream{Segments: []string{"I'll look.", report, "Done."}}, report},
		{cursor.Stream{Segments: []string{"Looking.", "An answer."}}, "An answer."},
		{cursor.Stream{Result: "only the result"}, "only the result"},
	} {
		if got := turnText(c.s); got != c.want {
			t.Errorf("segments %q: kept %q, want %q", c.s.Segments, got, c.want)
		}
	}
}
