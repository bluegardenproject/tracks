package tmux_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
)

// A pane's styles come with its capture, and a window no client shows
// had no key.
func TestCapturePaneStyledAndLastKey(t *testing.T) {
	socket := tmuxtest.Socket(t)
	if _, err := tmux.InstalledVersion(); err != nil {
		t.Skip(err)
	}
	c := tmux.New(socket)
	if err := c.NewSession("/dev/null", "s", "w", `printf '\033[2mdim\033[0m plain\n'; sleep 30`); err != nil {
		t.Fatal(err)
	}
	panes, err := c.ListSessionPanes("s")
	if err != nil || len(panes) != 1 {
		t.Fatalf("panes %+v, %v", panes, err)
	}
	var styled string
	for range 50 {
		if styled, err = c.CapturePaneStyled(panes[0].ID); err == nil && strings.Contains(styled, "plain") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(styled, "\x1b[2mdim") || !strings.Contains(styled, "plain") {
		t.Errorf("styled capture %q, want the dim escape kept", styled)
	}
	if plain, _ := c.CapturePane(panes[0].ID); strings.Contains(plain, "\x1b") {
		t.Errorf("plain capture %q kept escapes", plain)
	}
	if last, err := c.LastKey(panes[0].Window); err != nil || !last.IsZero() {
		t.Errorf("LastKey of a window no client shows = %v, %v", last, err)
	}
}
