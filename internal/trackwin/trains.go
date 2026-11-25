package trackwin

import (
	"errors"
	"strings"

	"github.com/bluegardenproject/tracks/internal/tmux"
)

// TrainPrefix starts the name of each train's session. A train runs in
// a session of its own, never among the tracks, so the overlay can
// attach a client to it.
const TrainPrefix = "tracks-train-"

// Window options of a train's window: the track it belongs to and its
// handle, for its header's buttons.
const (
	trainTrackOption  = "@tracks_train_track"
	trainHandleOption = "@tracks_train"
)

// TrainBadgeOption is the session option holding a train's status
// badge, a tmux format its header expands.
const TrainBadgeOption = "@tracks_train_badge"

// paneTrackOption names the track a train's pane belongs to.
const paneTrackOption = "@tracks_track"

// TrainTmux is what trains need from a tmux server beyond Tmux.
type TrainTmux interface {
	Tmux
	NewKeptSession(name, window, dir, command string) (string, error)
	SetSessionOption(session, name, value string) error
	ListAllPanes() ([]tmux.Pane, error)
	KillSession(session string) error
	SessionClients(session string) ([]string, error)
}

// ErrNoTrain means the track has no train in that role.
var ErrNoTrain = errors.New("no such train")

// TrainSpec describes a train's session.
type TrainSpec struct {
	Track, Handle, Role, Dir string
	Process                  Process
	// Header is the tmux format of its one-row status line, on top: the
	// overlay's header.
	Header string
}

// TrainSession is the name of the session of track's train handle.
func TrainSession(track, handle string) string { return TrainPrefix + track + "-" + handle }

// HandleOf is the handle of track's train in session, "" when session
// isn't one of track's.
func HandleOf(track, session string) string {
	if track == "" {
		return ""
	}
	h, ok := strings.CutPrefix(session, TrainSession(track, ""))
	if !ok {
		return ""
	}
	return h
}

// IsTrainSession says whether session is a train's.
func IsTrainSession(session string) bool { return strings.HasPrefix(session, TrainPrefix) }

// OpenTrain starts s's process in a session of its own and returns its
// pane. The pane stays after the process exits, until CloseTrains.
func OpenTrain(t TrainTmux, s TrainSpec) (string, error) {
	session := TrainSession(s.Track, s.Handle)
	pane, err := t.NewKeptSession(session, s.Role, s.Dir, s.Process.Command)
	if err != nil {
		return "", err
	}
	fail := func(err error) (string, error) {
		_ = t.KillSession(session)
		return "", err
	}
	if err := label(t, pane, s.Role, s.Process.Title); err != nil {
		return fail(err)
	}
	if err := t.SetPaneOption(pane, paneTrackOption, s.Track); err != nil {
		return fail(err)
	}
	for _, o := range [][2]string{{trainTrackOption, s.Track}, {trainHandleOption, s.Handle}} {
		if err := t.SetWindowOption("="+session+":", o[0], o[1]); err != nil {
			return fail(err)
		}
	}
	// Ending the train closes its overlay, whatever the user's config.
	for _, o := range [][2]string{{"status", "on"}, {"status-position", "top"}, {"status-format[0]", s.Header},
		{"detach-on-destroy", "on"}} {
		if err := t.SetSessionOption(session, o[0], o[1]); err != nil {
			return fail(err)
		}
	}
	return pane, nil
}

// Trains are the panes of track's trains, every track's for "".
func Trains(t TrainTmux, track string) ([]tmux.Pane, error) {
	panes, err := t.ListAllPanes()
	if err != nil {
		return nil, err
	}
	var out []tmux.Pane
	for _, p := range panes {
		if IsTrainSession(p.Session) && p.Track != "" && (track == "" || p.Track == track) {
			out = append(out, p)
		}
	}
	return out, nil
}

// FindTrain is track's train in role; ErrNoTrain when there's none.
func FindTrain(t TrainTmux, track, role string) (tmux.Pane, error) {
	panes, err := Trains(t, track)
	if err != nil {
		return tmux.Pane{}, err
	}
	for _, p := range panes {
		if p.Role == role {
			return p, nil
		}
	}
	return tmux.Pane{}, ErrNoTrain
}

// CloseTrains ends track's trains in role, "" for every role, with
// their sessions: an overlay on one closes.
func CloseTrains(t TrainTmux, track, role string) error {
	panes, err := Trains(t, track)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range panes {
		if role == "" || p.Role == role {
			errs = append(errs, t.KillSession(p.Session))
		}
	}
	return errors.Join(errs...)
}

// CloseTrain ends track's train handle with its session, nil when it
// has none.
func CloseTrain(t TrainTmux, track, handle string) error {
	panes, err := Trains(t, track)
	if err != nil {
		return err
	}
	session := TrainSession(track, handle)
	for _, p := range panes {
		if p.Session == session {
			return t.KillSession(session)
		}
	}
	return nil
}

// Attached says whether a client is attached to session, as an
// overlay's is.
func Attached(t TrainTmux, session string) (bool, error) {
	clients, err := t.SessionClients(session)
	return len(clients) > 0, err
}

// TrainOf is the track and handle of the train window belongs to, ""
// for none.
func TrainOf(t Tmux, window string) (track, handle string, err error) {
	if track, err = t.WindowOption(window, trainTrackOption); err != nil || track == "" {
		return "", "", err
	}
	handle, err = t.WindowOption(window, trainHandleOption)
	return track, handle, err
}

// TrackOf is the ID of the track window belongs to, "" for none.
func TrackOf(t Tmux, window string) (string, error) {
	return t.WindowOption(window, trackOption)
}
