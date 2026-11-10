package tracks

import (
	"slices"
	"testing"
)

func TestGroupLiving(t *testing.T) {
	ps := " 100 Ss\n 200 Z\n 200 Z+\n 300 S+\n"
	for pgid, want := range map[int]bool{100: true, 200: false, 300: true, 400: false} {
		if got := groupLiving(ps, pgid); got != want {
			t.Errorf("groupLiving(%d) = %v, want %v (zombies don't count)", pgid, got, want)
		}
	}
}

func TestTTYGroups(t *testing.T) {
	got := ttyGroups("  501\n  501\n  733\n\n 0\n")
	if !slices.Equal(got, []int{501, 733}) {
		t.Errorf("ttyGroups = %v, want [501 733]", got)
	}
}

func TestGroupsToEnd(t *testing.T) {
	got := groupsToEnd([]int{100, 200, 300, 400}, 100, 300)
	if !slices.Equal(got, []int{200, 400}) {
		t.Errorf("groupsToEnd = %v, want [200 400]: the pane's shell and the caller are spared", got)
	}
}
