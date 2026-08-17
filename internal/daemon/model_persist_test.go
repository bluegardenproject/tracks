package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

// transcriptFixture points the usage lookup at a temp CLAUDE_CONFIG_DIR
// and returns the transcript path for the given session, so tests can
// drive the real refreshUsage path instead of re-implementing it.
func transcriptFixture(t *testing.T, sessionID string) string {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	dir := filepath.Join(configDir, "projects", "-tmp-wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, sessionID+".jsonl")
}

func writeLines(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// turn is an assistant line that bills tokens.
func turn(ts, model string) string {
	return `{"type":"assistant","isSidechain":false,"timestamp":"` + ts +
		`","requestId":"` + ts + `","message":{"model":"` + model +
		`","usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"
}

// modelOnly is an assistant line naming a model but billing nothing —
// which is how a scan can see a new model with the totals unchanged.
func modelOnly(ts, model string) string {
	return `{"type":"assistant","isSidechain":false,"timestamp":"` + ts +
		`","message":{"model":"` + model + `"}}` + "\n"
}

func trackFor(t *testing.T, srv *Server, sessionID string) (state.Track, *supervisor) {
	t.Helper()
	tr := state.Track{
		ID:        "trk-model",
		Status:    state.StatusRunning,
		SessionID: sessionID,
		Repos:     []state.TrackRepo{{Name: "r", Path: t.TempDir()}},
	}
	if err := srv.store.Put(tr); err != nil {
		t.Fatal(err)
	}
	return tr, &supervisor{trackID: tr.ID, done: make(chan struct{})}
}

func TestRefreshUsageRecordsTheModel(t *testing.T) {
	const session = "aaaaaaaa-0000-4000-8000-000000000001"
	path := transcriptFixture(t, session)
	writeLines(t, path, turn("2026-08-17T10:00:00.000Z", "claude-opus-5"))

	srv := newReadinessTestServer(t)
	tr, sup := trackFor(t, srv, session)

	srv.refreshUsage(sup)

	got, _ := srv.store.Get(tr.ID)
	if got.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want claude-opus-5", got.Model)
	}
	if got.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", got.Usage.InputTokens)
	}
}

// The /model case: the new model has to be persisted even though the
// billed totals are byte-identical to the previous scan.
func TestRefreshUsageFollowsAModelSwitchWithUnchangedUsage(t *testing.T) {
	const session = "aaaaaaaa-0000-4000-8000-000000000002"
	path := transcriptFixture(t, session)
	writeLines(t, path, turn("2026-08-17T10:00:00.000Z", "claude-opus-5"))

	srv := newReadinessTestServer(t)
	tr, sup := trackFor(t, srv, session)
	srv.refreshUsage(sup)

	before, _ := srv.store.Get(tr.ID)
	writeLines(t, path,
		turn("2026-08-17T10:00:00.000Z", "claude-opus-5")+
			modelOnly("2026-08-17T10:05:00.000Z", "claude-haiku-4-5"))
	srv.refreshUsage(sup)

	after, _ := srv.store.Get(tr.ID)
	if after.Model != "claude-haiku-4-5" {
		t.Errorf("Model = %q, want the switched-to model", after.Model)
	}
	if after.Usage != before.Usage {
		t.Errorf("Usage changed (%+v → %+v) — the model-only line must not bill", before.Usage, after.Usage)
	}
}

// A scan that finds no usable model must not erase one an earlier scan
// established — here every later turn is a "<synthetic>" placeholder.
func TestRefreshUsageDoesNotBlankAKnownModel(t *testing.T) {
	const session = "aaaaaaaa-0000-4000-8000-000000000003"
	path := transcriptFixture(t, session)
	writeLines(t, path, turn("2026-08-17T10:00:00.000Z", "claude-opus-5"))

	srv := newReadinessTestServer(t)
	tr, sup := trackFor(t, srv, session)
	srv.refreshUsage(sup)

	writeLines(t, path,
		turn("2026-08-17T10:00:00.000Z", "claude-opus-5")+
			turn("2026-08-17T10:06:00.000Z", "<synthetic>"))
	srv.refreshUsage(sup)

	after, _ := srv.store.Get(tr.ID)
	if after.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want the last real model to survive", after.Model)
	}
	if after.Usage.InputTokens != 20 {
		t.Errorf("InputTokens = %d, want 20 — the synthetic turn still bills", after.Usage.InputTokens)
	}
}
