package tracks

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

// Created is a new or resumed track and its window.
type Created struct {
	Track  track.Track
	Window trackwin.Window
}

// Create makes the track req asks for, as v1 does: check, ID, session,
// worktrees, the engine's command line, the window, and the database
// row last. progress is told each slow step. When a step fails, what
// was made is undone and nothing is saved.
func (s *Service) Create(ctx context.Context, req Request, progress func(string)) (Created, error) {
	set, err := s.Settings()
	if err != nil {
		return Created{}, err
	}
	id, model := set.RunsOn(string(req.Kind))
	engine, ok := s.engine(id)
	info, known := agents.ByID(id)
	switch {
	case id == "":
		return Created{}, ErrNoEngine
	case !ok || !known:
		return Created{}, Problem(fmt.Sprintf("Tracks doesn't know the engine %s.", id))
	case set.Engines.Get(id) == nil:
		return Created{}, Problem(fmt.Sprintf("Add %s on the Engines tab, or pick another agent for %s tracks in Settings → Tracks.",
			info.Name, title(req.Kind)))
	}
	conf := set.Engines.Get(id)

	t, drafts, err := s.check(ctx, req)
	if err != nil {
		return Created{}, err
	}
	t.ID, t.Engine, t.Model, t.CreatedAt = s.newID(), id, model, s.now()
	name, release, err := s.claimName(t)
	if err != nil {
		return Created{}, err
	}
	defer release()
	t.Name = name

	if id == agents.Cursor.ID {
		progress("Starting a Cursor chat…")
	}
	if t.Session, err = engine.Session(ctx, info.Program); err != nil {
		return Created{}, err
	}
	if t.Repos, err = s.Worktrees.Add(ctx, t, progress); err != nil {
		return Created{}, err
	}
	// Undoing runs to the end even when ctx is cancelled.
	undo := context.WithoutCancel(ctx)
	removeWorktrees := func() { _ = s.Worktrees.Remove(undo, t) }

	start, err := engine.Command(agents.Spec{
		Track: t, Program: info.Program, Auto: conf.AutoMode(), DraftPRs: drafts,
		SocketDir: s.SocketDir, BinDir: s.BinDir,
	})
	if err != nil {
		removeWorktrees()
		return Created{}, err
	}

	progress(fmt.Sprintf("Starting %s…", info.Name))
	win, err := s.open(t, info, start)
	if err != nil {
		removeWorktrees()
		return Created{}, err
	}
	if err := s.Store.AddTrack(undo, t); err != nil {
		_ = s.Windows.Close(win.ID)
		removeWorktrees()
		return Created{}, fmt.Errorf("save the track: %w", err)
	}
	return Created{Track: t, Window: win}, nil
}

// open opens t's window with the agent start runs. When setting the
// window up fails, it closes it again.
func (s *Service) open(t track.Track, info agents.Engine, start agents.Start) (trackwin.Window, error) {
	terminals := 0
	if t.Terminal {
		terminals = 1
	}
	win, err := s.Windows.Open(trackwin.Spec{
		Track: t.ID, Name: t.Name, Kind: string(t.Kind), Repo: repoNames(t.Repos), Dir: start.Dir,
		Agent: trackwin.Process{Title: info.Name, Command: start.Command}, Terminals: terminals,
	})
	if err != nil {
		if win.ID != "" {
			_ = s.Windows.Close(win.ID)
		}
		return trackwin.Window{}, fmt.Errorf("open the window: %w", err)
	}
	return win, nil
}

func repoNames(repos []track.Repo) string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return strings.Join(names, ",")
}

// title is k as a type's name: "Work".
func title(k track.Kind) string {
	if k == "" {
		return ""
	}
	return strings.ToUpper(string(k[:1])) + string(k[1:])
}
