package tracks

import (
	"context"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Notice tells the user about a track: it needs them, it failed, or
// one of its PRs opened or settled.
type Notice struct {
	Track, Name string // the track's ID and name
	Event       string // a settings.Notify* event
	Title, Body string
}

// statusNotice is the notice for a track named name whose status went
// from to to, if that change is one to tell.
func statusNotice(id, name string, from, to track.Status) (Notice, bool) {
	if from == to {
		return Notice{}, false
	}
	n := Notice{Track: id, Name: name}
	switch to {
	case track.ActionRequired:
		n.Event, n.Title, n.Body = settings.NotifyActionRequired, "Tracks: "+name+" needs you", "Its agent is waiting for an answer."
	case track.Error:
		n.Event, n.Title, n.Body = settings.NotifyError, "Tracks: "+name+" failed", "Its agent exited with an error."
	default:
		return Notice{}, false
	}
	return n, true
}

// prNotice is the notice for pr of a track named name, whose state
// was was ("" for a new PR), if that change is one to tell: a new PR
// that's open, or one that was open and settled. A PR found settled
// was never seen open, such as on an old branch.
func prNotice(id, name string, pr track.PR, was track.PRState) (Notice, bool) {
	n := Notice{Track: id, Name: name, Body: fmt.Sprintf("%s#%d", pr.Repo, pr.Number)}
	switch {
	case was == "" && !pr.State.Settled():
		n.Event, n.Title = settings.NotifyPROpened, "Tracks: "+name+" opened a PR"
	case was != "" && !was.Settled() && pr.State.Settled():
		n.Event, n.Title = settings.NotifyPRSettled, "Tracks: "+name+"'s PR was "+string(pr.State)
	default:
		return Notice{}, false
	}
	return n, true
}

// notify hands n to Notify, if there's one.
func (s *Service) notify(n Notice, ok bool) {
	if ok && s.Notify != nil {
		s.Notify(n)
	}
}

// notifyPR tells about pr of track id, whose state was was, if that's
// a change to tell. It looks the track's name up only then.
func (s *Service) notifyPR(ctx context.Context, id string, pr track.PR, was track.PRState) error {
	if _, ok := prNotice(id, "", pr, was); !ok || s.Notify == nil {
		return nil
	}
	t, err := s.Store.Track(ctx, id)
	if err != nil {
		return err
	}
	s.notify(prNotice(id, t.Name, pr, was))
	return nil
}
