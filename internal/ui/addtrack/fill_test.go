package addtrack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

func fillForm(create CreateFunc) Model {
	runsOn := map[track.Kind]RunsOn{track.Work: {Engine: "claude", Model: "opus"}, track.Ask: {Engine: "claude"}}
	m, _ := send(New(Config{Theme: theme.Default(), Repos: []string{"api", "web"}, RunsOn: runsOn, Engines: []Engine{claudeCode}, Create: create}), resize)
	return m
}

func TestFillBuildsTheRequestBack(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(doc, []byte("# Spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, req := range []tracks.Request{
		{Kind: track.Work, Name: "Rate bug", Repos: []string{"api", "web"}, Prompt: "Fix it", Terminal: true, Engine: "claude", Model: "opus"},
		{Kind: track.Ask, Repos: []string{"web"}, Prompt: "Why?", Engine: "claude", Model: "claude-opus-4-8"},
		{Kind: track.Plan, Prompt: "Plan it", Engine: "claude", Model: "sonnet"},
		{Kind: track.Review, Repos: []string{"web"}, ReviewRef: "https://github.com/acme/web/pull/3", Prompt: "Look", Candor: 3, Engine: "claude", Model: "sonnet"},
		{Kind: track.Doc, Repos: []string{"api"}, Document: doc, Prompt: "Read", Candor: 1, Opinion: false, ClaimCheck: true, Engine: "claude", Model: "sonnet"},
	} {
		req.Draft = "20260930-170000-abcdef"
		m := fillForm(nil).Fill(req)
		if got := m.request(); !reflect.DeepEqual(got, req) {
			t.Errorf("%s: the filled form asks for\n%+v\nwant\n%+v", req.Kind, got, req)
		}
		if m.notice != "" {
			t.Errorf("%s: notice %q", req.Kind, m.notice)
		}
	}
}

func TestFillLeavesOutGoneRepos(t *testing.T) {
	m := fillForm(nil).Fill(tracks.Request{Kind: track.Work, Repos: []string{"api", "old"}, Prompt: "Fix", Draft: "d"})
	if got := m.request().Repos; len(got) != 1 || got[0] != "api" {
		t.Errorf("repos %q, want api only", got)
	}
	if !strings.Contains(m.notice, "old isn't on the Repositories tab") {
		t.Errorf("notice %q should name the repo left out", m.notice)
	}
}

func TestRetriesKeepTheDraftID(t *testing.T) {
	var drafts []string
	create := func(_ context.Context, req tracks.Request, _ func(string)) (Created, error) {
		drafts = append(drafts, req.Draft)
		return Created{}, errors.New("git: exit 128")
	}
	m := ready(t, create)
	for range 2 {
		var cmd tea.Cmd
		m, cmd = pressCreate(t, m)
		m, _ = step(m, cmd)
	}
	if len(drafts) != 2 || drafts[0] == "" || drafts[0] != drafts[1] {
		t.Errorf("draft IDs %q; want one, the same for each try", drafts)
	}
	if m.failure != "Couldn't create the track: git: exit 128" {
		t.Errorf("failure %q", m.failure)
	}
	m, _ = m.close()
	if m.discard == nil || !strings.Contains(plain(m), draftDiscardText) {
		t.Errorf("closing after a failure should say the draft stays:\n%s", plain(m))
	}
}
