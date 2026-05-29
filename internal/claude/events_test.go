package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func collectEvents(t *testing.T, ch <-chan Event, want int, timeout time.Duration) []Event {
	t.Helper()
	out := []Event{}
	deadline := time.After(timeout)
	for len(out) < want {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}

func TestPRMarkerInPlainText(t *testing.T) {
	out := make(chan Event, 8)
	emitFromLine("Some prose then TRACKS_PR_URL=https://github.com/x/y/pull/123 cool", out)
	close(out)
	got := collectEvents(t, out, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("got %d events", len(got))
	}
	pr, ok := got[0].(PRMarker)
	if !ok || pr.URL != "https://github.com/x/y/pull/123" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestPRMarkerNone(t *testing.T) {
	out := make(chan Event, 8)
	emitFromLine(`{"type":"assistant","message":{"content":[{"text":"all done. TRACKS_PR_URL=none"}]}}`, out)
	close(out)
	got := collectEvents(t, out, 2, time.Second)
	// We expect one PRMarker (URL="") and one AssistantText.
	sawNoPR := false
	sawText := false
	for _, e := range got {
		switch v := e.(type) {
		case PRMarker:
			if v.URL == "" {
				sawNoPR = true
			}
		case AssistantText:
			if v.Text != "" {
				sawText = true
			}
		}
	}
	if !sawNoPR || !sawText {
		t.Errorf("missing event(s): %+v", got)
	}
}

func TestAssistantText(t *testing.T) {
	out := make(chan Event, 8)
	emitFromLine(`{"type":"assistant_message","text":"hi there"}`, out)
	close(out)
	got := collectEvents(t, out, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	a, ok := got[0].(AssistantText)
	if !ok || a.Text != "hi there" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestToolUse(t *testing.T) {
	out := make(chan Event, 8)
	emitFromLine(`{"type":"tool_use","name":"Edit","input":{"path":"foo.go"}}`, out)
	close(out)
	got := collectEvents(t, out, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	tu, ok := got[0].(ToolUse)
	if !ok || tu.Name != "Edit" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestUnknownTypeIgnored(t *testing.T) {
	out := make(chan Event, 8)
	emitFromLine(`{"type":"completely_unknown","x":1}`, out)
	close(out)
	if got := collectEvents(t, out, 1, 50*time.Millisecond); len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestMalformedJSONIgnored(t *testing.T) {
	out := make(chan Event, 8)
	emitFromLine(`{this is not json`, out)
	close(out)
	if got := collectEvents(t, out, 1, 50*time.Millisecond); len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestTailLogFollow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"assistant","text":"first"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan Event, 32)
	go func() {
		_ = TailLog(ctx, path, out)
		close(out)
	}()

	// Wait for the first event.
	deadline := time.After(2 * time.Second)
	var first Event
loop:
	for {
		select {
		case ev := <-out:
			first = ev
			break loop
		case <-deadline:
			t.Fatal("never saw first event")
		}
	}
	if a, ok := first.(AssistantText); !ok || a.Text != "first" {
		t.Fatalf("got %+v", first)
	}

	// Append more — should be picked up by the tail loop.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"type":"assistant","text":"second"}` + "\n")
	_ = f.Close()

	deadline = time.After(2 * time.Second)
	for {
		select {
		case ev := <-out:
			if a, ok := ev.(AssistantText); ok && a.Text == "second" {
				cancel()
				return
			}
		case <-deadline:
			t.Fatal("never saw appended event")
		}
	}
}

func TestSpawnRejectsEmptyLogPath(t *testing.T) {
	_, err := Spawn(SpawnOptions{CLIBinary: "claude"})
	if err == nil {
		t.Fatal("expected error")
	}
}
