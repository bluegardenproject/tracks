package tracks

import "testing"

func TestGroupLiving(t *testing.T) {
	ps := " 100 Ss\n 200 Z\n 200 Z+\n 300 S+\n"
	for pgid, want := range map[int]bool{100: true, 200: false, 300: true, 400: false} {
		if got := groupLiving(ps, pgid); got != want {
			t.Errorf("groupLiving(%d) = %v, want %v (zombies don't count)", pgid, got, want)
		}
	}
}
