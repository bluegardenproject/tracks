package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

func dispatchServer(t *testing.T) *Server {
	t.Helper()
	cfg := testConfig(t)
	cfg.Claude.Binary = "claude"
	cfg.Cursor.Binary = "agent"
	return NewServer(cfg, state.NewMemoryStore(), "test")
}

func dispatchTrack(provider state.Provider) state.Track {
	return state.Track{
		ID: "20260101-000000-abcdef", Kind: state.KindWork,
		Provider: provider, TaskPrompt: "do it",
		SessionID: "08ab06bf-8ec1-495f-90c9-b7523131b824",
		Repos:     []state.TrackRepo{{Name: "demo", Path: "/tmp/demo"}},
	}
}

// The whole point of the phase: which binary a track launches follows
// the record, not the config.
func TestSpawnForDispatchesOnTheTrackProvider(t *testing.T) {
	s := dispatchServer(t)
	for _, tc := range []struct {
		provider   state.Provider
		wantBinary string
		wantFlag   string
	}{
		{state.ProviderClaude, "claude", "--permission-mode"},
		{state.ProviderCursor, "agent", "--force"},
	} {
		cmd, _, err := s.spawnFor(dispatchTrack(tc.provider), "/tmp/sentinel", false)
		if err != nil {
			t.Fatalf("%s: %v", tc.provider, err)
		}
		if !strings.Contains(cmd, tc.wantBinary) {
			t.Errorf("%s track does not launch %s: %s", tc.provider, tc.wantBinary, cmd)
		}
		if !strings.Contains(cmd, tc.wantFlag) {
			t.Errorf("%s track is missing %s: %s", tc.provider, tc.wantFlag, cmd)
		}
	}
}

// Every record written before providers existed carries "". Those
// tracks must keep launching Claude, which is what they were created
// as — the default arm, not an explicit case.
func TestSpawnForTreatsAnEmptyProviderAsClaude(t *testing.T) {
	s := dispatchServer(t)
	cmd, _, err := s.spawnFor(dispatchTrack(""), "/tmp/sentinel", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "claude") || strings.Contains(cmd, "--force") {
		t.Errorf("a pre-provider record did not launch Claude: %s", cmd)
	}
}

// Resume is the path where getting this wrong is worst: the session id
// belongs to one binary and the other cannot resume it.
func TestSpawnForResumeDispatchesToo(t *testing.T) {
	s := dispatchServer(t)
	for provider, wantBinary := range map[state.Provider]string{
		state.ProviderClaude: "claude",
		state.ProviderCursor: "agent",
	} {
		cmd, _, err := s.spawnFor(dispatchTrack(provider), "/tmp/sentinel", true)
		if err != nil {
			t.Fatalf("%s: %v", provider, err)
		}
		if !strings.Contains(cmd, "--resume") {
			t.Errorf("%s resume lost its session: %s", provider, cmd)
		}
		if !strings.Contains(cmd, wantBinary) {
			t.Errorf("%s resume launched the wrong binary: %s", provider, cmd)
		}
		// The assertions above are satisfied by BuildOptions too —
		// Cursor passes --resume on every run, and the binary is the
		// same either way — so they do not prove the resume builder was
		// used. The absent prompt does: a resume built by BuildOptions
		// would re-send the task and the agent would redo the work.
		if strings.Contains(cmd, "do it") {
			t.Errorf("%s resume re-sends the task prompt — the resume builder was not used: %s", provider, cmd)
		}
		if strings.Contains(cmd, "--model") {
			t.Errorf("%s resume forces a model, undoing an in-pane switch: %s", provider, cmd)
		}
	}
}

// A Cursor session id comes from the network, so creation must fail
// rather than produce a track that can never be resumed.
func TestNewSessionIDFailsWhenCursorIsUnreachable(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "no-such-agent")
	s := NewServer(cfg, state.NewMemoryStore(), "test")

	if _, err := s.newSessionID(t.Context(), state.ProviderCursor); err == nil {
		t.Error("a failed create-chat should fail creation, not yield an empty id")
	}
}

// Claude's id is generated locally and must not touch the network.
func TestNewSessionIDForClaudeIsLocal(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "no-such-agent")
	s := NewServer(cfg, state.NewMemoryStore(), "test")

	id, err := s.newSessionID(t.Context(), state.ProviderClaude)
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 {
		t.Errorf("session id = %q, want a uuid", id)
	}
}

// A Cursor track's id must be the chat id the CLI returned, not a
// locally generated uuid — `agent --resume <random uuid>` is not a
// chat that exists.
func TestCursorTrackTakesItsIDFromCreateChat(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "agent")
	script := "#!/bin/sh\necho 11111111-2222-3333-4444-555555555555\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t)
	cfg.Cursor.Binary = fake
	s := NewServer(cfg, state.NewMemoryStore(), "test")

	id, err := s.newSessionID(t.Context(), state.ProviderCursor)
	if err != nil {
		t.Fatal(err)
	}
	if id != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("session id = %q, want the chat id create-chat returned", id)
	}
}

// A failed create-chat is a network failure, which is exactly the case
// the draft mechanism exists for. Before this it happened before any
// record existed, so the user lost everything they had typed.
func TestFailedChatCreationKeepsTheDraft(t *testing.T) {
	cfg := testConfig(t)
	cfg.Repos = []config.Repo{{Name: "demo", Path: "/nonexistent/demo", Base: "main"}}
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "no-such-agent")
	store := state.NewMemoryStore()
	srv := NewServer(cfg, store, "test")

	raw, err := json.Marshal(NewParams{
		Repos: []string{"demo"}, TaskPrompt: "an expensive prompt to retype",
		Kind: "work", Provider: "cursor",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp := srv.handleNew(context.Background(), raw, func(string) {}); resp.Ok {
		t.Fatal("creation should fail when the chat cannot be created")
	}

	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected the failed creation to be persisted, got %d tracks", len(tracks))
	}
	tr := tracks[0]
	if tr.Draft == nil {
		t.Fatal("no draft saved — the user would have to retype the prompt")
	}
	if tr.Draft.TaskPrompt != "an expensive prompt to retype" {
		t.Errorf("draft lost the prompt: %q", tr.Draft.TaskPrompt)
	}
	if tr.Draft.Provider != state.ProviderCursor {
		t.Errorf("draft lost the provider: %q", tr.Draft.Provider)
	}
	if !strings.Contains(tr.ErrorMsg, "cursor chat") {
		t.Errorf("error message does not say what failed: %q", tr.ErrorMsg)
	}
}
