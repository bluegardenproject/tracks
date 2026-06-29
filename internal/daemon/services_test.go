package daemon

import (
	"syscall"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

func groupAlive(pgid int) bool { return syscall.Kill(-pgid, 0) == nil }

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

	st, err := srv.startService(sup, tr, config.Service{Name: "web", Cmd: "sleep 30"}, t.TempDir())
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

func TestStopPersistedServicesKillsGroup(t *testing.T) {
	srv := newServiceTestServer(t)
	tr := state.Track{ID: "trk-2", Status: state.StatusRunning}
	_ = srv.store.Put(tr)
	sup := &supervisor{trackID: tr.ID}

	// A backgrounded child ensures we're testing a real group kill.
	st, err := srv.startService(sup, tr, config.Service{Name: "svc", Cmd: "sleep 60 & sleep 60"}, t.TempDir())
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
