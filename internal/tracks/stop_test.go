package tracks

import (
	"errors"
	"os/exec"
	"slices"
	"syscall"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tmux"
)

type stopTmuxStub struct {
	*tmux.Client
	panes   []tmux.Pane
	listErr error
	listFn  func() ([]tmux.Pane, error)
	killErr error
	killed  []string
}

func (s *stopTmuxStub) ListAllPanes() ([]tmux.Pane, error) {
	if s.listFn != nil {
		return s.listFn()
	}
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.panes, nil
}

func (s *stopTmuxStub) KillPane(pane string) error {
	s.killed = append(s.killed, pane)
	return s.killErr
}

func TestGroupLiving(t *testing.T) {
	ps := " 100 Ss\n 200 Z\n 200 Z+\n 300 S+\n"
	for pgid, want := range map[int]bool{100: true, 200: false, 300: true, 400: false} {
		if got := groupLiving(ps, pgid); got != want {
			t.Errorf("groupLiving(%d) = %v, want %v (zombies don't count)", pgid, got, want)
		}
	}
}

func TestTTYGroups(t *testing.T) {
	got := ttyGroups("  501\n  501\n  733\n\n 0\n 1\n -5\n")
	if !slices.Equal(got, []int{501, 733}) {
		t.Errorf("ttyGroups = %v, want [501 733]", got)
	}
	for _, pgid := range []int{-1, 0, 1} {
		if err := stopGroup(pgid, 0); err != nil {
			t.Errorf("stopGroup(%d) = %v, want nil no-op", pgid, err)
		}
	}
}

func TestGroupsToEnd(t *testing.T) {
	got := groupsToEnd([]int{100, 200, 300, 400}, 100, 300)
	if !slices.Equal(got, []int{200, 400}) {
		t.Errorf("groupsToEnd = %v, want [200 400]: the pane's shell and the caller are spared", got)
	}
}

func TestStopAndEndPaneLiveChecks(t *testing.T) {
	t.Run("gone pane is a no-op", func(t *testing.T) {
		stub := &stopTmuxStub{panes: []tmux.Pane{{ID: "%other", PID: 999999}}}
		w := TmuxWindows{Tmux: stub}
		stale := tmux.Pane{ID: "%gone", PID: 1, TTY: "/dev/ttys999", State: "running"}
		if err := w.StopPane(stale); err != nil {
			t.Fatalf("StopPane(gone) = %v", err)
		}
		if err := w.EndPane(stale); err != nil {
			t.Fatalf("EndPane(gone) = %v", err)
		}
		if len(stub.killed) != 0 {
			t.Errorf("expected no KillPane calls on gone pane, got %v", stub.killed)
		}
	})

	t.Run("dead pane skips signaling and still kills pane", func(t *testing.T) {
		cmd := exec.Command("sleep", "60")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		pid := cmd.Process.Pid
		t.Cleanup(func() {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = cmd.Wait()
		})
		// Caller passes a stale non-dead pane carrying the live process group,
		// while tmux reports the pane as Dead.
		stale := tmux.Pane{ID: "%dead", PID: pid, Dead: false, State: "running"}
		live := tmux.Pane{ID: "%dead", PID: pid, Dead: true, State: "running"}
		stub := &stopTmuxStub{panes: []tmux.Pane{live}}
		w := TmuxWindows{Tmux: stub}
		if err := w.StopPane(stale); err != nil {
			t.Fatalf("StopPane(dead) = %v", err)
		}
		if err := w.EndPane(stale); err != nil {
			t.Fatalf("EndPane(dead) = %v", err)
		}
		if !groupAlive(pid) {
			t.Errorf("process group %d was signalled even though live pane is dead", pid)
		}
		if !slices.Equal(stub.killed, []string{"%dead", "%dead"}) {
			t.Errorf("killed = %v, want [%%dead %%dead]", stub.killed)
		}
	})

	t.Run("kill error propagated when pane still exists", func(t *testing.T) {
		dead := tmux.Pane{ID: "%stuck", Dead: true}
		wantErr := errors.New("tmux error")
		stub := &stopTmuxStub{panes: []tmux.Pane{dead}, killErr: wantErr}
		w := TmuxWindows{Tmux: stub}
		if err := w.StopPane(dead); !errors.Is(err, wantErr) {
			t.Errorf("StopPane error = %v, want %v", err, wantErr)
		}
		if err := w.EndPane(dead); !errors.Is(err, wantErr) {
			t.Errorf("EndPane error = %v, want %v", err, wantErr)
		}
	})

	t.Run("list error surfaces instead of looking like a gone pane", func(t *testing.T) {
		pane := tmux.Pane{ID: "%1", Dead: true}
		wantErr := errors.New("tmux socket timeout")
		stub := &stopTmuxStub{listErr: wantErr}
		w := TmuxWindows{Tmux: stub}
		if err := w.StopPane(pane); !errors.Is(err, wantErr) {
			t.Errorf("StopPane error = %v, want %v", err, wantErr)
		}
		if err := w.EndPane(pane); !errors.Is(err, wantErr) {
			t.Errorf("EndPane error = %v, want %v", err, wantErr)
		}
	})

	t.Run("kill error propagated when post-kill re-check fails", func(t *testing.T) {
		pane := tmux.Pane{ID: "%1", Dead: true}
		wantKill := errors.New("kill failed")
		recheckErr := errors.New("list failed")
		for _, op := range []struct {
			name string
			fn   func(TmuxWindows, tmux.Pane) error
		}{
			{"StopPane", TmuxWindows.StopPane},
			{"EndPane", TmuxWindows.EndPane},
		} {
			calls := 0
			stub := &stopTmuxStub{
				killErr: wantKill,
				listFn: func() ([]tmux.Pane, error) {
					calls++
					if calls == 1 {
						return []tmux.Pane{pane}, nil
					}
					return nil, recheckErr
				},
			}
			if err := op.fn(TmuxWindows{Tmux: stub}, pane); !errors.Is(err, wantKill) {
				t.Errorf("%s error = %v, want %v", op.name, err, wantKill)
			}
		}
	})

	t.Run("kill error ignored when pane closed concurrently", func(t *testing.T) {
		pane := tmux.Pane{ID: "%1", Dead: true}
		for _, op := range []struct {
			name string
			fn   func(TmuxWindows, tmux.Pane) error
		}{
			{"StopPane", TmuxWindows.StopPane},
			{"EndPane", TmuxWindows.EndPane},
		} {
			calls := 0
			stub := &stopTmuxStub{
				killErr: errors.New("can't find pane: %1"),
				listFn: func() ([]tmux.Pane, error) {
					calls++
					if calls == 1 {
						return []tmux.Pane{pane}, nil
					}
					return nil, nil
				},
			}
			if err := op.fn(TmuxWindows{Tmux: stub}, pane); err != nil {
				t.Errorf("%s error = %v, want nil when pane is gone", op.name, err)
			}
		}
	})
}
