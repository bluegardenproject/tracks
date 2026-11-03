package trackwin_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// vanishing is a tmux whose new panes end at once: labelling one fails,
// and it's no longer listed.
type vanishing struct{ trackwin.Tmux }

func (vanishing) ListPanes(string) ([]tmux.Pane, error) {
	return []tmux.Pane{{ID: "%1", Role: trackwin.RoleAgent, Height: 40}}, nil
}

func (vanishing) SplitPane(string, tmux.Split, int, string, string) (string, error) { return "%9", nil }

func (vanishing) SetPaneOption(pane, _, _ string) error {
	if pane == "%9" {
		return errors.New("no such pane: %9")
	}
	return nil
}

func (vanishing) ResizePaneHeight(string, int) error { return nil }
func (vanishing) SendText(string, string) error      { return nil }
func (vanishing) SendKey(string, string) error       { return nil }

func (vanishing) WindowOption(string, string) (string, error) { return "/tmp", nil }

// closing is a tmux whose column holds a finished setup's pane, %2,
// which closes once it's listed: splitting below it fails.
type closing struct {
	vanishing
	listed bool
	splits []string
}

// splitAt is where a pane was split, and how.
func splitAt(target string, dir tmux.Split) string { return fmt.Sprintf("%s %v", target, dir) }

func (c *closing) ListPanes(string) ([]tmux.Pane, error) {
	panes := []tmux.Pane{{ID: "%1", Role: trackwin.RoleAgent, Height: 40}}
	if !c.listed {
		panes = append(panes, tmux.Pane{ID: "%2", Role: trackwin.RoleSetup, Left: 80, Height: 40})
	}
	c.listed = true
	return panes, nil
}

func (c *closing) SplitPane(target string, dir tmux.Split, _ int, _, _ string) (string, error) {
	c.splits = append(c.splits, splitAt(target, dir))
	if target == "%2" {
		return "", errors.New("can't find pane: %2")
	}
	return "%3", nil
}

func (*closing) SetPaneOption(string, string, string) error { return nil }

// TestTheColumnsLastPaneClosing: a pane added while the column's last
// pane closes goes beside the agent instead of failing.
func TestTheColumnsLastPaneClosing(t *testing.T) {
	c := &closing{}
	pane, err := trackwin.AddSetup(c, "@1", "/tmp", trackwin.Process{Title: "setup · web", Command: "exit 3"})
	if err != nil || pane != "%3" {
		t.Fatalf("AddSetup = %q, %v; want %%3 after splitting again", pane, err)
	}
	if want := []string{splitAt("%2", tmux.Below), splitAt("%1", tmux.Right)}; !slices.Equal(c.splits, want) {
		t.Errorf("splits %v; want %v", c.splits, want)
	}
}

// shrinking is a tmux whose column's top pane, %2, closes while the
// column is evened out after a new pane, %4, was added below %3.
type shrinking struct {
	vanishing
	closed  bool
	resized []string
}

func (s *shrinking) ListPanes(string) ([]tmux.Pane, error) {
	panes := []tmux.Pane{{ID: "%1", Role: trackwin.RoleAgent, Height: 40}}
	if !s.closed {
		panes = append(panes, tmux.Pane{ID: "%2", Role: trackwin.RoleSetup, Left: 80, Top: 0, Height: 13})
	}
	return append(panes,
		tmux.Pane{ID: "%3", Role: trackwin.RoleSetup, Left: 80, Top: 14, Height: 13},
		tmux.Pane{ID: "%4", Role: trackwin.RoleSetup, Left: 80, Top: 28, Height: 12}), nil
}

func (*shrinking) SplitPane(string, tmux.Split, int, string, string) (string, error) {
	return "%4", nil
}
func (*shrinking) SetPaneOption(string, string, string) error { return nil }

func (s *shrinking) ResizePaneHeight(pane string, _ int) error {
	if pane == "%2" {
		s.closed = true
		return errors.New("can't find pane: %2")
	}
	s.resized = append(s.resized, pane)
	return nil
}

// TestAColumnPaneClosingWhileEvened: a pane that closed while the
// column was evened out is left out, and the rest are evened.
func TestAColumnPaneClosingWhileEvened(t *testing.T) {
	s := &shrinking{}
	if _, err := trackwin.AddSetup(s, "@1", "/tmp", trackwin.Process{Title: "setup · web", Command: "exit 3"}); err != nil {
		t.Fatalf("AddSetup: %v; want the closed pane left out", err)
	}
	if want := []string{"%3"}; !slices.Equal(s.resized, want) {
		t.Errorf("resized %v; want %v", s.resized, want)
	}
}

// TestAPaneThatClosedAtOnce: a setup fast enough to finish before its
// pane is labelled isn't an error; a terminal's pane closing is.
func TestAPaneThatClosedAtOnce(t *testing.T) {
	if _, err := trackwin.AddSetup(vanishing{}, "@1", "/tmp", trackwin.Process{Title: "setup · api", Command: "true"}); err != nil {
		t.Errorf("AddSetup: %v; want a setup that already finished taken as started", err)
	}
	if _, err := trackwin.AddTerminal(vanishing{}, "@1"); !errors.Is(err, trackwin.ErrPaneClosed) {
		t.Errorf("a terminal whose pane closed at once: %v; want ErrPaneClosed", err)
	}
}
