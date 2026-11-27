package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

func TestWithID(t *testing.T) {
	for in, want := range map[string]string{
		``:                              `{"id":"a"}`,
		`null`:                          `{"id":"a"}`,
		`{}`:                            `{"id":"a"}`,
		`{"id":"b","to":"reviewer"}`:    `{"id":"a","to":"reviewer"}`,
		`{"to":"track","kind":"event"}`: `{"id":"a","kind":"event","to":"track"}`,
	} {
		got, err := withID(json.RawMessage(in), "a")
		if err != nil || string(got) != want {
			t.Errorf("withID(%s) = %s, %v; want %s", in, got, err, want)
		}
	}
	if _, err := withID(json.RawMessage(`[1]`), "a"); err == nil {
		t.Error("params that aren't an object should be an error")
	}
}

func TestAnAgentsTokenOpensItsOwnTrackOnly(t *testing.T) {
	c, _ := config(t)
	ctx := context.Background()
	st := c.Tracks.Store
	for _, id := range []string{"a", "b"} {
		if err := st.AddTrack(ctx, track.Track{ID: id, Kind: track.Work, Name: id, Engine: "claude", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	token := agents.NewToken()
	if err := st.SetAgentToken(ctx, "a", agents.HashToken(token)); err != nil {
		t.Fatal(err)
	}
	for id, body := range map[string]string{"a": "For a.", "b": "For b."} {
		if _, err := st.AddRelayMessage(ctx, id, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	done := start(t, c)
	t.Cleanup(func() {
		_ = rpc.Client{Socket: c.Paths.Socket}.Shutdown(ctx)
		_ = wait(t, done)
	})

	agent := rpc.Client{Socket: c.Paths.Socket, Token: token}
	if id, err := agent.Agent(ctx); err != nil || id != "a" {
		t.Errorf("Agent = %q, %v; want a", id, err)
	}
	// Whatever track it names, an agent reads its own.
	msgs, err := agent.RelayRead(ctx, "b")
	if err != nil || len(msgs) != 1 || msgs[0].Body != "For a." {
		t.Errorf("RelayRead naming b = %+v, %v; want a's message", msgs, err)
	}

	var p tracks.Problem
	if _, err := agent.List(ctx); !errors.As(err, &p) || !strings.Contains(err.Error(), "doesn't open list") {
		t.Errorf("List with a token: %v, want it refused", err)
	}
	stranger := rpc.Client{Socket: c.Paths.Socket, Token: agents.NewToken()}
	if _, err := stranger.RelayRead(ctx, "b"); !errors.As(err, &p) || !strings.Contains(err.Error(), "doesn't know this agent's token") {
		t.Errorf("an unknown token: %v, want it refused", err)
	}
	if msgs, err := (rpc.Client{Socket: c.Paths.Socket}).RelayRead(ctx, "b"); err != nil || len(msgs) != 1 || msgs[0].Body != "For b." {
		t.Errorf("without a token, RelayRead(b) = %+v, %v; want b's message, as before", msgs, err)
	}
	if _, err := (rpc.Client{Socket: c.Paths.Socket}).Agent(ctx); !errors.As(err, &p) {
		t.Errorf("Agent without a token: %v, want a problem", err)
	}
}
