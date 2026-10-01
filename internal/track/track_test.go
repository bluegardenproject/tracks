package track

import "testing"

func TestWindowLabelOfDocument(t *testing.T) {
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
