package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/services"
	"github.com/bluegardenproject/tracks/internal/state"
)

// newReadinessTestServer is newServiceTestServer with notifications off —
// the readiness paths all notify, and a test must not fire real desktop
// notifications or write to /dev/tty.
func newReadinessTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.StateDir = t.TempDir()
	cfg.Notify = config.Notify{}
	srv := NewServer(cfg, state.NewMemoryStore(), "test")
	srv.readyTimeout = 300 * time.Millisecond
	return srv
}

// listenOnFreePort opens a real listener so a port probe can succeed, and
// returns its port.
func listenOnFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

func trackWithService(t *testing.T, srv *Server, id string, st state.ServiceState) state.Track {
	t.Helper()
	tr := state.Track{ID: id, Status: state.StatusRunning, Services: []state.ServiceState{st}}
	if err := srv.store.Put(tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

func serviceStatus(t *testing.T, srv *Server, trackID, name string) state.ServiceStatus {
	t.Helper()
	tr, ok := srv.store.Get(trackID)
	if !ok {
		t.Fatalf("track %s missing", trackID)
	}
	for _, sv := range tr.Services {
		if sv.Name == name {
			return sv.Status
		}
	}
	t.Fatalf("service %s missing from track %s", name, trackID)
	return ""
}

func TestRenderProbeResolvesTrackPort(t *testing.T) {
	data := services.NewTemplateData("trk", "/work", map[string]int{"live-app": 20007})
	probe, err := renderProbe(config.ReadyProbe{Port: `{{.Port "live-app"}}`}, data)
	if err != nil {
		t.Fatalf("renderProbe: %v", err)
	}
	if probe.Port != "20007" {
		t.Errorf("Port = %q, want 20007", probe.Port)
	}
}

func TestRenderProbeZeroStaysZero(t *testing.T) {
	probe, err := renderProbe(config.ReadyProbe{}, services.NewTemplateData("trk", "/work", nil))
	if err != nil {
		t.Fatalf("renderProbe: %v", err)
	}
	if !probe.IsZero() {
		t.Errorf("expected zero probe, got %+v", probe)
	}
}

func TestRenderProbeUnknownPortErrors(t *testing.T) {
	data := services.NewTemplateData("trk", "/work", map[string]int{"other": 1})
	if _, err := renderProbe(config.ReadyProbe{Port: `{{.Port "live-app"}}`}, data); err == nil {
		t.Fatal("expected an error for a port the track never allocated")
	}
}

func TestWatchServiceReadyMarksReady(t *testing.T) {
	srv := newReadinessTestServer(t)
	port := listenOnFreePort(t)
	tr := trackWithService(t, srv, "trk-ready", state.ServiceState{
		Name: "web", Status: state.ServiceStarting, PGID: 1, Port: port,
	})
	sup := &supervisor{trackID: tr.ID, done: make(chan struct{})}

	svc := config.Service{Name: "web"}
	probe := services.Probe{Port: strconv.Itoa(port)}
	srv.watchServiceReady(sup, tr.ID, svc, probe, t.TempDir(), filepath.Join(t.TempDir(), "web.log"), map[string]int{"web": port})

	if got := serviceStatus(t, srv, tr.ID, "web"); got != state.ServiceReady {
		t.Errorf("status = %q, want ready", got)
	}
}

func TestWatchServiceReadyMarksFailedOnTimeout(t *testing.T) {
	srv := newReadinessTestServer(t)
	tr := trackWithService(t, srv, "trk-fail", state.ServiceState{
		Name: "web", Status: state.ServiceStarting, PGID: 1, Port: 1,
	})
	sup := &supervisor{trackID: tr.ID, done: make(chan struct{})}

	// Port 1 is not bound by anything we can reach, so the probe times out.
	srv.watchServiceReady(sup, tr.ID, config.Service{Name: "web"}, services.Probe{Port: "1"},
		t.TempDir(), filepath.Join(t.TempDir(), "web.log"), map[string]int{"web": 1})

	if got := serviceStatus(t, srv, tr.ID, "web"); got != state.ServiceFailed {
		t.Errorf("status = %q, want failed", got)
	}
}

// A daemon shutdown (or track end) closes sup.done. The service's end
// state then belongs to teardown, so the watcher must leave it alone —
// marking it failed here would take it out of NeedsTeardown and leak the
// pane process.
func TestWatchServiceReadyLeavesStatusOnTeardown(t *testing.T) {
	srv := newReadinessTestServer(t)
	srv.readyTimeout = 10 * time.Second // long enough that only the cancel ends it
	tr := trackWithService(t, srv, "trk-cancel", state.ServiceState{
		Name: "web", Status: state.ServiceStarting, PGID: 1, Port: 1,
	})
	done := make(chan struct{})
	sup := &supervisor{trackID: tr.ID, done: done}

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(done)
	}()
	srv.watchServiceReady(sup, tr.ID, config.Service{Name: "web"}, services.Probe{Port: "1"},
		t.TempDir(), filepath.Join(t.TempDir(), "web.log"), map[string]int{"web": 1})

	if got := serviceStatus(t, srv, tr.ID, "web"); got != state.ServiceStarting {
		t.Errorf("status = %q, want it left at starting for teardown", got)
	}
}

func TestWatchServiceReadyRunsPostStartHooks(t *testing.T) {
	srv := newReadinessTestServer(t)
	port := listenOnFreePort(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "hook-ran")
	tr := trackWithService(t, srv, "trk-hooks", state.ServiceState{
		Name: "web", Status: state.ServiceStarting, PGID: 1, Port: port,
	})
	sup := &supervisor{trackID: tr.ID, done: make(chan struct{})}

	svc := config.Service{Name: "web", PostStart: []string{"touch " + marker}}
	srv.watchServiceReady(sup, tr.ID, svc, services.Probe{Port: strconv.Itoa(port)},
		dir, filepath.Join(dir, "web.log"), map[string]int{"web": port})

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("post_start hook did not run: %v", err)
	}
	if got := serviceStatus(t, srv, tr.ID, "web"); got != state.ServiceReady {
		t.Errorf("status = %q, want ready", got)
	}
}

func TestWatchServiceReadyFailsWhenPostStartFails(t *testing.T) {
	srv := newReadinessTestServer(t)
	port := listenOnFreePort(t)
	dir := t.TempDir()
	tr := trackWithService(t, srv, "trk-hookfail", state.ServiceState{
		Name: "web", Status: state.ServiceStarting, PGID: 1, Port: port,
	})
	sup := &supervisor{trackID: tr.ID, done: make(chan struct{})}

	svc := config.Service{Name: "web", PostStart: []string{"exit 3"}}
	srv.watchServiceReady(sup, tr.ID, svc, services.Probe{Port: strconv.Itoa(port)},
		dir, filepath.Join(dir, "web.log"), map[string]int{"web": port})

	if got := serviceStatus(t, srv, tr.ID, "web"); got != state.ServiceFailed {
		t.Errorf("status = %q, want failed", got)
	}
}

// A probe that finishes after the user ran `tracks down` must not bring
// the service back to life.
func TestMarkServiceWillNotResurrectAStoppedService(t *testing.T) {
	srv := newReadinessTestServer(t)
	tr := trackWithService(t, srv, "trk-stopped", state.ServiceState{
		Name: "web", Status: state.ServiceStopped, PGID: 1,
	})

	if srv.markService(tr.ID, "web", state.ServiceReady) {
		t.Error("markService reported a change on a stopped service")
	}
	if got := serviceStatus(t, srv, tr.ID, "web"); got != state.ServiceStopped {
		t.Errorf("status = %q, want it left stopped", got)
	}
}

func TestMarkServiceSetsExitedAtOnFailure(t *testing.T) {
	srv := newReadinessTestServer(t)
	tr := trackWithService(t, srv, "trk-exit", state.ServiceState{
		Name: "web", Status: state.ServiceStarting, PGID: 1,
	})

	if !srv.markService(tr.ID, "web", state.ServiceFailed) {
		t.Fatal("markService did not apply the failure")
	}
	got, _ := srv.store.Get(tr.ID)
	if got.Services[0].ExitedAt == nil {
		t.Error("ExitedAt not set on failure")
	}
}

// A failed service keeps its pane process, so teardown must still signal
// it — otherwise the process and its port outlive the track.
func TestStopPersistedServicesTearsDownFailedWithPGID(t *testing.T) {
	pgid := spawnGroup(t, "sleep 60 & sleep 60")
	st := state.ServiceState{Name: "svc", Status: state.ServiceFailed, PGID: pgid}

	stopped := stopPersistedServices([]state.ServiceState{st}, true)

	if stopped[0].Status != state.ServiceStopped {
		t.Errorf("status = %q, want stopped", stopped[0].Status)
	}
	time.Sleep(100 * time.Millisecond)
	if groupAlive(pgid) {
		t.Error("failed service's group survived teardown — leak")
	}
}

func TestServiceOccupied(t *testing.T) {
	cases := []struct {
		name string
		svcs []state.ServiceState
		want bool
	}{
		{"unknown service", nil, false},
		{"running", []state.ServiceState{{Name: "web", Status: state.ServiceRunning}}, true},
		{"starting", []state.ServiceState{{Name: "web", Status: state.ServiceStarting}}, true},
		{"ready", []state.ServiceState{{Name: "web", Status: state.ServiceReady}}, true},
		{"stopped", []state.ServiceState{{Name: "web", Status: state.ServiceStopped, PGID: 5}}, false},
		{"failed, pane gone", []state.ServiceState{{Name: "web", Status: state.ServiceFailed}}, false},
		{"failed, pane alive", []state.ServiceState{{Name: "web", Status: state.ServiceFailed, PGID: 5}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, why := serviceOccupied(tc.svcs, "web")
			if got != tc.want {
				t.Errorf("serviceOccupied = %v (%q), want %v", got, why, tc.want)
			}
			if got && why == "" {
				t.Error("occupied service returned no explanation")
			}
		})
	}
}
