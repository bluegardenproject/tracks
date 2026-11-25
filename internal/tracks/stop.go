package tracks

import (
	"errors"
	"os/exec"
	"slices"
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
// process alone isn't enough. pgid <= 1 is refused so kill(-1, ...) can
// never broadcast to every user process.
func stopGroup(pgid int, grace time.Duration) error {
	if pgid <= 1 {
		return nil
	}
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
	live, ok, err := livePane(w.Tmux, p.ID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if !live.Dead && live.PID > 1 && serverState(live) == ServerRunning {
		if err := stopGroup(live.PID, stopGrace); err != nil {
			return err
		}
	}
	// The pane may have closed with its processes.
	if err := w.Tmux.KillPane(p.ID); err != nil {
		if _, still, checkErr := livePane(w.Tmux, p.ID); still || checkErr != nil {
			return err
		}
	}
	return nil
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

// EndPane ends every process group on p's terminal, then closes p.
// A shell runs each job it starts in a group of its own, so ending the
// pane's first process's group, as StopPane does, would leave a server
// started from a terminal running. The pane's first process, an
// interactive shell that ignores SIGTERM, ends with the pane, and the
// caller's own group is spared so that closing its own pane finishes.
// A dead pane's terminal is closed, and may be another pane's by now:
// nothing is signalled.
func (w TmuxWindows) EndPane(p tmux.Pane) error {
	// Re-query the pane immediately before inspecting its TTY so a
	// recycled PTY from an already-closed pane is never signalled.
	live, ok, err := livePane(w.Tmux, p.ID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if !live.Dead {
		if tty := strings.TrimPrefix(live.TTY, "/dev/"); tty != "" {
			out, err := exec.Command("ps", "-t", tty, "-o", "pgid=").Output()
			if err == nil {
				groups := groupsToEnd(ttyGroups(string(out)), live.PID, syscall.Getpgrp())
				errs := make([]error, len(groups))
				var wg sync.WaitGroup
				for i, g := range groups {
					wg.Go(func() { errs[i] = stopGroup(g, stopGrace) })
				}
				wg.Wait()
				if err := errors.Join(errs...); err != nil {
					return err
				}
			}
		}
	}
	// The pane may have closed with its processes.
	if err := w.Tmux.KillPane(p.ID); err != nil {
		if _, still, checkErr := livePane(w.Tmux, p.ID); still || checkErr != nil {
			return err
		}
	}
	return nil
}

// livePane returns the current state of pane id if it still exists.
// ListAllPanes succeeds even when id is already gone, so an absent pane
// reports (false, nil) while a tmux failure surfaces as an error.
func livePane(t trackwin.TrainTmux, id string) (tmux.Pane, bool, error) {
	panes, err := t.ListAllPanes()
	if err != nil {
		return tmux.Pane{}, false, err
	}
	for _, q := range panes {
		if q.ID == id {
			return q, true, nil
		}
	}
	return tmux.Pane{}, false, nil
}

// ttyGroups are the process groups (> 1) in `ps -o pgid=` output, each once.
func ttyGroups(ps string) []int {
	var groups []int
	for _, f := range strings.Fields(ps) {
		g, err := strconv.Atoi(f)
		if err == nil && g > 1 && !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	return groups
}

// groupsToEnd are groups without the pane's first process's, which
// ends with the pane, and the caller's own.
func groupsToEnd(groups []int, first, own int) []int {
	return slices.DeleteFunc(groups, func(g int) bool { return g == first || g == own })
}

func (w TmuxWindows) Capture(pane string, lines int) (string, error) {
	return w.Tmux.CaptureHistory(pane, lines)
}
