package tracks

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/side"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/train"
)

// Member is one of those on a track's relay: the track, one of its
// trains or one of its side tracks.
type Member struct {
	Handle, Agent, Engine string
	Status, Outcome       track.Status
}

// relayPoll is how often RelayWait looks for messages.
const relayPoll = 500 * time.Millisecond

// relayWait bounds RelayWait, as reviewWait bounds WaitReview.
var relayWait = 30 * time.Minute

// RelaySend sends body, of kind, from track id to one on its relay:
// one of its trains or side tracks, by its handle or a prefix only one
// of them has. A side track sends to its main track as @track, on the
// main track's relay and under its own handle there, never the main
// track's; and to another side track of it, there too, by its handle.
func (s *Service) RelaySend(ctx context.Context, id, to string, kind relay.Kind, body string) (relay.Message, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return relay.Message{}, err
	}
	if side.Is(t) && strings.TrimPrefix(to, "@") == relay.TrackHandle {
		if err := relay.CheckSend(t.Side, relay.TrackHandle, kind, body); err != nil {
			return relay.Message{}, Problem(err.Error() + ".")
		}
		return s.Store.AddRelayMessage(ctx, t.Parent, relay.Message{From: t.Side, To: relay.TrackHandle, Kind: kind, Body: body, At: s.now()})
	}
	if side.Is(t) && strings.TrimPrefix(to, "@") == t.Side {
		return relay.Message{}, Problem("A message goes to someone else.")
	}
	trains, err := s.Trains(ctx, id)
	if err != nil {
		return relay.Message{}, err
	}
	sides, err := s.sideTracksOn(ctx, t)
	if err != nil {
		return relay.Message{}, err
	}
	// A side track's prefix names another side track, never itself.
	sides = slices.DeleteFunc(sides, func(st track.Track) bool { return st.ID == t.ID })
	handles := make([]string, 0, len(trains)+len(sides))
	for _, tr := range trains {
		handles = append(handles, tr.Handle)
	}
	for _, st := range sides {
		handles = append(handles, st.Side)
	}
	handle, err := train.Resolve(to, handles)
	if err != nil {
		return relay.Message{}, Problem(err.Error() + ": tracks relay who lists who's on the relay.")
	}
	if i := slices.IndexFunc(sides, func(st track.Track) bool { return st.Side == handle }); i >= 0 && side.Is(t) {
		return s.sendSibling(ctx, t, sides[i], kind, body)
	}
	if err := relay.CheckSend(relay.TrackHandle, handle, kind, body); err != nil {
		return relay.Message{}, Problem(err.Error() + ".")
	}
	for _, st := range sides {
		if st.Side == handle && !st.Open() {
			return relay.Message{}, Problem("@" + handle + ", side track " + st.Name + ", has ended.")
		}
	}
	for _, tr := range trains {
		switch {
		case tr.Handle != handle || tr.Agent.ReadsRelay():
		case tr.Agent == train.Reviewer && kind == relay.Question:
			// It runs no commands: Tracks types the question to it.
			return s.askReview(ctx, id, body)
		case tr.Agent == train.Reviewer:
			return relay.Message{}, Problem("The reviewer takes questions: send it --kind question.")
		default:
			return relay.Message{}, Problem("The " + tr.Agent.Name() + " doesn't read the relay.")
		}
	}
	return s.Store.AddRelayMessage(ctx, id, relay.Message{From: relay.TrackHandle, To: handle, Kind: kind, Body: body, At: s.now()})
}

// inbox is a relay a track's agent reads, and the handle it reads as.
type inbox struct{ relay, reader string }

// inboxes are the relays t's agent reads as reader: its own, and when
// it reads as the track and t is a side track, its main track's under
// its handle there.
func inboxes(t track.Track, reader string) []inbox {
	out := []inbox{{t.ID, reader}}
	if reader == relay.TrackHandle && side.Is(t) {
		out = append(out, inbox{t.Parent, t.Side})
	}
	return out
}

// unread is what reader hasn't read in t's inboxes and is addressed to
// it, oldest first: what its side tracks send each other wakes no main
// track, which reads it all the same.
func (s *Service) unread(ctx context.Context, t track.Track, reader string) ([]relay.Message, error) {
	var out []relay.Message
	for _, in := range inboxes(t, reader) {
		msgs, err := s.Store.RelayUnread(ctx, in.relay, in.reader)
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			if relay.For(in.reader, m) {
				out = append(out, m)
			}
		}
	}
	// IDs grow across relays, so they order the two.
	slices.SortFunc(out, func(a, b relay.Message) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// RelayRead is what reader hasn't read on track id's relay, now marked
// read; a side track's agent also reads what came for it on its main
// track's.
func (s *Service) RelayRead(ctx context.Context, id, reader string) ([]relay.Message, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return nil, err
	}
	// Every inbox is read before any is marked, so a failure loses
	// nothing.
	ins := inboxes(t, reader)
	unread := make([][]relay.Message, len(ins))
	var out []relay.Message
	for i, in := range ins {
		if unread[i], err = s.Store.RelayUnread(ctx, in.relay, in.reader); err != nil {
			return nil, err
		}
		out = append(out, unread[i]...)
	}
	mark := context.WithoutCancel(ctx)
	for i, in := range ins {
		if n := len(unread[i]); n > 0 {
			if _, err := s.Store.MarkRelayRead(mark, in.relay, in.reader, unread[i][n-1].ID); err != nil {
				return nil, err
			}
		}
	}
	slices.SortFunc(out, func(a, b relay.Message) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// RelayShow is message msg of track id's relay, or of a side track's
// main track's, if reader may read it there.
func (s *Service) RelayShow(ctx context.Context, id, reader string, msg int64) (relay.Message, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return relay.Message{}, err
	}
	for _, in := range inboxes(t, reader) {
		m, err := s.Store.RelayMessage(ctx, in.relay, msg)
		switch {
		case errors.Is(err, store.ErrNotFound):
		case err != nil:
			return relay.Message{}, err
		case relay.CanRead(in.reader, m):
			return m, nil
		}
	}
	return relay.Message{}, Problem(fmt.Sprintf("This track's relay has no message %d for you.", msg))
}

// RelayWait waits for a message for reader on track id's relay, other
// than a notice, which wakes no one, and returns what it hadn't read,
// marked read, as RelayRead does.
func (s *Service) RelayWait(ctx context.Context, id, reader string) ([]relay.Message, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, relayWait)
	defer cancel()
	timedOut := func() bool { return errors.Is(ctx.Err(), context.DeadlineExceeded) }
	// read are notices it read while another reader took what woke it:
	// they come with what it returns.
	var read []relay.Message
	for {
		woken, err := s.woken(ctx, id, reader)
		if err == nil && woken {
			var msgs []relay.Message
			msgs, err = s.RelayRead(ctx, id, reader)
			read = append(read, msgs...)
			if err == nil && slices.ContainsFunc(msgs, func(m relay.Message) bool { return wakes(t, reader, m) }) {
				return read, nil
			}
		}
		switch {
		case err != nil && timedOut():
			return nil, Problem(fmt.Sprintf("No message after %s.", relayWait))
		case err != nil:
			return nil, err
		}
		select {
		case <-ctx.Done():
			if timedOut() {
				return nil, Problem(fmt.Sprintf("No message after %s.", relayWait))
			}
			return nil, ctx.Err()
		case <-time.After(relayPoll):
		}
	}
}

// wakes reports whether m, read by reader in t's inboxes, wakes it: not
// a notice, and addressed to it, on its own relay or, in a side track,
// on its main track's under its handle.
func wakes(t track.Track, reader string, m relay.Message) bool {
	if m.Kind == relay.Notice {
		return false
	}
	return relay.For(reader, m) || reader == relay.TrackHandle && side.Is(t) && relay.For(t.Side, m)
}

// woken reports whether reader has an unread message on track id's
// relay that isn't a notice.
func (s *Service) woken(ctx context.Context, id, reader string) (bool, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return false, err
	}
	msgs, err := s.unread(ctx, t, reader)
	return slices.ContainsFunc(msgs, func(m relay.Message) bool { return m.Kind != relay.Notice }), err
}

// RelayWho is who's on track id's relay: the track, its trains, then
// its side tracks. A side track sees its main track's relay as its own:
// the main track as @track, itself, its own trains, then the other side
// tracks.
func (s *Service) RelayWho(ctx context.Context, id string) ([]Member, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return nil, err
	}
	trains, err := s.Trains(ctx, id)
	if err != nil {
		return nil, err
	}
	var out []Member
	if side.Is(t) {
		main, err := s.relayTrack(ctx, t.Parent)
		if err != nil {
			return nil, err
		}
		out = append(out, trackMember(relay.TrackHandle, "Main track "+main.Name, main),
			trackMember(t.Side, "Side track "+t.Name+" (you)", t))
	} else {
		out = append(out, trackMember(relay.TrackHandle, "Track", t))
	}
	for _, tr := range trains {
		out = append(out, Member{Handle: tr.Handle, Agent: tr.Agent.Name(), Engine: tr.Engine, Status: tr.Status, Outcome: tr.Outcome})
	}
	sides, err := s.sideTracksOn(ctx, t)
	if err != nil {
		return nil, err
	}
	for _, st := range sides {
		if st.ID != t.ID {
			out = append(out, trackMember(st.Side, "Side track "+st.Name, st))
		}
	}
	return out, nil
}

// sideTracksOn are the side tracks on the relay t's agent sends on as
// @track: its own, or, for a side track, which starts none, its main
// track's, itself among them.
func (s *Service) sideTracksOn(ctx context.Context, t track.Track) ([]track.Track, error) {
	if side.Is(t) {
		return s.Store.SideTracks(ctx, t.Parent)
	}
	return s.Store.SideTracks(ctx, t.ID)
}

// trackMember is track t on a relay as handle, shown as agent.
func trackMember(handle, agent string, t track.Track) Member {
	engine := t.Engine
	if e, ok := agents.ByID(t.Engine); ok {
		engine = e.Name
	}
	if t.Model != "" {
		engine += " (" + t.Model + ")"
	}
	return Member{Handle: handle, Agent: agent, Engine: engine, Status: t.Status()}
}

// relayTrack is track id, or a Problem when it's gone.
func (s *Service) relayTrack(ctx context.Context, id string) (track.Track, error) {
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return t, Problem("That track is gone.")
	}
	return t, err
}
