package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFollowStream(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "report.out")
	in := `{"type":"tool_call","subtype":"started","tool_call":{"readToolCall":{"args":{"path":"a.go"}}}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Report."}]}}
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

	failed := filepath.Join(dir, "failed.out")
	if err := followStream(strings.NewReader(`{"type":"result","is_error":true,"result":"no"}`), &shown, failed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(failed); !os.IsNotExist(err) {
		t.Error("a failed turn's text was kept as a report")
	}
}
