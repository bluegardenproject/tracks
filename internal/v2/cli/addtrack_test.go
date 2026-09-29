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
		track.Work:   {Engine: "Claude Code", Model: "opus"},
		track.Ask:    {Engine: "Cursor", Missing: true},
		track.Plan:   {Engine: "Claude Code", Model: "sonnet"},
		track.Review: {Engine: "Claude Code", Model: "opus"},
		track.Doc:    {Engine: "Claude Code", Model: "opus"},
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
