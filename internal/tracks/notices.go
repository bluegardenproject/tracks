package tracks

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
)

// Notice tells the user about a track: it needs them, it failed, or
// one of its PRs opened or settled.
type Notice struct {
	Track, Name string // the track's ID and name
	Event       string // a settings.Notify* event
	Title, Body string
	// Train says a train of the track waits on the user: its overlay
	// isn't on screen when the track is, so it's told even then, and
	// it's told once a run already, so no quiet time holds it back.
	Train bool
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

// trainNotice is the notice for m, a train's message to the track named
// name, if it's one to tell: a report, an answer, a question or an
// event. Its body is the message's first line, for the user.
func trainNotice(id, name string, m relay.Message) (Notice, bool) {
	var did string
	switch m.Kind {
	case relay.Report:
		did = "reported"
	case relay.Answer:
		did = "answered"
	case relay.Question:
		did = "asks something"
	case relay.Event:
		did = "saw something"
	default:
		return Notice{}, false
	}
	if m.To != relay.TrackHandle {
		return Notice{}, false
	}
	first, _, _ := strings.Cut(strings.TrimSpace(m.Body), "\n")
	if r := []rune(first); len(r) > 120 {
		first = string(r[:119]) + "…"
	}
	return Notice{Track: id, Name: name, Event: settings.NotifyTrainReported,
		Title: "Tracks: @" + m.From + " " + did + " on " + name, Body: first}, true
}

// trainWaitingNotice is the notice that train handle of the track named
// name waits on the user, as on Claude's trust question, which only its
// overlay shows. It counts as the track needing them.
func trainWaitingNotice(id, name, handle string) (Notice, bool) {
	return Notice{Track: id, Name: name, Event: settings.NotifyActionRequired,
		Title: "Tracks: @" + handle + " on " + name + " needs you", Body: "It's waiting for an answer in its overlay.", Train: true}, true
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
