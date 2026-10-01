package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

func TestPRStatus(t *testing.T) {
	prs := []track.PR{
		{URL: "https://github.com/acme/web/pull/6", Repo: "acme/web", Number: 6, State: track.PRMerged},
		{URL: "https://github.com/acme/web/pull/7", Repo: "acme/web", Number: 7, State: track.PROpen},
		{URL: "https://github.com/acme/api/pull/8", Repo: "acme/api", Number: 8, State: track.PRDraft},
	}
	withPRs := func(prs []track.PR) source.Track {
		t := source.Track{ID: "a", Number: 1, Name: "open-one", Kind: "work", Status: track.Active, PRStatus: track.PRStatus(prs)}
		for _, p := range prs {
			t.PRs = append(t.PRs, source.PR{Repo: p.Repo, Number: p.Number, State: string(p.State), URL: p.URL})
		}
		return t
	}
	merged := withPRs(prs[:1])
	merged.ID, merged.Number, merged.Name, merged.Status = "b", 0, "merged-one", track.Done
	m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 140, Height: 40},
		tracksMsg{tracks: []source.Track{withPRs(prs), merged, {ID: "c", Number: 2, Name: "no-pr", Kind: "ask", Status: track.Active}}})

	view := plainView(m)
	for _, want := range []string{" active   2 PRs open ", " done   PR merged ", "work   active   2 PRs open ",
		"web#6 merged, web#7 open, api#8 draft"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "no-pr") && strings.Contains(line, "PR") {
			t.Errorf("a track without PRs shows no PR status: %q", line)
		}
	}

	if strings.Contains(view, "Open PR") {
		t.Errorf("the details offer no button for PRs, the agent handles them:\n%s", view)
	}
}
