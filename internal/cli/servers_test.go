package cli

import (
	"bytes"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tracks"
)

func TestPrintServers(t *testing.T) {
	var out bytes.Buffer
	err := printServers(&out, []tracks.Server{
		{Repo: "api", Name: "web", Port: 20001, State: tracks.ServerRunning},
		{Repo: "api", Name: "app", State: tracks.ServerRunning},
		{Repo: "api", Name: "sb", State: tracks.ServerCrashed, Code: 2},
		{Repo: "web", Name: "docs", State: tracks.ServerStopped},
	})
	want := "api/web: running on http://localhost:20001\napi/app: running\napi/sb: crashed (exit 2); its pane shows why\nweb/docs: stopped\n"
	if err != nil || out.String() != want {
		t.Errorf("printed %q, %v; want %q", out.String(), err, want)
	}
}
