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
		ID: "t1", Status: state.StatusRunning, Model: "claude-haiku-4-5-20251001",
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
		state.Track{ID: "t1", Status: state.StatusRunning, Model: "claude-opus-5"},
		state.Track{ID: "t2", Status: state.StatusRunning, Model: "claude-sonnet-5"},
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
	if got := stripStyles(m.renderModel(m.tracks[0])); strings.TrimSpace(got) != "—" {
		t.Errorf("renderModel with no model = %q, want the em-dash placeholder", got)
	}
}

// knownModelIDs are the model ids actually in circulation. The MODEL
// column exists to tell them apart, so truncating one defeats the
// column: "sonnet-4…" is indistinguishable from sonnet-4-5. Add new ids
// here when they appear — a failure means modelColWidth needs raising,
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
		if len(short) > modelColWidth {
			t.Errorf("ShortModel(%q) = %q (%d chars) exceeds modelColWidth=%d — it would render truncated",
				id, short, len(short), modelColWidth)
		}
	}
}

// Two ids that differ only in their point release must stay distinct
// after shortening and truncation — the exact failure a too-narrow
// column produces.
func TestSimilarModelsStayDistinguishable(t *testing.T) {
	m := modelWith(state.Track{ID: "t"})
	a := stripStyles(m.renderModel(state.Track{Model: "claude-sonnet-4-5"}))
	b := stripStyles(m.renderModel(state.Track{Model: "claude-sonnet-4-6"}))
	if strings.TrimSpace(a) == strings.TrimSpace(b) {
		t.Errorf("sonnet-4-5 and sonnet-4-6 both render as %q", strings.TrimSpace(a))
	}
}
