package tracks

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// Promote makes Ask or Plan track id a Work track: worktrees on a new
// branch, and a new session on the same agent and model that starts
// with the original task, v1's note and, where the engine can read it,
// the old session's last reply. An open track's agent restarts in its
// window; an ended one gets a window again. The track keeps its ID.
// When a step fails, what Promote made is undone.
func (s *Service) Promote(ctx context.Context, id string, progress func(string)) (Created, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return Created{}, err
	}
	defer release()
	switch {
	case t.Kind == track.Doc:
		return Created{}, Problem("A Doc track can't be promoted: start a Work track for the changes it found.")
	case t.Kind != track.Ask && t.Kind != track.Plan:
		return Created{}, Problem("Only Ask and Plan tracks can be promoted.")
	case t.Archived():
		return Created{}, Problem(t.Name + " is archived: unarchive it first.")
	case len(t.Repos) == 0:
		return Created{}, Problem(t.Name + " has no repos to promote.")
	}
	info, known := agents.ByID(t.Engine)
	engine, ok := s.engine(t.Engine)
	if !known || !ok {
		return Created{}, Problem(fmt.Sprintf("Tracks doesn't know the engine %s.", t.Engine))
	}
	set, err := s.Settings()
	if err != nil {
		return Created{}, err
	}
	conf := set.Engines.Get(t.Engine)
	if conf == nil {
		return Created{}, Problem(fmt.Sprintf("Add %s on the Engines tab to promote this track.", info.Name))
	}

	window := ""
	if t.Open() {
		infos, err := s.Windows.List()
		if err != nil {
			return Created{}, err
		}
		for _, in := range infos {
			if in.Track == t.ID {
				window = in.Window
			}
		}
	}
	work := t
	work.Kind = track.Work
	if window == "" {
		name, releaseName, err := s.claimName(t)
		if err != nil {
			return Created{}, err
		}
		defer releaseName()
		work.Name = name
	}

	if work.Repos, err = s.Worktrees.Add(ctx, work, progress); err != nil {
		return Created{}, err
	}
	undo := context.WithoutCancel(ctx)
	removeWorktrees := func() { _ = s.Worktrees.Remove(undo, work) }
	if t.Engine == agents.Cursor.ID {
		progress("Starting a Cursor chat…")
	}
	if work.Session, err = engine.Session(ctx, info.Program); err != nil {
		removeWorktrees()
		return Created{}, err
	}
	work.Prompt = promotePrompt(t.Prompt, work.Repos[0].Branch, engine.LastReply(t.Session))
	drafts, err := s.draftRepos(ctx, work.Repos)
	if err != nil {
		removeWorktrees()
		return Created{}, err
	}
	hooks, err := s.installHooks(work)
	if err != nil {
		removeWorktrees()
		return Created{}, err
	}
	start, err := engine.Command(agents.Spec{
		Track: work, Program: info.Program, Auto: conf.AutoMode(), DraftPRs: drafts,
		SocketDir: s.SocketDir, BinDir: s.BinDir, Hooks: hooks,
	})
	if err != nil {
		removeWorktrees()
		return Created{}, err
	}

	progress(fmt.Sprintf("Starting %s…", info.Name))
	var win trackwin.Window
	if window != "" {
		win, err = s.Windows.Respawn(window, windowSpec(work, info, start))
	} else {
		win, err = s.open(work, info, start)
	}
	if err != nil {
		removeWorktrees()
		return Created{}, fmt.Errorf("start %s: %w", info.Name, err)
	}
	if err = s.Store.Promote(undo, work); err == nil && work.Name != t.Name {
		err = s.Store.Rename(undo, work.ID, work.Name)
	}
	if err != nil {
		// The agent can't be put back as it was: the track ends, and
		// can be resumed as the Ask or Plan track it's saved as.
		_ = s.Windows.Close(win.ID)
		removeWorktrees()
		return Created{}, fmt.Errorf("save the track: %w", err)
	}
	work.State = track.State{}
	return Created{Track: work, Window: win}, nil
}

// promotePrompt is v1's: the original task first, then that the
// worktree is ready, and the reply the read-only phase ended with.
func promotePrompt(original, branch, reply string) string {
	prompt := strings.TrimRight(original, " \t\n\r") +
		"\n\n---\nThe read-only investigation/plan phase is complete. A worktree " +
		"has been created on branch `" + branch + "` — implement the change here."
	if reply != "" {
		prompt += "\n\nThis is how you ended that phase:\n\n" + reply
	}
	return prompt
}

// draftRepos are the names of repos whose pull requests open as drafts.
func (s *Service) draftRepos(ctx context.Context, repos []track.Repo) ([]string, error) {
	all, err := s.Store.Repos(ctx)
	if err != nil {
		return nil, err
	}
	var drafts []string
	for _, r := range repos {
		for _, known := range all {
			if known.ID == r.RepoID && known.DraftPRs {
				drafts = append(drafts, r.Name)
			}
		}
	}
	return drafts, nil
}
