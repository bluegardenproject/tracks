package agents_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1claude "github.com/bluegardenproject/tracks/internal/claude"
	"github.com/bluegardenproject/tracks/internal/config"
	v1cursor "github.com/bluegardenproject/tracks/internal/cursor"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/agents/claude"
	"github.com/bluegardenproject/tracks/internal/v2/agents/cursor"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// v2Names and v2JiraErrors are what v2 changes in v1's prompts, and
// withoutDevServers what it leaves out; withoutLinks takes out the one
// thing it adds. v2Exit adds the line saving the agent's exit code to
// v1's command.
var (
	v2Exit = strings.NewReplacer("\nexec ${SHELL:-bash} -l",
		"\ncode=$?\ntmux set-option -w -t \"$TMUX_PANE\" @tracks_exit \"$code\" 2>/dev/null\nexec ${SHELL:-bash} -l")
	v2Names      = strings.NewReplacer("tracks-reviewer", "tracks-v2-reviewer", "tracks-docs-reviewer", "tracks-v2-docs-reviewer")
	v2JiraErrors = strings.NewReplacer("  4. Any Atlassian-tool error is non-fatal — note it in your reply and carry on with the actual work.",
		"  4. An error while assigning or moving the ticket is non-fatal — note it in your reply and carry on with the actual work. "+
			"A ticket you cannot read is not: follow **Links you cannot read**.")
)

// withoutDevServers takes the dev-server text out of v1's command for
// a work or review track, quoted twice as the pane's command line and
// its sh -c are.
func (c spawnCase) withoutDevServers(t *testing.T, command string) string {
	t.Helper()
	if c.kind == track.Doc || c.kind.ReadOnly() {
		return command
	}
	quoted := agents.DevServerContract + "\n\n"
	for range 2 {
		quoted = strings.ReplaceAll(quoted, "'", `'\''`)
	}
	if !strings.Contains(command, quoted) {
		t.Errorf("v1's command lacks the dev-server text:\n%s", command)
	}
	return strings.Replace(command, quoted, "", 1)
}

func withoutLinks(t *testing.T, command string) string {
	t.Helper()
	links := "\n\n" + agents.LinksContract
	if strings.Count(command, links) != 1 {
		t.Errorf("the command should carry the links rule once:\n%s", command)
	}
	return strings.Replace(command, links, "", 1)
}

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

// tracks builds the same track for v1 and v2.
func (c spawnCase) tracks(t *testing.T) (track.Track, state.Track, config.Config) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	v2 := track.Track{
		ID: "20260928-101500-abc123", Kind: c.kind, Model: c.model,
		Session: "11111111-2222-4333-8444-555555555555",
		Prompt:  "Fix what's flaky in \"CI\" $HOME\n\n", Candor: c.candor,
		Opinion: !c.skip, ClaimCheck: !c.skip,
	}
	if c.noSession {
		v2.Session = ""
	}
	v1 := state.Track{
		ID: v2.ID, Kind: state.Kind(c.kind), TaskPrompt: v2.Prompt,
		SessionID: v2.Session, RequestedModel: c.model,
	}
	cfg := config.Config{Claude: config.Claude{Binary: "claude", PermissionMode: "default"}, Cursor: config.Cursor{Binary: "agent"}}
	if c.auto {
		cfg.Claude.PermissionMode = "auto"
	}
	for i := range c.repos {
		name := []string{"api", "web"}[i]
		r := track.Repo{Name: name, Path: filepath.Join(dir, name)}
		if c.kind.Worktrees() {
			r.Worktree = filepath.Join(dir, "worktrees", name)
		}
		v2.Repos = append(v2.Repos, r)
		v1.Repos = append(v1.Repos, state.TrackRepo{Name: name, Path: r.Dir()})
		cfg.Repos = append(cfg.Repos, config.Repo{Name: name, Path: r.Path, DraftPRs: i < len(c.draft) && c.draft[i]})
	}
	if c.kind == track.Review || c.kind == track.Doc {
		v1.Review = &state.ReviewSpec{Candor: c.candor}
	}
	if c.doc != "" {
		v2.Document = filepath.Join(dir, c.doc)
		v1.Doc = &state.DocSpec{Path: v2.Document, SkipClaimCheck: c.skip, SkipOpinion: c.skip}
	}
	return v2, v1, cfg
}

func (c spawnCase) spec(v2 track.Track, program string) agents.Spec {
	s := agents.Spec{Track: v2, Program: program, Auto: c.auto, SocketDir: "/data/tracks", BinDir: "/data/tracks/bin"}
	for i, r := range v2.Repos {
		if i < len(c.draft) && c.draft[i] {
			s.DraftPRs = append(s.DraftPRs, r.Name)
		}
	}
	return s
}

func TestClaudeMatchesV1(t *testing.T) {
	for _, c := range spawnCases {
		t.Run(c.name, func(t *testing.T) {
			v2, v1, cfg := c.tracks(t)
			opts, err := v1claude.BuildOptions(cfg, v1, "/data/tracks", "")
			if err != nil {
				t.Fatal(err)
			}
			opts.BinDir = "/data/tracks/bin"
			got, err := claude.Command(c.spec(v2, "claude"))
			if err != nil {
				t.Fatal(err)
			}
			if want := v2Exit.Replace(v2JiraErrors.Replace(v2Names.Replace(c.withoutDevServers(t, opts.ShellCommand())))); withoutLinks(t, got.Command) != want {
				t.Errorf("command\n got: %s\nwant: %s", got.Command, want)
			}
			if got.Dir != opts.CWD {
				t.Errorf("dir = %q, want %q", got.Dir, opts.CWD)
			}
		})
	}
}

func TestCursorMatchesV1(t *testing.T) {
	for _, c := range spawnCases {
		t.Run(c.name, func(t *testing.T) {
			v2, v1, cfg := c.tracks(t)
			opts, err := v1cursor.BuildOptions(cfg, v1, "/data/tracks", "")
			if err != nil {
				t.Fatal(err)
			}
			opts.BinDir = "/data/tracks/bin"
			// v1 always forces; v2 forces only in auto mode.
			opts.Force = opts.Force && c.auto
			got, err := cursor.Command(c.spec(v2, "agent"))
			if err != nil {
				t.Fatal(err)
			}
			if want := v2Exit.Replace(v2Names.Replace(c.withoutDevServers(t, opts.ShellCommand()))); withoutLinks(t, got.Command) != want {
				t.Errorf("command\n got: %s\nwant: %s", got.Command, want)
			}
			if got.Dir != opts.CWD {
				t.Errorf("dir = %q, want %q", got.Dir, opts.CWD)
			}
		})
	}
}

func TestClaudeResumeMatchesV1(t *testing.T) {
	for _, c := range spawnCases {
		t.Run(c.name, func(t *testing.T) {
			v2, v1, cfg := c.tracks(t)
			opts, err := v1claude.BuildResumeOptions(cfg, v1, "/data/tracks", "")
			if err != nil {
				t.Fatal(err)
			}
			opts.BinDir = "/data/tracks/bin"
			// v1 resumes ask and plan in the configured mode.
			if c.kind.ReadOnly() {
				opts.PermissionMode = "plan"
			}
			s := c.spec(v2, "claude")
			s.Resume = true
			got, err := claude.Command(s)
			if err != nil {
				t.Fatal(err)
			}
			if want := v2Exit.Replace(opts.ShellCommand()); got.Command != want {
				t.Errorf("command\n got: %s\nwant: %s", got.Command, want)
			}
			if got.Dir != opts.CWD {
				t.Errorf("dir = %q, want %q", got.Dir, opts.CWD)
			}
		})
	}
}

func TestCursorResumeMatchesV1(t *testing.T) {
	for _, c := range spawnCases {
		t.Run(c.name, func(t *testing.T) {
			v2, v1, cfg := c.tracks(t)
			opts, err := v1cursor.BuildResumeOptions(cfg, v1, "/data/tracks", "")
			if err != nil {
				t.Fatal(err)
			}
			opts.BinDir = "/data/tracks/bin"
			opts.Force = opts.Force && c.auto
			s := c.spec(v2, "agent")
			s.Resume = true
			got, err := cursor.Command(s)
			if err != nil {
				t.Fatal(err)
			}
			if want := v2Exit.Replace(opts.ShellCommand()); got.Command != want {
				t.Errorf("command\n got: %s\nwant: %s", got.Command, want)
			}
			if got.Dir != opts.CWD {
				t.Errorf("dir = %q, want %q", got.Dir, opts.CWD)
			}
		})
	}
}

func TestCommandErrors(t *testing.T) {
	norepos := spawnCase{kind: track.Work, auto: true}
	v2, _, _ := norepos.tracks(t)
	if _, err := claude.Command(norepos.spec(v2, "claude")); err != agents.ErrNoRepos {
		t.Errorf("claude without repos: %v", err)
	}
	if _, err := cursor.Command(norepos.spec(v2, "agent")); err != agents.ErrNoRepos {
		t.Errorf("cursor without repos: %v", err)
	}
	nochat := spawnCase{kind: track.Ask, noSession: true}
	v2, _, _ = nochat.tracks(t)
	if _, err := cursor.Command(nochat.spec(v2, "agent")); err != cursor.ErrNoChat {
		t.Errorf("cursor without a chat: %v", err)
	}
	s := nochat.spec(v2, "claude")
	s.Resume = true
	if _, err := claude.Command(s); err != claude.ErrNoSession {
		t.Errorf("resuming claude without a session: %v", err)
	}
}
