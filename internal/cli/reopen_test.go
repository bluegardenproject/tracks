package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

type fakeReopener struct {
	interrupted []track.Track
	reopening   []tracks.Reopening
	reopened    bool
}

func (f *fakeReopener) Interrupted(context.Context) ([]track.Track, error) {
	return f.interrupted, nil
}

func (f *fakeReopener) Reopen(_ context.Context, progress func(string)) ([]tracks.Reopening, error) {
	f.reopened = true
	for _, r := range f.reopening {
		progress("Reopening " + r.Name + "…")
	}
	return f.reopening, nil
}

func TestReopen(t *testing.T) {
	two := []track.Track{
		{ID: "a", Name: "fix-it", Kind: track.Work, Repos: []track.Repo{{Name: "api"}, {Name: "web"}}},
		{ID: "b", Name: "why", Kind: track.Ask},
	}
	tests := map[string]struct {
		interrupted []track.Track
		reopening   []tracks.Reopening
		reopens     bool
		window      string
		says        []string
	}{
		"nothing interrupted": {},
		"two": {
			interrupted: two, reopens: true,
			reopening: []tracks.Reopening{{ID: "a", Name: "fix-it", Window: "@1"}, {ID: "b", Name: "why", Window: "@2"}},
			says:      []string{"Reopening 2 tracks that were open when Tracks closed:", "  fix-it  work · api, web", "  why     ask", "  Reopening why…", "Reopened 2 tracks."},
		},
		"one": {
			interrupted: two[1:], reopens: true, window: "@2",
			reopening: []tracks.Reopening{{ID: "b", Name: "why", Window: "@2"}},
			says:      []string{"Reopening 1 track that was open when Tracks closed:", "Reopened 1 track."},
		},
		"one failing": {
			interrupted: two, reopens: true, window: "@2",
			reopening: []tracks.Reopening{{ID: "a", Name: "fix-it", Error: "the worktree couldn't be found for api"}, {ID: "b", Name: "why", Window: "@2"}},
			says:      []string{"Couldn't reopen fix-it: the worktree couldn't be found for api", "Reopened 1 track."},
		},
		"all failing": {
			interrupted: two[1:], reopens: true,
			reopening: []tracks.Reopening{{ID: "b", Name: "why", Error: "boom"}},
			says:      []string{"Couldn't reopen why: boom"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			daemon := &fakeReopener{interrupted: tt.interrupted, reopening: tt.reopening}
			var out strings.Builder
			window, err := reopen(context.Background(), daemon, &out)
			if err != nil {
				t.Fatal(err)
			}
			if daemon.reopened != tt.reopens || window != tt.window {
				t.Errorf("reopened %v, window %q; want %v, %q", daemon.reopened, window, tt.reopens, tt.window)
			}
			if tt.interrupted == nil && out.Len() > 0 {
				t.Errorf("said %q with nothing to reopen", out.String())
			}
			if strings.Contains(out.String(), "?") {
				t.Errorf("output %q asks a question", out.String())
			}
			for _, s := range tt.says {
				if !strings.Contains(out.String(), s) {
					t.Errorf("output %q lacks %q", out.String(), s)
				}
			}
		})
	}
}
