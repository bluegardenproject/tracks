package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

func TestTerminalRejectsWorktreelessTrack(t *testing.T) {
	srv := newServiceTestServer(t)
	tr := state.Track{
		ID:     "ask-track",
		Kind:   state.KindAsk,
		Status: state.StatusRunning,
	}
	if err := srv.store.Put(tr); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(TerminalParams{TrackID: tr.ID})
	resp := srv.handleTerminal(raw)
	if resp.Ok || !strings.Contains(resp.Error, "no worktree") {
		t.Fatalf("worktree-less terminal response = %+v", resp)
	}
}

func TestSidePanesShareAColumnAndStackDown(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	session := fmt.Sprintf("tracks-terminal-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	tm := tmux.New()
	if err := tm.NewSession(session, "track", "printf 'agent-pane\n'; sleep 60", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tm.KillSession(session) })

	activePane := func() string {
		t.Helper()
		out, err := exec.Command("tmux", "display-message", "-p", "-t", session+":track", "#{pane_id}").CombinedOutput()
		if err != nil {
			t.Fatalf("active pane: %v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	agentPane := activePane()

	sup := &supervisor{windowName: "track"}
	first, _, err := splitSidePaneLocked(tm, session, sup, "sleep 60", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := splitSidePaneLocked(tm, session, sup, "sleep 60", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	position := func(paneID string) (left, top int) {
		t.Helper()
		out, err := exec.Command("tmux", "display-message", "-p", "-t", paneID, "#{pane_left} #{pane_top}").CombinedOutput()
		if err != nil {
			t.Fatalf("pane position: %v: %s", err, out)
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &left, &top); err != nil {
			t.Fatalf("parse pane position %q: %v", out, err)
		}
		return left, top
	}

	firstLeft, firstTop := position(first)
	secondLeft, secondTop := position(second)
	if firstLeft != secondLeft {
		t.Errorf("side panes are in different columns: left=%d and left=%d", firstLeft, secondLeft)
	}
	if secondTop <= firstTop {
		t.Errorf("second pane is not below the first: top=%d and top=%d", firstTop, secondTop)
	}
	if got := activePane(); got != agentPane {
		t.Errorf("side-pane split selected %s; agent pane %s must stay active for supervision", got, agentPane)
	}
	if out, err := exec.Command("tmux", "select-pane", "-t", second).CombinedOutput(); err != nil {
		t.Fatalf("select side pane: %v: %s", err, out)
	}
	snapshot, err := tm.CapturePaneByID(agentPane)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot, "agent-pane") {
		t.Errorf("capturing agent pane while side pane is selected returned %q", snapshot)
	}
}
