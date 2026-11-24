package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestNewRequest(t *testing.T) {
	req, err := newRequest(" Plan ", newTrackFlags{name: "plugins", repos: []string{"tracks"}}, "  Plan a plugin system.\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Kind != track.Plan || req.Name != "plugins" || len(req.Repos) != 1 || req.Prompt != "Plan a plugin system." || req.Parent != "" {
		t.Errorf("request %+v", req)
	}

	if inside, err := newRequest("work", newTrackFlags{repos: []string{"tracks"}}, "Fix it.", "20261124-151900-737de6"); err != nil || inside.Parent != "20261124-151900-737de6" {
		t.Errorf("run in a track: %+v, %v; want that track as its parent", inside, err)
	}

	spec := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(spec, []byte("# Spec"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := newRequest("doc", newTrackFlags{document: spec}, "", "")
	if err != nil || doc.Document != spec || !doc.Opinion || !doc.ClaimCheck || !doc.NoDraft {
		t.Errorf("a doc track %+v: %v, want its document, Opinion and Claim check as the form has them", doc, err)
	}
	if doc, _ := newRequest("doc", newTrackFlags{document: spec, noOpinion: true, noClaimCheck: true}, "", ""); doc.Opinion || doc.ClaimCheck {
		t.Errorf("--no-opinion and --no-claim-check were ignored: %+v", doc)
	}

	for _, tc := range []struct {
		kind string
		f    newTrackFlags
		want string
	}{
		{"feature", newTrackFlags{}, "no track type"},
		{"work", newTrackFlags{model: "opus"}, "--model needs --engine"},
		{"work", newTrackFlags{ref: "main"}, "--ref doesn't go with a work track"},
		{"plan", newTrackFlags{candor: 3}, "--candor doesn't go with a plan track"},
		{"review", newTrackFlags{noOpinion: true}, "--no-opinion doesn't go with a review track"},
	} {
		if _, err := newRequest(tc.kind, tc.f, "task", ""); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("newRequest(%q, %+v) = %v, want %q", tc.kind, tc.f, err, tc.want)
		}
	}
}

func TestReadPrompt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(file, []byte("from a file"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		f    newTrackFlags
		want string
	}{
		{newTrackFlags{prompt: "inline"}, "inline"},
		{newTrackFlags{promptFile: "-"}, "from stdin"},
		{newTrackFlags{promptFile: file}, "from a file"},
		{newTrackFlags{}, ""},
	} {
		got, err := readPrompt(strings.NewReader("from stdin"), tc.f)
		if err != nil || got != tc.want {
			t.Errorf("readPrompt(%+v) = %q, %v, want %q", tc.f, got, err, tc.want)
		}
	}
	if _, err := readPrompt(strings.NewReader(""), newTrackFlags{prompt: "a", promptFile: "-"}); err == nil {
		t.Error("--prompt and --prompt-file together were accepted")
	}
}
