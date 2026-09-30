package cli

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/addtrack"
)

func TestRunsOnPerType(t *testing.T) {
	s := settings.Settings{
		Engines: settings.Engines{Claude: &settings.Engine{Model: "opus"}},
		Tracks: settings.Tracks{
			Ask:  &settings.TrackType{Engine: "cursor"},
			Plan: &settings.TrackType{Engine: "claude", Model: "sonnet"},
		},
	}
	got := runsOn(s)
	want := map[track.Kind]addtrack.RunsOn{
		track.Work:   {Engine: "claude"},
		track.Ask:    {Engine: "cursor"},
		track.Plan:   {Engine: "claude", Model: "sonnet"},
		track.Review: {Engine: "claude"},
		track.Doc:    {Engine: "claude"},
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s runs on %+v, want %+v", k, got[k], w)
		}
	}
	if len(runsOn(settings.Settings{})) != 0 {
		t.Error("with nothing added and nothing set, no type runs on anything")
	}
}

func TestTheFormOffersTheAddedEngines(t *testing.T) {
	s := settings.Settings{Engines: settings.Engines{
		Claude: &settings.Engine{Model: "opus", Models: []string{"claude-opus-4-8"}},
	}}
	got := addedEngines(s)
	if len(got) != 1 {
		t.Fatalf("engines %+v", got)
	}
	if e := got[0]; e.ID != "claude" || e.Name != "Claude Code" || e.Default != "opus" || len(e.Models) == 0 ||
		len(e.Added) != 1 || e.Added[0] != "claude-opus-4-8" || e.Lists {
		t.Errorf("Claude Code is offered as %+v", e)
	}
	s.Engines.Cursor = &settings.Engine{}
	if got := addedEngines(s); len(got) != 2 || got[1].ID != "cursor" || !got[1].Lists {
		t.Errorf("engines %+v", got)
	}
}
