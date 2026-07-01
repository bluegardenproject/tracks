package daemon

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

func groupAlive(pgid int) bool { return syscall.Kill(-pgid, 0) == nil }

func readFileContent(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// newServiceTestServer builds a Server backed by a memory store with its
// state dir under a temp dir, so service logs have somewhere to land.
func newServiceTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.StateDir = t.TempDir()
	return NewServer(cfg, state.NewMemoryStore(), "test")
}

func TestStartServicePersistsAndRuns(t *testing.T) {
	srv := newServiceTestServer(t)
	tr := state.Track{ID: "trk-1", Status: state.StatusRunning, Ports: map[string]int{"web": 20001}}
	if err := srv.store.Put(tr); err != nil {
		t.Fatal(err)
	}
	sup := &supervisor{trackID: tr.ID}

	st, err := srv.startService(context.Background(), sup, tr, config.Service{Name: "web", Cmd: "sleep 30"}, t.TempDir())
	if err != nil {
		t.Fatalf("startService: %v", err)
	}
	defer stopPersistedServices([]state.ServiceState{st}, true)

	if st.Status != state.ServiceRunning || st.PGID <= 0 {
		t.Fatalf("unexpected service state: %+v", st)
	}
	if st.Port != 20001 {
		t.Errorf("port not recorded: %d", st.Port)
	}
	if !groupAlive(st.PGID) {
		t.Error("service group not alive after start")
	}
	// Persisted on the track.
	got, _ := srv.store.Get(tr.ID)
	if len(got.Services) != 1 || got.Services[0].Name != "web" {
		t.Errorf("service not persisted on track: %+v", got.Services)
	}
}

func TestStartServiceWaitsForReadinessAndRunsHooks(t *testing.T) {
	srv := newServiceTestServer(t)
	tr := state.Track{ID: "trk-ready", Status: state.StatusRunning}
	if err := srv.store.Put(tr); err != nil {
		t.Fatal(err)
	}
	sup := &supervisor{trackID: tr.ID}

	// The service announces readiness via a log line; a post_start hook
	// appends a marker to the same log once ready.
	svc := config.Service{
		Name:      "web",
		Cmd:       "echo LISTENING; sleep 30",
		Ready:     config.ReadyProbe{LogRegex: "LISTENING"},
		PostStart: []string{"echo POSTSTART_RAN"},
	}
	st, err := srv.startService(context.Background(), sup, tr, svc, t.TempDir())
	if err != nil {
		t.Fatalf("startService: %v", err)
	}
	defer stopPersistedServices([]state.ServiceState{st}, true)

	if st.Status != state.ServiceReady {
		t.Errorf("expected ServiceReady after probe passed, got %q", st.Status)
	}
	// The probe passing and the post_start hook are both reflected in the log.
	got := readFileContent(t, st.LogPath)
	if !strings.Contains(got, "LISTENING") || !strings.Contains(got, "POSTSTART_RAN") {
		t.Errorf("log missing readiness or post_start output: %q", got)
	}
	// Persisted status matches.
	stored, _ := srv.store.Get(tr.ID)
	if len(stored.Services) != 1 || stored.Services[0].Status != state.ServiceReady {
		t.Errorf("persisted service not ready: %+v", stored.Services)
	}
}

func TestStartServicePreStartFailureAborts(t *testing.T) {
	srv := newServiceTestServer(t)
	tr := state.Track{ID: "trk-pre", Status: state.StatusRunning}
	_ = srv.store.Put(tr)
	sup := &supervisor{trackID: tr.ID}

	svc := config.Service{Name: "web", Cmd: "sleep 30", PreStart: []string{"false"}}
	_, err := srv.startService(context.Background(), sup, tr, svc, t.TempDir())
	if err == nil {
		t.Fatal("expected pre_start failure to abort start")
	}
	// Nothing should have been launched or persisted.
	stored, _ := srv.store.Get(tr.ID)
	if len(stored.Services) != 0 {
		t.Errorf("no service should be persisted after pre_start failure: %+v", stored.Services)
	}
}

func TestStopPersistedServicesKillsGroup(t *testing.T) {
	srv := newServiceTestServer(t)
	tr := state.Track{ID: "trk-2", Status: state.StatusRunning}
	_ = srv.store.Put(tr)
	sup := &supervisor{trackID: tr.ID}

	// A backgrounded child ensures we're testing a real group kill.
	st, err := srv.startService(context.Background(), sup, tr, config.Service{Name: "svc", Cmd: "sleep 60 & sleep 60"}, t.TempDir())
	if err != nil {
		t.Fatalf("startService: %v", err)
	}
	if !groupAlive(st.PGID) {
		t.Fatal("group should be alive")
	}

	stopped := stopPersistedServices([]state.ServiceState{st}, true)
	if stopped[0].Status != state.ServiceStopped {
		t.Errorf("status not marked stopped: %v", stopped[0].Status)
	}
	// Give the group a beat to fully exit.
	time.Sleep(100 * time.Millisecond)
	if groupAlive(st.PGID) {
		t.Error("service group still alive after stopPersistedServices — leak")
	}
}

func TestUpsertService(t *testing.T) {
	list := []state.ServiceState{{Name: "a", Port: 1}}
	list = upsertService(list, state.ServiceState{Name: "b", Port: 2})
	if len(list) != 2 {
		t.Fatalf("expected append, got %v", list)
	}
	list = upsertService(list, state.ServiceState{Name: "a", Port: 9})
	if len(list) != 2 || list[0].Port != 9 {
		t.Errorf("expected in-place replace, got %v", list)
	}
}
