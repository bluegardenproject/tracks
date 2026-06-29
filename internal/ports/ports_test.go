package ports

import "testing"

func TestAllocateDeterministic(t *testing.T) {
	names := []string{"lld", "live-app"}
	a, err := Allocate("track-abc", names, nil)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	b, err := Allocate("track-abc", names, nil)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	for _, n := range names {
		if a[n] != b[n] {
			t.Errorf("non-deterministic for %q: %d vs %d", n, a[n], b[n])
		}
	}
}

func TestAllocateContiguousAndInRange(t *testing.T) {
	names := []string{"a", "b", "c"}
	got, err := Allocate("xyz", names, nil)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	base := got["a"]
	if base < RangeStart || base >= RangeEnd {
		t.Errorf("base %d out of range [%d,%d)", base, RangeStart, RangeEnd)
	}
	if base%BlockSize != 0 {
		t.Errorf("base %d not block-aligned", base)
	}
	if got["b"] != base+1 || got["c"] != base+2 {
		t.Errorf("ports not contiguous: %v", got)
	}
}

func TestAllocateAvoidsTaken(t *testing.T) {
	names := []string{"a", "b"}
	first, err := Allocate("track-1", names, nil)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	taken := map[int]bool{}
	for _, p := range first {
		taken[p] = true
	}
	// A different track whose natural block might overlap must avoid taken.
	second, err := Allocate("track-1", names, taken)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	for _, p := range second {
		if taken[p] {
			t.Errorf("allocated a taken port %d", p)
		}
	}
}

func TestAllocateEmpty(t *testing.T) {
	got, err := Allocate("t", nil, nil)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestAllocateTooMany(t *testing.T) {
	names := make([]string, BlockSize+1)
	for i := range names {
		names[i] = string(rune('a' + i))
	}
	if _, err := Allocate("t", names, nil); err == nil {
		t.Fatal("expected error for too many services")
	}
}
