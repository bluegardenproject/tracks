package tracks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

func turn(id string) string {
	return `{"type":"assistant","requestId":"` + id + `","message":{"model":"claude-opus-4-8","usage":{"input_tokens":100000,"output_tokens":10000}}}` + "\n"
}

func TestCosts(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	project := filepath.Join(claude, "projects", "-src-api")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(session, line string) {
		t.Helper()
		file, err := os.OpenFile(filepath.Join(project, session+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.WriteString(line); err != nil {
			t.Fatal(err)
		}
	}
	for _, tr := range []track.Track{
		{ID: "a", Kind: track.Ask, Name: "a", Engine: "claude", Session: "s-a"},
		{ID: "b", Kind: track.Ask, Name: "b", Engine: "cursor", Session: "s-b"},
		{ID: "c", Kind: track.Ask, Name: "c", Engine: "claude", Session: "s-c"},
	} {
		if err := f.store.AddTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	write("s-a", turn("1"))
	write("s-b", turn("1"))
	cost := func(id string) float64 {
		t.Helper()
		if err := f.svc.Costs(ctx); err != nil {
			t.Fatal(err)
		}
		tr, err := f.store.Track(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return tr.Cost
	}

	first := cost("a")
	if first <= 0 {
		t.Fatalf("a costs %v after a turn", first)
	}
	if b := cost("b"); b != 0 {
		t.Errorf("a Cursor track costs %v", b)
	}
	if c := cost("c"); c != 0 {
		t.Errorf("a track without a transcript costs %v", c)
	}
	write("s-a", turn("2"))
	second := cost("a")
	if second <= first {
		t.Errorf("a costs %v after a second turn, %v before", second, first)
	}

	if err := f.store.SetState(ctx, "a", track.State{ClosedAt: f.svc.now()}); err != nil {
		t.Fatal(err)
	}
	write("s-a", turn("3"))
	last := cost("a")
	if last <= second {
		t.Errorf("the turn before the window closed isn't counted: %v, %v before", last, second)
	}
	write("s-a", turn("4"))
	if got := cost("a"); got != last {
		t.Errorf("an ended track is read again: %v, %v before", got, last)
	}
}
