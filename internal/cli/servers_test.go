package cli

import (
	"bytes"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tracks"
)

func TestPrintServers(t *testing.T) {
	var out bytes.Buffer
	err := printServers(&out, []tracks.Server{
		{Repo: "api", Name: "web", Port: 20001, State: tracks.ServerReady},
		{Repo: "api", Name: "app", State: tracks.ServerStarting},
		{Port: 5173, Type: "vite", State: tracks.ServerReady},
		{Repo: "api", Name: "sb", State: tracks.ServerCrashed, Code: 2},
		{Repo: "web", Name: "docs", State: tracks.ServerStopped},
	})
	want := "api/web: ready on http://localhost:20001\napi/app: starting\n:5173 (vite, not started by Tracks): ready on http://localhost:5173\napi/sb: crashed (exit 2); its pane shows why\nweb/docs: stopped\n"
	if err != nil || out.String() != want {
		t.Errorf("printed %q, %v; want %q", out.String(), err, want)
	}
}

func TestServerURL(t *testing.T) {
	servers := []tracks.Server{
		{Repo: "api", Name: "web", Port: 20001, State: tracks.ServerReady},
		{Repo: "api", Name: "app", State: tracks.ServerStarting},
		{Port: 5173, Type: "vite", State: tracks.ServerReady},
	}
	if url, err := serverURL(servers, "api/web"); err != nil || url != "http://localhost:20001" {
		t.Errorf("api/web: %q, %v", url, err)
	}
	if _, err := serverURL(servers, "app"); err == nil {
		t.Error("a server without a port yet has a URL")
	}
	if _, err := serverURL(servers, "vite"); err == nil {
		t.Error("an unnamed server was found by its type")
	}
	twice := append(servers, tracks.Server{Repo: "shop", Name: "web", Port: 20011, State: tracks.ServerReady})
	if _, err := serverURL(twice, "web"); err == nil {
		t.Error("web in two repos: want it to ask for repo/server")
	}
	if url, _ := serverURL(twice, "shop/web"); url != "http://localhost:20011" {
		t.Errorf("shop/web = %q", url)
	}
}

func TestPrintAllServers(t *testing.T) {
	var out bytes.Buffer
	err := printAllServers(&out, []tracks.Server{
		{Track: "a", TrackName: "fix-login", Repo: "api", Name: "web", Port: 20001, State: tracks.ServerReady},
		{Track: "a", TrackName: "fix-login", Port: 5173, Type: "vite", State: tracks.ServerReady},
		{Track: "b", TrackName: "new-nav", Repo: "web", Name: "docs", State: tracks.ServerStopped},
	})
	want := "fix-login\n  api/web: ready on http://localhost:20001\n  :5173 (vite, not started by Tracks): ready on http://localhost:5173\nnew-nav\n  web/docs: stopped\n"
	if err != nil || out.String() != want {
		t.Errorf("printed %q, %v; want %q", out.String(), err, want)
	}
}
