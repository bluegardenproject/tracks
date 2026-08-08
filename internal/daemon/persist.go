package daemon

import (
	"github.com/bluegardenproject/tracks/internal/dlog"
	"github.com/bluegardenproject/tracks/internal/state"
)

// The three helpers below exist for the store writes that have no caller
// to report an error to — a supervisor tick, the shutdown sweep, a
// recovery pass, a best-effort cleanup. Those sites used to discard the
// error entirely (`_ = s.store.Put(t)`), which meant a full disk or a
// read-only state dir diverged the daemon from its own state file in
// silence, and the divergence only surfaced as inexplicable track states
// after the next restart. Logging is all we can do there — but it is the
// difference between a bug report that names the cause and one that
// doesn't.
//
// Handlers that CAN report the failure to their caller still call
// s.store directly and return the error; don't convert those.

// persist writes t back to the store, logging rather than discarding a
// failure. what names the write, so the log line says what was lost.
func (s *Server) persist(t state.Track, what string) {
	if err := s.store.Put(t); err != nil {
		dlog.Printf("persist %s for track %s: %v", what, t.ID, err)
	}
}

// update read-modify-writes a track under the store's lock, logging a
// failed flush. Returns the resulting track and whether it existed.
func (s *Server) update(id, what string, mutate func(*state.Track) bool) (state.Track, bool) {
	t, found, err := s.store.Update(id, mutate)
	if err != nil {
		dlog.Printf("persist %s for track %s: %v", what, id, err)
	}
	return t, found
}

// forget removes a track, logging a failed flush. Reports whether the
// track was removed *and* the removal reached disk — handlePruneCompleted
// counts the result, and a track whose delete failed to flush is still
// there after a restart, so counting it would overstate the sweep.
func (s *Server) forget(id, what string) bool {
	existed, err := s.store.Delete(id)
	if err != nil {
		dlog.Printf("delete track %s (%s): %v", id, what, err)
		return false
	}
	return existed
}
