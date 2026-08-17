package dashboard

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/charmbracelet/lipgloss"
)

// At exactly the width that fits the minimums, nothing is scaled.
func TestLayoutUsesMinimumsWhenExactlyFitting(t *testing.T) {
	got := layoutFor(fixedColsWidth + branchMinWidth + slugMinWidth)
	if got.branch != branchMinWidth || got.slug != slugMinWidth {
		t.Errorf("layoutFor(exact fit) = %+v, want the minimums (%d/%d)", got, branchMinWidth, slugMinWidth)
	}
}

// Below that, the flexible columns give width back so the fixed
// right-hand columns stay on screen — SLUG first, since BRANCH is the
// column people actually scan.
func TestLayoutShrinksToKeepTheRightHandColumns(t *testing.T) {
	exact := fixedColsWidth + branchMinWidth + slugMinWidth

	got := layoutFor(exact - 6)
	if got.slug != slugMinWidth-6 {
		t.Errorf("slug = %d, want %d — SLUG should absorb the deficit first", got.slug, slugMinWidth-6)
	}
	if got.branch != branchMinWidth {
		t.Errorf("branch = %d, want it untouched while SLUG still has room", got.branch)
	}

	// Deep enough to exhaust SLUG and eat into BRANCH.
	deep := layoutFor(exact - (slugMinWidth - slugFloorWidth) - 5)
	if deep.slug != slugFloorWidth {
		t.Errorf("slug = %d, want the floor %d", deep.slug, slugFloorWidth)
	}
	if deep.branch != branchMinWidth-5 {
		t.Errorf("branch = %d, want %d", deep.branch, branchMinWidth-5)
	}
}

// However cramped, neither column goes below its floor — past that the
// frame clips, which is what it always did.
func TestLayoutNeverGoesBelowTheFloors(t *testing.T) {
	for _, w := range []int{0, 20, 40, 80, 100} {
		got := layoutFor(w)
		if got.branch < branchFloorWidth || got.slug < slugFloorWidth {
			t.Errorf("layoutFor(%d) = %+v, below the floors (%d/%d)", w, got, branchFloorWidth, slugFloorWidth)
		}
	}
}

// BRANCH is the better identifier of the two, so spare width goes there
// before SLUG sees any.
func TestLayoutGrowsBranchFirst(t *testing.T) {
	base := fixedColsWidth + branchMinWidth + slugMinWidth
	got := layoutFor(base + 4)
	if got.branch != branchMinWidth+4 {
		t.Errorf("branch = %d, want %d — spare width should reach BRANCH first", got.branch, branchMinWidth+4)
	}
	if got.slug != slugMinWidth {
		t.Errorf("slug = %d, want it untouched until BRANCH is full", got.slug)
	}
}

func TestLayoutCapsBothColumns(t *testing.T) {
	got := layoutFor(10_000)
	if got.branch != branchMaxWidth || got.slug != slugMaxWidth {
		t.Errorf("layoutFor(huge) = %+v, want the caps (%d/%d) so the right-hand columns stay reachable",
			got, branchMaxWidth, slugMaxWidth)
	}
}

// The regression this fixes: a long branch name was truncated even on a
// 300-column terminal, because the column was a fixed 28.
func TestWideTerminalShowsTheWholeBranch(t *testing.T) {
	const branch = "chore/deps-bump-shell-quote-and-dependencies"
	m := modelWith(state.Track{ID: "t1", Branch: branch, Status: state.StatusRunning})
	m.width = 250

	out := stripStyles(m.View())
	if !strings.Contains(out, branch) {
		t.Errorf("branch truncated despite a 250-column terminal:\n%s", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("something is still being truncated at 250 columns:\n%s", out)
	}
}

// The frame's MaxWidth clamp truncates every line before a test can see
// it, so asserting "no line exceeds the width" passes even when the
// layout is silently amputating columns. Assert the exact width instead:
// the row must be as wide as the layout intends, which is only true when
// nothing is being clipped.
func TestRowWidthMatchesTheLayout(t *testing.T) {
	maxTotal := fixedColsWidth + branchMaxWidth + slugMaxWidth
	for _, w := range []int{130, 138, 145, 151, 160, 185, 250, 319} {
		m := modelWith(state.Track{
			ID: "20260817-101530-aa11bb", Branch: strings.Repeat("b", 80),
			Slug: strings.Repeat("s", 60), Status: state.StatusInterrupted,
			Kind: state.KindWork, Model: "claude-sonnet-4-6",
			Usage: state.Usage{CostUSD: 3.45},
		})
		m.width, m.height = w, 40
		cols := layoutFor(w)
		want := min(fixedColsWidth+cols.branch+cols.slug, w)
		if want > maxTotal {
			want = maxTotal
		}
		var row string
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(stripStyles(l), "aa11bb") {
				row = l
				break
			}
		}
		if row == "" {
			t.Fatalf("terminal %d: no track row rendered", w)
		}
		if got := lipgloss.Width(row); got != want {
			t.Errorf("terminal %d: row is %d wide, want %d — columns are being clipped", w, got, want)
		}
		// COST is the rightmost column and the first casualty of a clip.
		if !strings.Contains(stripStyles(row), "$3.45") {
			t.Errorf("terminal %d: COST was clipped off the row: %q", w, stripStyles(row))
		}
	}
}

// Whatever the layout picks, the frame must not overflow the terminal —
// bubbletea garbles a frame wider than the window.
func TestRowsNeverExceedTerminalWidth(t *testing.T) {
	long := state.Track{
		ID: "20260817-101530-aa11bb", Branch: strings.Repeat("x", 80),
		Slug: strings.Repeat("y", 60), Status: state.StatusInterrupted,
		Kind: state.KindWork, Model: "claude-sonnet-4-6",
		Changes: state.Changes{Files: 3567, Insertions: 148204, Deletions: 175786},
		Usage:   state.Usage{CostUSD: 1234.56},
	}
	for _, w := range []int{60, 120, 152, 200, 319} {
		m := modelWith(long, long)
		m.width, m.height = w, 40
		for _, line := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("terminal %d: rendered line is %d wide:\n%q", w, got, line)
			}
		}
	}
}
