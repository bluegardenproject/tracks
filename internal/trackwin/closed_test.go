package trackwin_test

import (
	"errors"
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

func (c *closing) ListPanes(string) ([]tmux.Pane, error) {
	panes := []tmux.Pane{{ID: "%1", Role: trackwin.RoleAgent, Height: 40}}
	if !c.listed {
		panes = append(panes, tmux.Pane{ID: "%2", Role: trackwin.RoleSetup, Left: 80, Height: 40})
	}
	c.listed = true
	return panes, nil
}

func (c *closing) SplitPane(target string, _ tmux.Split, _ int, _, _ string) (string, error) {
	c.splits = append(c.splits, target)
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
	if want := []string{"%2", "%1"}; !slices.Equal(c.splits, want) {
		t.Errorf("split below %v; want %v", c.splits, want)
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
