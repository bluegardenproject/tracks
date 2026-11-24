package tracks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/train"
)

// Member is one of those on a track's relay: the track or one of its
// trains.
type Member struct {
	Handle, Agent, Engine string
	Status, Outcome       track.Status
}

// relayPoll is how often RelayWait looks for messages.
const relayPoll = 500 * time.Millisecond

// relayWait bounds RelayWait, as reviewWait bounds WaitReview.
var relayWait = 30 * time.Minute

// RelaySend sends body, of kind, from track id to its train to, a
// handle or a prefix only one of its trains has.
func (s *Service) RelaySend(ctx context.Context, id, to string, kind relay.Kind, body string) (relay.Message, error) {
	trains, err := s.Trains(ctx, id)
	if err != nil {
		return relay.Message{}, err
	}
	handles := make([]string, len(trains))
	for i, tr := range trains {
		handles[i] = tr.Handle
	}
	handle, err := train.Resolve(to, handles)
	if err != nil {
		return relay.Message{}, Problem(err.Error() + ": tracks relay who lists who's on the relay.")
	}
	if err := relay.CheckSend(relay.TrackHandle, handle, kind, body); err != nil {
		return relay.Message{}, Problem(err.Error() + ".")
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
	return s.Store.AddRelayMessage(ctx, id, relay.Message{From: relay.TrackHandle, To: handle, Kind: kind, Body: body})
}

// RelayRead is what reader hasn't read on track id's relay, now marked
// read.
func (s *Service) RelayRead(ctx context.Context, id, reader string) ([]relay.Message, error) {
	if _, err := s.relayTrack(ctx, id); err != nil {
		return nil, err
	}
	msgs, err := s.Store.RelayUnread(ctx, id, reader)
	if err != nil || len(msgs) == 0 {
		return nil, err
	}
	return msgs, s.Store.MarkRelayRead(ctx, id, reader, msgs[len(msgs)-1].ID)
}

// RelayShow is message msg of track id's relay, if reader may read it.
func (s *Service) RelayShow(ctx context.Context, id, reader string, msg int64) (relay.Message, error) {
	if _, err := s.relayTrack(ctx, id); err != nil {
		return relay.Message{}, err
	}
	m, err := s.Store.RelayMessage(ctx, id, msg)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !relay.CanRead(reader, m)) {
		return relay.Message{}, Problem(fmt.Sprintf("This track's relay has no message %d for you.", msg))
	}
	return m, err
}

// RelayWait waits for messages for reader on track id's relay and
// returns them, marked read, as RelayRead does.
func (s *Service) RelayWait(ctx context.Context, id, reader string) ([]relay.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, relayWait)
	defer cancel()
	timedOut := func() bool { return errors.Is(ctx.Err(), context.DeadlineExceeded) }
	for {
		msgs, err := s.RelayRead(ctx, id, reader)
		switch {
		case err != nil && timedOut():
			return nil, Problem(fmt.Sprintf("No message after %s.", relayWait))
		case err != nil || len(msgs) > 0:
			return msgs, err
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

// RelayWho is who's on track id's relay: the track, then its trains.
func (s *Service) RelayWho(ctx context.Context, id string) ([]Member, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return nil, err
	}
	trains, err := s.Trains(ctx, id)
	if err != nil {
		return nil, err
	}
	engine := t.Engine
	if e, ok := agents.ByID(t.Engine); ok {
		engine = e.Name
	}
	if t.Model != "" {
		engine += " (" + t.Model + ")"
	}
	out := []Member{{Handle: relay.TrackHandle, Agent: "Track", Engine: engine, Status: t.Status()}}
	for _, tr := range trains {
		out = append(out, Member{Handle: tr.Handle, Agent: tr.Agent.Name(), Engine: tr.Engine, Status: tr.Status, Outcome: tr.Outcome})
	}
	return out, nil
}

// relayTrack is track id, or a Problem when it's gone.
func (s *Service) relayTrack(ctx context.Context, id string) (track.Track, error) {
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return t, Problem("That track is gone.")
	}
	return t, err
}
