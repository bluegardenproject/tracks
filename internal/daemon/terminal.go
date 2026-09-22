package daemon

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/agent"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

func (s *Server) handleTerminal(raw json.RawMessage) Response {
	var p TerminalParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}

	t, found := s.store.Get(p.TrackID)
	if !found {
		return fail("track not found: " + p.TrackID)
	}
	if t.Status.IsTerminal() {
		return fail("track is no longer active")
	}
	if t.Kind.Worktreeless() || len(t.Repos) == 0 {
		return fail("track has no worktree to open a terminal in")
	}

	s.mu.Lock()
	sup := s.supervisors[p.TrackID]
	s.mu.Unlock()
	if sup == nil {
		return fail("no running supervisor for track " + p.TrackID)
	}

	opened, err := s.openTerminalPane(sup, t)
	if err != nil {
		return fail("open terminal: " + err.Error())
	}
	if !t.OpenTerminal {
		_, found, err := s.store.Update(t.ID, func(t *state.Track) bool {
			t.OpenTerminal = true
			return true
		})
		if err != nil {
			return fail("persist terminal preference: " + err.Error())
		}
		if !found {
			return fail("track disappeared while opening terminal")
		}
	}
	return ok(TerminalResult{Opened: opened})
}

func (s *Server) openTerminalPane(sup *supervisor, t state.Track) (bool, error) {
	if len(t.Repos) == 0 {
		return false, errors.New("track has no worktree")
	}

	tm := tmux.New()
	sup.paneMu.Lock()
	defer sup.paneMu.Unlock()

	if sup.terminalPane != "" {
		live, err := tm.HasPane(sup.terminalPane)
		if err != nil {
			return false, err
		}
		if live {
			return false, nil
		}
		_ = tm.KillPane(sup.terminalPane)
		removeSidePaneLocked(sup, sup.terminalPane)
		sup.terminalPane = ""
	}

	binDir := ""
	if s.exePath != "" {
		binDir = filepath.Dir(s.exePath)
	}
	command := agent.Wrapper{
		TrackID:   t.ID,
		SocketDir: s.socketDir,
		BinDir:    binDir,
	}.Command("")
	paneID, _, err := splitSidePaneLocked(
		tm,
		s.config().Tmux.SessionName,
		sup,
		command,
		t.Repos[0].Path,
	)
	if err != nil {
		return false, err
	}
	_ = tm.SetPaneTitle(paneID, "terminal")
	sup.terminalPane = paneID
	return true, nil
}

func splitSidePaneLocked(tm *tmux.Client, session string, sup *supervisor, command, worktree string) (string, int, error) {
	live := sup.sidePanes[:0]
	for _, paneID := range sup.sidePanes {
		exists, err := tm.HasPane(paneID)
		if err != nil {
			return "", 0, err
		}
		if exists {
			live = append(live, paneID)
		} else {
			_ = tm.KillPane(paneID)
		}
	}
	sup.sidePanes = live

	var (
		paneID  string
		panePID int
		err     error
	)
	if len(sup.sidePanes) == 0 {
		paneID, panePID, err = tm.SplitWindowRight(session, sup.windowName, command, worktree, 30)
	} else {
		paneID, panePID, err = tm.SplitPaneDown(sup.sidePanes[len(sup.sidePanes)-1], command, worktree)
	}
	if err == nil {
		sup.sidePanes = append(sup.sidePanes, paneID)
	}
	return paneID, panePID, err
}

func removeSidePaneLocked(sup *supervisor, paneID string) {
	for i, id := range sup.sidePanes {
		if id == paneID {
			sup.sidePanes = append(sup.sidePanes[:i], sup.sidePanes[i+1:]...)
			return
		}
	}
}
