package cli

import (
	"bytes"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tracks"
)

func TestPrintSetup(t *testing.T) {
	states := []tracks.SetupRepo{
		{Repo: "api", State: tracks.SetupDone},
		{Repo: "web", State: tracks.SetupFailed, Code: 1},
		{Repo: "app", State: tracks.SetupRunning},
	}
	var out bytes.Buffer
	if err := printSetup(&out, states, false); err != nil {
		t.Errorf("without waiting, a failure isn't an error: %v", err)
	}
	want := "api: done\nweb: failed (exit 1); its pane shows why\napp: running in its pane\n"
	if out.String() != want {
		t.Errorf("printed %q, want %q", out.String(), want)
	}
	if err := printSetup(&bytes.Buffer{}, states, true); err == nil || err.Error() != "the setup failed in web" {
		t.Errorf("after waiting: %v; want the failure named", err)
	}
	out.Reset()
	if err := printSetup(&out, nil, true); err != nil || out.String() != "This track's repos have no setup.\n" {
		t.Errorf("no setups: %q, %v", out.String(), err)
	}
}
