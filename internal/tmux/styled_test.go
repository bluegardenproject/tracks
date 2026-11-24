package tmux_test

import (
	"os/exec"
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

// A paste lands in the pane whole, and its buffer goes after it.
func TestPasteText(t *testing.T) {
	socket := tmuxtest.Socket(t)
	if _, err := tmux.InstalledVersion(); err != nil {
		t.Skip(err)
	}
	c := tmux.New(socket)
	if err := c.NewSession("/dev/null", "s", "w", "cat"); err != nil {
		t.Fatal(err)
	}
	panes, err := c.ListSessionPanes("s")
	if err != nil || len(panes) != 1 {
		t.Fatalf("panes %+v, %v", panes, err)
	}
	text := "1 message from @reviewer-527192 on the relay: run `tracks relay read`."
	if err := c.PasteText(panes[0].ID, text); err != nil {
		t.Fatal(err)
	}
	var screen string
	for range 50 {
		if screen, _ = c.CapturePane(panes[0].ID); strings.Contains(screen, text) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(screen, text) {
		t.Errorf("the pane shows %q, want the pasted text", screen)
	}
	if out, err := exec.Command("tmux", "-L", socket, "list-buffers").Output(); err != nil || strings.Contains(string(out), "tracks-paste") {
		t.Errorf("the paste's buffer stayed: %q, %v", out, err)
	}
}
