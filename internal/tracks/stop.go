package tracks

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// stopGrace is how long a dev server's processes get to exit after
// SIGTERM before they're killed.
const stopGrace = 5 * time.Second

// stopGroup ends process group pgid: SIGTERM, then SIGKILL for what's
// left after grace. Node servers fork workers, so the pane's first
// process alone isn't enough.
func stopGroup(pgid int, grace time.Duration) error {
	if err := syscall.Kill(-pgid, syscall.SIGTERM); errors.Is(err, syscall.ESRCH) {
		return nil
	} else if err != nil {
		return err
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !groupAlive(pgid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// groupAlive reports whether process group pgid has a process that
// isn't a zombie. kill(-pgid, 0) counts zombies too, which stay while
// nothing reaps them, as under a CI runner that adopts orphans.
func groupAlive(pgid int) bool {
	out, err := exec.Command("ps", "-A", "-o", "pgid=,stat=").Output()
	if err != nil {
		return !errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH)
	}
	return groupLiving(string(out), pgid)
}

// groupLiving reports whether ps output of pgid and stat lines lists a
// process of pgid that isn't a zombie.
func groupLiving(ps string, pgid int) bool {
	id := strconv.Itoa(pgid)
	for _, line := range strings.Split(ps, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == id && !strings.HasPrefix(f[1], "Z") {
			return true
		}
	}
	return false
}

// StopPane stops p's dev server, its whole process group, and closes
// the pane. A pane whose server ended runs only its shell, which
// closing the pane ends.
func (w TmuxWindows) StopPane(p tmux.Pane) error {
	if p.PID > 0 && serverState(p) == ServerRunning {
		if err := stopGroup(p.PID, stopGrace); err != nil {
			return err
		}
		// The pane may have closed with its processes.
		_ = w.Tmux.KillPane(p.ID)
		return nil
	}
	return w.Tmux.KillPane(p.ID)
}

// StopServers stops the dev servers in window, all at once.
func (w TmuxWindows) StopServers(window string) error {
	panes, err := w.Tmux.ListPanes(window)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make([]error, len(panes))
	for i, p := range panes {
		if p.Role != trackwin.RoleDevServer {
			continue
		}
		wg.Go(func() { errs[i] = w.StopPane(p) })
	}
	wg.Wait()
	return errors.Join(errs...)
}

// AddDevServer starts p in a pane of window. A pane that couldn't be
// labelled is closed: without its key Tracks can't find it again.
func (w TmuxWindows) AddDevServer(window, dir string, p trackwin.Process, key string, port int) error {
	pane, err := trackwin.AddDevServer(w.Tmux, window, dir, p, key, port)
	if err != nil && pane != "" {
		_ = w.Tmux.KillPane(pane)
	}
	return err
}

func (w TmuxWindows) Capture(pane string, lines int) (string, error) {
	return w.Tmux.CaptureHistory(pane, lines)
}
