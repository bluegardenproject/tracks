package tracks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// ServerPanes runs the dev servers in panes of the track windows.
type ServerPanes interface {
	Panes(window string) ([]tmux.Pane, error)
	AddDevServer(window, dir string, p trackwin.Process, key string, port int) error
	// StopPane stops what runs in p, its whole process group, and
	// closes it.
	StopPane(p tmux.Pane) error
	// Capture is pane's last lines rows, scrollback included.
	Capture(pane string, lines int) (string, error)
}

// Dev server states.
const (
	ServerStopped  = "stopped"  // no pane
	ServerStarting = "starting" // running, not listening yet
	ServerReady    = "ready"    // listening
	ServerExited   = "exited"   // it ended with code 0; its pane stays
	ServerCrashed  = "crashed"  // it ended with another code; its pane shows why
	// ServerRunning is a pane's own state while its server runs; a
	// Server is then starting or ready.
	ServerRunning = "running"
)

// Server is one of a track's dev servers, or a server found listening
// in one of its panes, which has no Repo or Name.
type Server struct {
	Track     string `json:"track"`
	TrackName string `json:"track_name"`
	Repo      string `json:"repo,omitempty"`
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	// Inferred says Type was guessed from the listening process.
	Inferred bool   `json:"inferred,omitempty"`
	Mode     string `json:"mode"`
	Port     int    `json:"port,omitempty"` // the port it was started on, 0 for none
	State    string `json:"state"`
	Code     int    `json:"code,omitempty"`
	// Problem is why Up didn't start it.
	Problem string `json:"problem,omitempty"`
}

// devServer is a dev server of one of a track's repos.
type devServer struct {
	repo  track.Repo
	def   store.DevServer
	setup bool // the repo has a setup to wait for
}

func (d devServer) key() string { return d.repo.Name + "/" + d.def.Name }

// Up starts track id's dev server name, or all of them for "", each in
// a pane once its repo's setup is done. Running ones are left alone;
// one that exited starts again. It returns every server's state; one
// that can't start, such as for its port, has a Problem and the others
// still start.
func (s *Service) Up(ctx context.Context, id, name string) ([]Server, error) {
	// Read before the lock: what Up starts is starting either way.
	snap := s.snapshot(ctx)
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	t, window, all, err := s.serversOf(ctx, id)
	if err != nil {
		return nil, err
	}
	targets, err := pick(all, name)
	if err != nil {
		return nil, err
	}
	panes, err := s.Servers.Panes(window)
	if err != nil {
		return nil, err
	}
	used := runningPorts(panes)
	problems := map[string]string{}
	for _, d := range targets {
		if p, ok := serverPane(panes, d.key()); ok {
			if serverState(p) == ServerRunning {
				continue
			}
			if err := s.Servers.StopPane(p); err != nil {
				return nil, err
			}
		}
		dir := filepath.Join(d.repo.Worktree, d.def.Dir)
		// tmux would start a pane whose folder is missing in $HOME.
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			folder := d.def.Dir
			if folder == "" {
				folder = d.repo.Worktree
			}
			problems[d.key()] = fmt.Sprintf("its folder %s isn't on this branch.", folder)
			continue
		}
		port, err := s.portFor(ctx, t, d, used)
		var problem Problem
		if errors.As(err, &problem) {
			problems[d.key()] = problem.Error()
			continue
		} else if err != nil {
			return nil, err
		}
		used[port] = true
		title := d.def.Name
		if port != 0 {
			title += " · :" + strconv.Itoa(port)
		}
		s.clearFailure(ctx, t.ID, track.ServerError, d.key())
		p := trackwin.Process{Title: title, Command: s.serverCommand(t.ID, d, port)}
		if err := s.Servers.AddDevServer(window, dir, p, d.key(), port); err != nil {
			return nil, fmt.Errorf("start %s: %w", d.def.Name, err)
		}
	}
	states, err := s.serverStates(t, window, all, snap, false)
	for i, sv := range states {
		states[i].Problem = problems[sv.Repo+"/"+sv.Name]
	}
	return states, err
}

// Down stops track id's dev server name, or all of them for "", and
// closes their panes.
func (s *Service) Down(ctx context.Context, id, name string) ([]Server, error) {
	snap := s.snapshot(ctx)
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	t, window, all, err := s.serversOf(ctx, id)
	if err != nil {
		return nil, err
	}
	targets, err := pick(all, name)
	if err != nil {
		return nil, err
	}
	panes, err := s.Servers.Panes(window)
	if err != nil {
		return nil, err
	}
	for _, d := range targets {
		if p, ok := serverPane(panes, d.key()); ok {
			if err := s.Servers.StopPane(p); err != nil {
				return nil, fmt.Errorf("stop %s: %w", d.def.Name, err)
			}
		}
		s.clearFailure(ctx, t.ID, track.ServerError, d.key())
	}
	return s.serverStates(t, window, all, snap, false)
}

// Logs is the last lines lines dev server name of track id printed.
func (s *Service) Logs(ctx context.Context, id, name string, lines int) (string, error) {
	_, window, all, err := s.serversOf(ctx, id)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", Problem("Name the server whose logs to show.")
	}
	if lines < 1 {
		return "", Problem("Show at least one line.")
	}
	targets, err := pick(all, name)
	if err != nil {
		return "", err
	}
	panes, err := s.Servers.Panes(window)
	if err != nil {
		return "", err
	}
	p, ok := serverPane(panes, targets[0].key())
	if !ok {
		return "", Problem(targets[0].def.Name + " isn't running; start it with tracks up.")
	}
	return s.Servers.Capture(p.ID, lines)
}

// serversOf reads track id, its window, and the dev servers of its
// repos that have a worktree.
func (s *Service) serversOf(ctx context.Context, id string) (track.Track, string, []devServer, error) {
	if s.Servers == nil {
		return track.Track{}, "", nil, errors.New("dev servers need tmux")
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
	all, err := s.devServers(ctx, t)
	return t, window, all, err
}

// hasDevServers reports whether any of repos has a worktree and dev
// servers.
func (s *Service) hasDevServers(ctx context.Context, repos []track.Repo) bool {
	all, err := s.devServers(ctx, track.Track{Repos: repos})
	return err == nil && len(all) > 0
}

// devServers are the dev servers of t's repos that have a worktree.
func (s *Service) devServers(ctx context.Context, t track.Track) ([]devServer, error) {
	configs, err := s.repoConfigs(ctx)
	if err != nil {
		return nil, err
	}
	var all []devServer
	for _, r := range t.Repos {
		c, ok := configs[r.RepoID]
		if !ok || r.Worktree == "" {
			continue
		}
		for _, def := range c.Servers {
			all = append(all, devServer{repo: r, def: def, setup: c.Setup != ""})
		}
	}
	return all, nil
}

// pick is the servers name means: all for "", else the one called name
// or repo/name.
func pick(all []devServer, name string) ([]devServer, error) {
	if len(all) == 0 {
		return nil, Problem("This track's repos have no dev servers. Add them on the Repositories tab.")
	}
	if name == "" {
		return all, nil
	}
	var found []devServer
	names := make([]string, len(all))
	for i, d := range all {
		names[i] = d.key()
		if strings.EqualFold(d.key(), name) || strings.EqualFold(d.def.Name, name) {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return nil, Problem(fmt.Sprintf("No dev server %s. Servers: %s.", name, strings.Join(names, ", ")))
	case 1:
		return found, nil
	}
	return nil, Problem(fmt.Sprintf("More than one repo has a server %s; name it as repo/server: %s.", name, strings.Join(names, ", ")))
}

func serverPane(panes []tmux.Pane, key string) (tmux.Pane, bool) {
	for _, p := range panes {
		if p.Role == trackwin.RoleDevServer && p.Key == key {
			return p, true
		}
	}
	return tmux.Pane{}, false
}

// serverState is what a dev-server pane's state means. A pane opened
// a moment ago hasn't set its state yet.
func serverState(p tmux.Pane) string {
	if !strings.HasPrefix(p.State, ServerExited) {
		return ServerRunning
	}
	if exitCode(p.State) == 0 {
		return ServerExited
	}
	return ServerCrashed
}

// exitCode is the code in a pane state such as "exited 1".
func exitCode(state string) int {
	_, code, _ := strings.Cut(state, " ")
	n, _ := strconv.Atoi(code)
	return n
}

// runningPorts are the ports of panes' running dev servers.
func runningPorts(panes []tmux.Pane) map[int]bool {
	out := map[int]bool{}
	for _, p := range panes {
		if p.Role == trackwin.RoleDevServer && p.Port != 0 && serverState(p) == ServerRunning {
			out[p.Port] = true
		}
	}
	return out
}

// serverCommand is what a dev-server pane runs: the repo's setup is
// waited for, then the server, with $PORT and {{port}} set when it has
// a port. The pane keeps its state in trackwin.StateOption, and once the
// server ends it stays open on a shell, showing why.
func (s *Service) serverCommand(id string, d devServer, port int) string {
	command := d.def.Command
	if port != 0 {
		command = strings.ReplaceAll(command, "{{port}}", strconv.Itoa(port))
	}
	// ran is 0 when the setup failed and the server never started: that
	// is the setup's error, not a crash of the server.
	run := "sh -c " + shellx.Quote(command) + "\ncode=$?\nran=1\n"
	if d.setup {
		run = "if tracks setup --wait; then\n  sh -c " + shellx.Quote(command) + "\n  code=$?\n  ran=1\n" +
			"else\n  code=$?\n  ran=0\nfi\n"
	}
	inner := `tmux set-option -p -t "$TMUX_PANE" ` + trackwin.StateOption + " " + ServerRunning + " 2>/dev/null\n" +
		run +
		`tmux set-option -p -t "$TMUX_PANE" ` + trackwin.StateOption + ` "` + ServerExited + ` $code" 2>/dev/null` + "\n" +
		`if [ "$ran" -eq 0 ]; then` + "\n" +
		"  printf '\\n%s didn\\047t start: the setup didn\\047t finish.\\n' " + shellx.Quote(d.def.Name) + "\n" +
		"else\n" +
		`  [ "$code" -eq 0 ] || tracks report-exit --kind ` + track.ServerError + " --subject " + shellx.Quote(d.key()) + ` --code "$code" 2>/dev/null` + "\n" +
		"  printf '\\n%s stopped (exit %s).\\n' " + shellx.Quote(d.def.Name) + ` "$code"` + "\n" +
		"fi\n" +
		"exec ${SHELL:-bash} -l"
	env := "TRACKS_ID=" + shellx.Quote(id) + " TRACKS_SOCKET_DIR=" + shellx.Quote(s.SocketDir)
	if s.BinDir != "" {
		env += " PATH=" + shellx.Quote(s.BinDir) + `:"$PATH"`
	}
	if port != 0 {
		env += " PORT=" + strconv.Itoa(port)
	}
	return env + " sh -c " + shellx.Quote(inner)
}
