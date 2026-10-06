package tracks

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/workspace"
)

// SetupPanes runs the repos' setups in panes of the track windows.
type SetupPanes interface {
	Panes(window string) ([]tmux.Pane, error)
	AddSetup(window, dir string, p trackwin.Process) error
	ClosePane(pane string) error
}

// Setup states of a track's repo.
const (
	SetupNone    = ""        // not run in this worktree yet
	SetupRunning = "running" // its pane is running it
	SetupDone    = "done"
	SetupFailed  = "failed" // its pane shows why
)

// SetupRepo is the setup of one of a track's repos.
type SetupRepo struct {
	Repo  string `json:"repo"`
	State string `json:"state"`
	// Code is a failed setup's exit code.
	Code int `json:"code,omitempty"`
}

// setupPoll is how often WaitSetup looks at the setup panes.
const setupPoll = 300 * time.Millisecond

// eagerSetup reports whether k's tracks start their setup when they're
// created; the others run it when first needed.
func eagerSetup(k track.Kind) bool { return k == track.Work }

// StartSetup starts the setup of track id's repos that have one and
// haven't run it, each in a pane. retry runs failed ones again too.
// It returns every setup's state.
func (s *Service) StartSetup(ctx context.Context, id string, retry bool) ([]SetupRepo, error) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	t, window, commands, err := s.setupOf(ctx, id)
	if err != nil {
		return nil, err
	}
	panes, err := s.Setups.Panes(window)
	if err != nil {
		return nil, err
	}
	states := setupStates(t, commands, panes)
	for i, st := range states {
		if st.State == SetupFailed && retry {
			if p, ok := setupPane(panes, st.Repo); ok {
				if err := s.Setups.ClosePane(p.ID); err != nil {
					return states, err
				}
			}
			st.State = SetupNone
		}
		if st.State != SetupNone {
			continue
		}
		r := repoNamed(t.Repos, st.Repo)
		s.clearFailure(ctx, t.ID, track.SetupError, r.Name)
		p := trackwin.Process{Title: setupTitle(r.Name), Command: s.setupCommand(t.ID, r.Name, commands[r.Name])}
		if err := s.Setups.AddSetup(window, r.Worktree, p); err != nil {
			return states, fmt.Errorf("start the setup of %s: %w", r.Name, err)
		}
		states[i].State, states[i].Code = SetupRunning, 0
	}
	return states, nil
}

// WaitSetup starts the setups track id's repos never ran, then waits
// until none is running. Failed setups aren't run again.
func (s *Service) WaitSetup(ctx context.Context, id string) ([]SetupRepo, error) {
	states, err := s.StartSetup(ctx, id, false)
	for err == nil && running(states) {
		select {
		case <-ctx.Done():
			return states, ctx.Err()
		case <-time.After(setupPoll):
		}
		states, err = s.SetupStates(ctx, id)
	}
	return states, err
}

// SetupStates is the state of each setup of track id's repos.
func (s *Service) SetupStates(ctx context.Context, id string) ([]SetupRepo, error) {
	t, window, commands, err := s.setupOf(ctx, id)
	if err != nil {
		return nil, err
	}
	panes, err := s.Setups.Panes(window)
	if err != nil {
		return nil, err
	}
	return setupStates(t, commands, panes), nil
}

// FinishSetup records that the setup of track id's repo succeeded.
func (s *Service) FinishSetup(ctx context.Context, id, repo string) error {
	err := s.Store.SetSetupDone(ctx, id, repo, true)
	if errors.Is(err, store.ErrNotFound) {
		return Problem(fmt.Sprintf("Track %s has no repo %s.", id, repo))
	}
	if err == nil {
		s.clearFailure(ctx, id, track.SetupError, repo)
	}
	return err
}

// setupOf reads track id, its window and its repos' setup commands by
// repo name, for repos with a worktree and a setup.
func (s *Service) setupOf(ctx context.Context, id string) (track.Track, string, map[string]string, error) {
	if s.Setups == nil {
		return track.Track{}, "", nil, errors.New("setups need tmux")
	}
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return t, "", nil, Problem("That track is gone.")
	} else if err != nil {
		return t, "", nil, err
	}
	window, err := s.windowOf(id)
	if err != nil {
		return t, "", nil, err
	}
	commands, err := s.setupCommands(ctx, t.Repos)
	return t, window, commands, err
}

// setupCommands are the setup commands of repos that have a worktree
// and a setup, by repo name.
func (s *Service) setupCommands(ctx context.Context, repos []track.Repo) (map[string]string, error) {
	configs, err := s.repoConfigs(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range repos {
		if c, ok := configs[r.RepoID]; ok && r.Worktree != "" && c.Setup != "" {
			out[r.Name] = c.Setup
		}
	}
	return out, nil
}

func (s *Service) repoConfigs(ctx context.Context) (map[int64]store.Repo, error) {
	all, err := s.Store.Repos(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]store.Repo, len(all))
	for _, r := range all {
		out[r.ID] = r
	}
	return out, nil
}

// windowOf is the window of the open track id.
func (s *Service) windowOf(id string) (string, error) {
	infos, err := s.Windows.List()
	if err != nil {
		return "", err
	}
	for _, in := range infos {
		if in.Track == id {
			return in.Window, nil
		}
	}
	return "", Problem("That track has no open window.")
}

// setupStates works out each setup's state: done from the database,
// running or failed from its pane.
func setupStates(t track.Track, commands map[string]string, panes []tmux.Pane) []SetupRepo {
	var out []SetupRepo
	for _, r := range t.Repos {
		if _, ok := commands[r.Name]; !ok {
			continue
		}
		st := SetupRepo{Repo: r.Name}
		p, hasPane := setupPane(panes, r.Name)
		switch {
		// A pane opened a moment ago hasn't set its state yet.
		case hasPane && (p.State == SetupRunning || p.State == ""):
			st.State = SetupRunning
		case hasPane && strings.HasPrefix(p.State, SetupFailed):
			st.State = SetupFailed
			st.Code, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(p.State, SetupFailed)))
		case r.SetupDone:
			st.State = SetupDone
		}
		out = append(out, st)
	}
	return out
}

func setupPane(panes []tmux.Pane, repo string) (tmux.Pane, bool) {
	for _, p := range panes {
		if p.Role == trackwin.RoleSetup && p.Title == setupTitle(repo) {
			return p, true
		}
	}
	return tmux.Pane{}, false
}

// setupTitle is the title of repo's setup pane, which finds it again.
func setupTitle(repo string) string { return "setup · " + repo }

func running(states []SetupRepo) bool {
	for _, st := range states {
		if st.State == SetupRunning {
			return true
		}
	}
	return false
}

func repoNamed(repos []track.Repo, name string) track.Repo {
	for _, r := range repos {
		if r.Name == name {
			return r
		}
	}
	return track.Repo{}
}

// setupCommand is what a setup pane runs: command, in the worktree it
// opens in. The pane keeps its state in trackwin.StateOption. On
// success it tells the daemon and closes; on a failure it stays open on
// a shell, showing why.
func (s *Service) setupCommand(id, repo, command string) string {
	state := func(v string) string {
		return `tmux set-option -p -t "$TMUX_PANE" ` + trackwin.StateOption + " " + v + " 2>/dev/null\n"
	}
	inner := state(SetupRunning) +
		"printf '\\033[1m$ %s\\033[0m\\n' " + shellx.Quote(command) + "\n" +
		"sh -c " + shellx.Quote(command) + "\n" +
		"code=$?\n" +
		"if [ \"$code\" -eq 0 ]; then\n" +
		"  tracks setup done --repo " + shellx.Quote(repo) + " && exit 0\n" +
		"  msg=\"Setup finished, but Tracks couldn't record it.\"\n" +
		"else\n" +
		"  msg=\"Setup failed (exit $code).\"\n" +
		"  tracks report-exit --kind " + track.SetupError + " --subject " + shellx.Quote(repo) + " --code \"$code\" 2>/dev/null\n" +
		"fi\n" +
		state(`"`+SetupFailed+` $code"`) +
		"printf '\\n%s\\n' \"$msg\"\n" +
		"exec ${SHELL:-bash} -l"
	env := "TRACKS_ID=" + shellx.Quote(id) + " TRACKS_SOCKET_DIR=" + shellx.Quote(s.SocketDir)
	if s.BinDir != "" {
		env += " PATH=" + shellx.Quote(s.BinDir) + `:"$PATH"`
	}
	return env + " sh -c " + shellx.Quote(inner)
}

// hasSetup reports whether any of repos has a worktree and a setup.
func (s *Service) hasSetup(ctx context.Context, repos []track.Repo) bool {
	commands, err := s.setupCommands(ctx, repos)
	return err == nil && len(commands) > 0
}

// prepare readies repos' new worktrees in track t: their .env files
// are copied when the repo has a setup or dev servers. Copying is best
// effort: a track works without them.
func (s *Service) prepare(ctx context.Context, repos []track.Repo) {
	configs, err := s.repoConfigs(ctx)
	if err != nil {
		return
	}
	copyEnv := s.CopyEnv
	if copyEnv == nil {
		copyEnv = workspace.CopyEnv
	}
	for _, r := range repos {
		c, ok := configs[r.RepoID]
		if !ok || r.Worktree == "" || (c.Setup == "" && len(c.Servers) == 0) {
			continue
		}
		_, _ = copyEnv(ctx, r.Path, r.Worktree)
	}
}

// startEager starts t's setups when its kind runs them on creation.
func (s *Service) startEager(ctx context.Context, t track.Track) {
	if s.Setups == nil || !eagerSetup(t.Kind) {
		return
	}
	_, _ = s.StartSetup(ctx, t.ID, false)
}
