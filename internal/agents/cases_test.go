package agents_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/claude"
	"github.com/bluegardenproject/tracks/internal/agents/cursor"
	"github.com/bluegardenproject/tracks/internal/track"
)

type spawnCase struct {
	name      string
	kind      track.Kind
	repos     int
	draft     []bool
	doc       string
	candor    int
	skip      bool
	auto      bool
	model     string
	noSession bool
}

var spawnCases = []spawnCase{
	{name: "work", kind: track.Work, repos: 1, auto: true, model: "opus"},
	{name: "work not auto", kind: track.Work, repos: 1},
	{name: "work all drafts", kind: track.Work, repos: 1, draft: []bool{true}, auto: true},
	{name: "work some drafts", kind: track.Work, repos: 2, draft: []bool{false, true}, auto: true},
	{name: "review", kind: track.Review, repos: 1, auto: true},
	{name: "review blunt", kind: track.Review, repos: 2, candor: 1, auto: true},
	{name: "ask", kind: track.Ask, repos: 1, auto: true},
	{name: "ask alone", kind: track.Ask, auto: true},
	{name: "plan", kind: track.Plan, repos: 2, auto: true},
	{name: "doc file", kind: track.Doc, doc: "spec.md", candor: 7, auto: true},
	{name: "doc folder", kind: track.Doc, doc: "docs", skip: true},
	{name: "doc with repos", kind: track.Doc, repos: 2, doc: "spec.md", auto: true},
}

// track builds the case's track, its repos and document in a temporary
// folder.
func (c spawnCase) track(t *testing.T) track.Track {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	tr := track.Track{
		ID: "20260928-101500-abc123", Kind: c.kind, Model: c.model,
		Session: "11111111-2222-4333-8444-555555555555",
		Prompt:  "Fix what's flaky in \"CI\" $HOME\n\n", Candor: c.candor,
		Opinion: !c.skip, ClaimCheck: !c.skip,
	}
	if c.noSession {
		tr.Session = ""
	}
	for i := range c.repos {
		name := []string{"api", "web"}[i]
		r := track.Repo{Name: name, Path: filepath.Join(dir, name)}
		if c.kind.Worktrees() {
			r.Worktree = filepath.Join(dir, "worktrees", name)
		}
		tr.Repos = append(tr.Repos, r)
	}
	if c.doc != "" {
		tr.Document = filepath.Join(dir, c.doc)
	}
	return tr
}

func (c spawnCase) spec(tr track.Track, program string) agents.Spec {
	s := agents.Spec{Track: tr, Program: program, Auto: c.auto, SocketDir: "/data/tracks", BinDir: "/data/tracks/bin"}
	for i, r := range tr.Repos {
		if i < len(c.draft) && c.draft[i] {
			s.DraftPRs = append(s.DraftPRs, r.Name)
		}
	}
	return s
}

func TestCommandErrors(t *testing.T) {
	norepos := spawnCase{kind: track.Work, auto: true}
	tr := norepos.track(t)
	if _, err := claude.Command(norepos.spec(tr, "claude")); err != agents.ErrNoRepos {
		t.Errorf("claude without repos: %v", err)
	}
	if _, err := cursor.Command(norepos.spec(tr, "agent")); err != agents.ErrNoRepos {
		t.Errorf("cursor without repos: %v", err)
	}
	nochat := spawnCase{kind: track.Ask, noSession: true}
	tr = nochat.track(t)
	if _, err := cursor.Command(nochat.spec(tr, "agent")); err != cursor.ErrNoChat {
		t.Errorf("cursor without a chat: %v", err)
	}
	s := nochat.spec(tr, "claude")
	s.Resume = true
	if _, err := claude.Command(s); err != claude.ErrNoSession {
		t.Errorf("resuming claude without a session: %v", err)
	}
}
