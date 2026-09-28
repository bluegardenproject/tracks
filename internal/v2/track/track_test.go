package track

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

// v1Label is the label v1's handleNew gives a track.
func v1Label(name, document, prompt string) string {
	slug := strings.TrimSpace(name)
	if slug == "" && document != "" {
		base := filepath.Base(document)
		slug = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return state.Track{Slug: slug, TaskPrompt: prompt}.WindowLabel()
}

func TestWindowLabelMatchesV1(t *testing.T) {
	long := "Investigate the rate spike on swap quotes since Monday"
	for _, tt := range []struct{ name, document, prompt string }{
		{name: "Rate bug", prompt: "anything"},
		{name: "  ", prompt: long},
		{name: "fix: a.b c", prompt: long},
		{document: "/Users/u/Downloads/Q3 Architecture.deck.pdf", prompt: long},
		{name: "own name", document: "/tmp/x.md"},
		{prompt: "日本語だけ"},
	} {
		if got, want := WindowLabel(tt.name, tt.document, tt.prompt), v1Label(tt.name, tt.document, tt.prompt); got != want {
			t.Errorf("WindowLabel(%q, %q, %q) = %q, v1 gives %q", tt.name, tt.document, tt.prompt, got, want)
		}
	}
	if got := WindowLabel("", "/tmp/Q3 Architecture.deck.pdf", "p"); got != "q3-architecture-deck" {
		t.Errorf("a document's name gives %q", got)
	}
}

func TestWindowName(t *testing.T) {
	const id = "20260928-151500-a1b2c3"
	used := map[string]bool{"rate-bug": true, "rate-bug-2": true}
	taken := func(n string) bool { return used[n] }
	if got := WindowName("fresh", id, taken); got != "fresh" {
		t.Errorf("a free name gives %q", got)
	}
	if got := WindowName("rate-bug", id, taken); got != "rate-bug-3" {
		t.Errorf("a taken name gives %q, want rate-bug-3", got)
	}
	if got := WindowName("", id, taken); got != "t-a1b2c3" {
		t.Errorf("no label gives %q", got)
	}
	if got := WindowName("x", id, func(string) bool { return true }); got != "x-a1b2c3" {
		t.Errorf("every suffix taken gives %q", got)
	}
	if got := Branch(id); got != "tracks/a1b2c3" {
		t.Errorf("Branch = %q", got)
	}
}

func TestCandorMatchesV1(t *testing.T) {
	if MinCandor != state.MinCandor || MaxCandor != state.MaxCandor || DefaultCandor != state.DefaultCandor {
		t.Fatalf("candor %d–%d (default %d), v1 has %d–%d (%d)",
			MinCandor, MaxCandor, DefaultCandor, state.MinCandor, state.MaxCandor, state.DefaultCandor)
	}
	for level := -1; level <= MaxCandor+1; level++ {
		if got, want := CandorLabel(level), state.CandorLabel(level); got != want {
			t.Errorf("candor %d is %q, v1 has %q", level, got, want)
		}
		if got, want := CandorLevel(level), (state.Track{Review: &state.ReviewSpec{Candor: level}}).CandorLevel(); got != want {
			t.Errorf("CandorLevel(%d) = %d, v1 gives %d", level, got, want)
		}
	}
}
