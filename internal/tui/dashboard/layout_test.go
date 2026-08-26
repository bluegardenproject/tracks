package dashboard

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/charmbracelet/lipgloss"
)

// At exactly the width that fits the minimums, nothing is scaled.
func TestLayoutUsesMinimumsWhenExactlyFitting(t *testing.T) {
	got := layoutFor(fixedColsWidth+branchMinWidth+slugMinWidth+modelMinWidth, true)
	if got.branch != branchMinWidth || got.slug != slugMinWidth {
		t.Errorf("layoutFor(exact fit) = %+v, want the minimums (%d/%d)", got, branchMinWidth, slugMinWidth)
	}
}

// Below that, the flexible columns give width back so the fixed
// right-hand columns stay on screen — SLUG first, since BRANCH is the
// column people actually scan.
func TestLayoutShrinksToKeepTheRightHandColumns(t *testing.T) {
	exact := fixedColsWidth + branchMinWidth + slugMinWidth + modelMinWidth

	got := layoutFor(exact-6, true)
	if got.slug != slugMinWidth-6 {
		t.Errorf("slug = %d, want %d — SLUG should absorb the deficit first", got.slug, slugMinWidth-6)
	}
	if got.branch != branchMinWidth {
		t.Errorf("branch = %d, want it untouched while SLUG still has room", got.branch)
	}

	// Deep enough to exhaust SLUG and eat into BRANCH.
	deep := layoutFor(exact-(slugMinWidth-slugFloorWidth)-5, true)
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
		got := layoutFor(w, true)
		if got.branch < branchFloorWidth || got.slug < slugFloorWidth {
			t.Errorf("layoutFor(%d) = %+v, below the floors (%d/%d)", w, got, branchFloorWidth, slugFloorWidth)
		}
	}
}

// Spare width is served MODEL first — its extra buys a whole piece of
// information (the sub-agent's model), where BRANCH and SLUG degrade a
// character at a time — then BRANCH, then SLUG.
func TestLayoutGrowthPriority(t *testing.T) {
	base := fixedColsWidth + branchMinWidth + slugMinWidth + modelMinWidth

	// A little spare: MODEL takes it, nothing else moves.
	got := layoutFor(base+4, true)
	if got.model != modelMinWidth+4 {
		t.Errorf("model = %d, want %d — MODEL should be served first", got.model, modelMinWidth+4)
	}
	if got.branch != branchMinWidth || got.slug != slugMinWidth {
		t.Errorf("branch/slug = %d/%d, want them untouched until MODEL is full", got.branch, got.slug)
	}

	// Enough to fill MODEL and start on BRANCH.
	got = layoutFor(base+(modelMaxWidth-modelMinWidth)+5, true)
	if got.model != modelMaxWidth {
		t.Errorf("model = %d, want the cap %d", got.model, modelMaxWidth)
	}
	if got.branch != branchMinWidth+5 {
		t.Errorf("branch = %d, want %d", got.branch, branchMinWidth+5)
	}
	if got.slug != slugMinWidth {
		t.Errorf("slug = %d, want it untouched until BRANCH is full", got.slug)
	}
}

func TestLayoutCapsEveryColumn(t *testing.T) {
	got := layoutFor(10_000, true)
	if got.branch != branchMaxWidth || got.slug != slugMaxWidth || got.model != modelMaxWidth {
		t.Errorf("layoutFor(huge) = %+v, want the caps (%d/%d/%d) so the right-hand columns stay reachable",
			got, branchMaxWidth, slugMaxWidth, modelMaxWidth)
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
	maxTotal := fixedColsWidth + branchMaxWidth + slugMaxWidth + modelMaxWidth
	// Both column-sizing regimes: a table with a sub-agent to show (MODEL
	// widens) and one without (MODEL stays at its minimum).
	for _, withSub := range []bool{true, false} {
		sub := ""
		if withSub {
			sub = "claude-haiku-4-5"
		}
		for _, w := range []int{130, 138, 145, 151, 160, 185, 250, 319} {
			m := modelWith(state.Track{
				ID: "20260817-101530-aa11bb", Branch: strings.Repeat("b", 80),
				Slug: strings.Repeat("s", 60), Status: state.StatusInterrupted,
				Kind: state.KindWork, Model: "claude-sonnet-4-6", SubagentModel: sub,
				Usage: state.Usage{CostUSD: 3.45},
			})
			m.width, m.height = w, 40
			cols := layoutFor(w, withSub)
			want := min(fixedColsWidth+cols.branch+cols.slug+cols.model, w)
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
				t.Fatalf("subagent=%v terminal %d: no track row rendered", withSub, w)
			}
			if got := lipgloss.Width(row); got != want {
				t.Errorf("subagent=%v terminal %d: row is %d wide, want %d — columns are being clipped",
					withSub, w, got, want)
			}
			// COST is the rightmost column and the first casualty of a clip.
			if !strings.Contains(stripStyles(row), "$3.45") {
				t.Errorf("subagent=%v terminal %d: COST was clipped off the row: %q", withSub, w, stripStyles(row))
			}
		}
	}
}

// A table with no sub-agent anywhere must not pay for the wider column.
func TestModelColumnStaysNarrowWithoutSubagents(t *testing.T) {
	if got := layoutFor(10_000, false); got.model != modelMinWidth {
		t.Errorf("model = %d on a huge terminal with no sub-agents, want the minimum %d", got.model, modelMinWidth)
	}
	if got := layoutFor(10_000, true); got.model != modelMaxWidth {
		t.Errorf("model = %d with a sub-agent to show, want the cap %d", got.model, modelMaxWidth)
	}
}

// MODEL is never shrunk: its minimum is exactly one model id, and
// truncating an id is the failure the column exists to avoid.
func TestLayoutNeverShrinksModel(t *testing.T) {
	for _, w := range []int{0, 20, 60, 100, 130, 151} {
		for _, withSub := range []bool{true, false} {
			if got := layoutFor(w, withSub); got.model != modelMinWidth {
				t.Errorf("layoutFor(%d, %v).model = %d, want it pinned at %d",
					w, withSub, got.model, modelMinWidth)
			}
		}
	}
}

func TestAnyTrackHasSubagent(t *testing.T) {
	if anyTrackHasSubagent(nil) {
		t.Error("no tracks should mean no sub-agent")
	}
	if anyTrackHasSubagent([]state.Track{{Model: "claude-opus-5"}}) {
		t.Error("a track with no sub-agent model should not widen the column")
	}
	if !anyTrackHasSubagent([]state.Track{{Model: "claude-opus-5"}, {SubagentModel: "claude-haiku-4-5"}}) {
		t.Error("one track with a sub-agent should widen the column")
	}
}

// Whatever the layout picks, no line of the frame may overflow the
// terminal — bubbletea garbles a frame wider than the window. Broader
// than TestRowWidthMatchesTheLayout: that one checks a single track row
// at usable widths, this one checks every line (header, footer, detail
// panel, rows) right down to 60 columns. The distinction matters now
// that column widths respond to track content, not just terminal size.
func TestRowsNeverExceedTerminalWidth(t *testing.T) {
	long := state.Track{
		ID: "20260817-101530-aa11bb", Branch: strings.Repeat("x", 80),
		Slug: strings.Repeat("y", 60), Status: state.StatusInterrupted,
		Kind: state.KindWork, Model: "claude-sonnet-4-6",
		Changes: state.Changes{Files: 3567, Insertions: 148204, Deletions: 175786},
		Usage:   state.Usage{CostUSD: 1234.56},
	}
	withSub := long
	withSub.SubagentModel = "claude-haiku-4-5"

	for _, tracks := range [][]state.Track{{long, long}, {long, withSub}} {
		for _, w := range []int{60, 120, 152, 200, 319} {
			m := modelWith(tracks...)
			m.width, m.height = w, 40
			for _, line := range strings.Split(m.View(), "\n") {
				if got := lipgloss.Width(line); got > w {
					t.Errorf("terminal %d: rendered line is %d wide:\n%q", w, got, line)
				}
			}
		}
	}
}
