package tmux_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
)

// ClientSession is each client's own session, with clients on two
// sessions, as an overlay's and the one behind it are.
func TestClientSession(t *testing.T) {
	socket := tmuxtest.Socket(t)
	c := tmux.New(socket)
	if err := c.NewSession("/dev/null", "tracks", "Tracks", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NewKeptSession("tracks-train-t1-reviewer-1", "reviewer", "", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{"tracks", "tracks-train-t1-reviewer-1"} {
		client := exec.Command("tmux", "-L", socket, "-C", "attach", "-t", "="+session)
		stdin, err := client.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = stdin.Close(); _ = client.Wait() })
	}
	// Both attached before either is checked: the client used last is
	// then one of the two, whichever ClientSession is asked about.
	clientOf := map[string]string{}
	for _, session := range []string{"tracks", "tracks-train-t1-reviewer-1"} {
		var clients []string
		for deadline := time.Now().Add(5 * time.Second); len(clients) == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			clients, _ = c.SessionClients(session)
		}
		if len(clients) != 1 {
			t.Fatalf("%s has clients %v, want one", session, clients)
		}
		clientOf[session] = clients[0]
	}
	for session, client := range clientOf {
		if got, err := c.ClientSession(client); err != nil || got != session {
			t.Errorf("ClientSession(%s) = %q, %v; want %s", client, got, err, session)
		}
	}
	if _, err := c.ClientSession("/dev/nope"); err == nil {
		t.Error("a client that isn't there has a session")
	}
}
