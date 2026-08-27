package dashboard

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/usage"
)

// stripStyles removes ANSI escapes so a rendered row can be asserted on
// as plain text.
func stripStyles(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc && (r == 'm' || r == 'K'):
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// modelWith builds a dashboard model around specific tracks, wide
// enough that no column is clipped by the frame's width clamp.
func modelWith(tracks ...state.Track) *model {
	return &model{
		styles: defaultStyles(),
		tracks: tracks,
		width:  200,
		height: 40,
	}
}

func TestHeaderCarriesTheModelColumn(t *testing.T) {
	m := modelWith(state.Track{ID: "t1", Status: state.StatusRunning})
	out := stripStyles(m.View())

	hdr := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "STATUS") && strings.Contains(l, "COST") {
			hdr = l
			break
		}
	}
	if hdr == "" {
		t.Fatal("no header row rendered")
	}
	if !strings.Contains(hdr, "MODEL") {
		t.Fatalf("header has no MODEL column: %q", hdr)
	}
	// MODEL sits between SVC and COST, as asked.
	svc, mdl, cost := strings.Index(hdr, "SVC"), strings.Index(hdr, "MODEL"), strings.Index(hdr, "COST")
	if !(svc < mdl && mdl < cost) {
		t.Errorf("column order wrong — SVC@%d MODEL@%d COST@%d in %q", svc, mdl, cost, hdr)
	}
}

func TestRowRendersTheShortenedModel(t *testing.T) {
	m := modelWith(state.Track{
		ID: "t1", Status: state.StatusRunning, ObservedModel: "claude-haiku-4-5-20251001",
	})
	out := stripStyles(m.View())
	if !strings.Contains(out, "haiku-4-5") {
		t.Errorf("row does not show the shortened model:\n%s", out)
	}
	if strings.Contains(out, "claude-haiku") {
		t.Errorf("row shows the raw model id instead of the shortened one:\n%s", out)
	}
}

// The highlighted (cursor) row takes a separate code path, so it needs
// its own assertion — this is where a forgotten cell shows up.
func TestHighlightedRowRendersTheModel(t *testing.T) {
	m := modelWith(
		state.Track{ID: "t1", Status: state.StatusRunning, ObservedModel: "claude-opus-5"},
		state.Track{ID: "t2", Status: state.StatusRunning, ObservedModel: "claude-sonnet-5"},
	)
	m.cursor = 0 // t1 is highlighted, t2 is plain
	out := stripStyles(m.View())

	if !strings.Contains(out, "opus-5") {
		t.Errorf("highlighted row lost its model cell:\n%s", out)
	}
	if !strings.Contains(out, "sonnet-5") {
		t.Errorf("plain row lost its model cell:\n%s", out)
	}
}

func TestModelIsDashedBeforeTheFirstTurn(t *testing.T) {
	m := modelWith(state.Track{ID: "t1", Status: state.StatusPending})
	if got := stripStyles(m.renderModel(m.tracks[0], modelMinWidth)); strings.TrimSpace(got) != "—" {
		t.Errorf("renderModel with no model = %q, want the em-dash placeholder", got)
	}
}

// knownModelIDs are the model ids actually in circulation. The MODEL
// column exists to tell them apart, so truncating one defeats the
// column: "sonnet-4…" is indistinguishable from sonnet-4-5. Add new ids
// here when they appear — a failure means modelMinWidth needs raising,
// not that the id should be shortened further.
var knownModelIDs = []string{
	"claude-opus-5",
	"claude-opus-4-8",
	"claude-sonnet-5",
	"claude-sonnet-4-6",
	"claude-sonnet-4-5",
	"claude-haiku-4-5-20251001",
	"claude-fable-5",
}

func TestKnownModelsFitTheColumn(t *testing.T) {
	for _, id := range knownModelIDs {
		short := usage.ShortModel(id)
		// The minimum is what a narrow terminal gives the column, and a
		// track's own model must be legible even there.
		if len(short) > modelMinWidth {
			t.Errorf("ShortModel(%q) = %q (%d chars) exceeds modelMinWidth=%d — it would render truncated",
				id, short, len(short), modelMinWidth)
		}
	}
}

// Two ids that differ only in their point release must stay distinct
// after shortening and truncation — the exact failure a too-narrow
// column produces.
func TestSimilarModelsStayDistinguishable(t *testing.T) {
	m := modelWith(state.Track{ID: "t"})
	a := stripStyles(m.renderModel(state.Track{ObservedModel: "claude-sonnet-4-5"}, modelMaxWidth))
	b := stripStyles(m.renderModel(state.Track{ObservedModel: "claude-sonnet-4-6"}, modelMaxWidth))
	if strings.TrimSpace(a) == strings.TrimSpace(b) {
		t.Errorf("sonnet-4-5 and sonnet-4-6 both render as %q", strings.TrimSpace(a))
	}
}

func TestTrackModelPairsMainAndSubagent(t *testing.T) {
	cases := []struct {
		name      string
		main, sub string
		width     int
		want      string
	}{
		{"main only", "claude-opus-5", "", modelMaxWidth, "opus-5"},
		{"main and subagent", "claude-opus-5", "claude-opus-4-8", modelMaxWidth, "opus-5 (opus-4-8)"},
		// Shown even when identical: the parentheses mean "a sub-agent ran",
		// which is information whether or not the models differ.
		{"same model both", "claude-opus-5", "claude-opus-5", modelMaxWidth, "opus-5 (opus-5)"},
		{"neither", "", "", modelMaxWidth, ""},
		{"subagent before any main turn", "", "claude-haiku-4-5", modelMaxWidth, "(haiku-4-5)"},
		// The solo parenthetical obeys the same width rule as the pair:
		// "(sonnet-4-…" names a model that doesn't exist.
		{"solo too wide for the column", "", "claude-sonnet-4-6", modelMinWidth, ""},
		// Too narrow for both: drop the sub-agent's whole rather than cut it,
		// since "opus-5 (opus-4…" reads as a model that doesn't exist.
		{"narrow drops the parenthetical", "claude-opus-5", "claude-opus-4-8", modelMinWidth, "opus-5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trackModel(state.Track{ObservedModel: tc.main, ObservedSubagentModel: tc.sub}, tc.width)
			if got != tc.want {
				t.Errorf("trackModel = %q, want %q", got, tc.want)
			}
		})
	}
}

// The widest pair the column can be asked to show must fit its cap.
//
// Measured from the raw ids, NOT from trackModel's output: trackModel
// drops the parenthetical when it wouldn't fit, so asking it for the
// widest cell can only ever return something within the cap — the
// assertion would hold no matter how narrow the column got. Adding
// "claude-sonnet-4-10" (pair: 24 chars) must fail this test, because
// that combination's sub-agent model would be invisible at every
// terminal width.
func TestWidestModelPairFitsTheCap(t *testing.T) {
	widest, widestPair := 0, ""
	for _, a := range knownModelIDs {
		for _, b := range knownModelIDs {
			// main + " (" + sub + ")"
			n := len(usage.ShortModel(a)) + 2 + len(usage.ShortModel(b)) + 1
			if n > widest {
				widest, widestPair = n, usage.ShortModel(a)+" ("+usage.ShortModel(b)+")"
			}
		}
	}
	if widest > modelMaxWidth {
		t.Errorf("widest pair %q is %d chars, exceeds modelMaxWidth=%d — that pair's sub-agent model would never be shown",
			widestPair, widest, modelMaxWidth)
	}
	t.Logf("widest pair: %q (%d chars, cap %d)", widestPair, widest, modelMaxWidth)
}

// Both row renderers must show the pair — the highlighted row builds its
// cell separately.
func TestBothRowsRenderTheSubagentModel(t *testing.T) {
	m := modelWith(
		state.Track{ID: "t1", Status: state.StatusRunning, ObservedModel: "claude-opus-5", ObservedSubagentModel: "claude-haiku-4-5"},
		state.Track{ID: "t2", Status: state.StatusRunning, ObservedModel: "claude-sonnet-5", ObservedSubagentModel: "claude-opus-4-8"},
	)
	m.width, m.height = 250, 40
	m.cursor = 0
	out := stripStyles(m.View())
	for _, want := range []string{"opus-5 (haiku-4-5)", "sonnet-5 (opus-4-8)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A Cursor track never has an observed model — internal/usage reads
// Claude transcripts and Cursor writes none — so the column would be
// permanently blank without the requested-model fallback.
func TestModelFallsBackToTheRequestedModel(t *testing.T) {
	cursorTrack := state.Track{
		Provider: state.ProviderCursor, RequestedModel: "gpt-5.3-codex",
	}
	if got := trackModel(cursorTrack, modelMaxWidth); got != "gpt-5.3-codex" {
		t.Errorf("cursor track shows %q, want its requested model", got)
	}
}

// Observed still wins wherever it exists: it is what actually ran, and
// it follows an in-pane /model switch that the requested value cannot.
func TestObservedModelBeatsRequested(t *testing.T) {
	tr := state.Track{
		ObservedModel:  "claude-sonnet-4-6", // the user switched mid-session
		RequestedModel: "claude-opus-4-8",
	}
	if got := trackModel(tr, modelMaxWidth); got != "sonnet-4-6" {
		t.Errorf("trackModel = %q, want the observed model to win", got)
	}
}

// Before the first turn a Claude track now shows what it will run
// rather than an em-dash — strictly more than we knew before.
func TestClaudeTrackShowsItsPickBeforeTheFirstTurn(t *testing.T) {
	tr := state.Track{RequestedModel: "claude-opus-4-8"}
	if got := trackModel(tr, modelMaxWidth); got != "opus-4-8" {
		t.Errorf("trackModel = %q, want the requested model before any turn", got)
	}
}

// And with nothing at all it is still blank.
func TestModelBlankWithNeither(t *testing.T) {
	if got := trackModel(state.Track{}, modelMaxWidth); got != "" {
		t.Errorf("trackModel = %q, want empty", got)
	}
}
