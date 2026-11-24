package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/relay"
)

// RelayLog is a track's whole relay, as the Relay Station shows it.
type RelayLog struct {
	Messages []relay.Message
	// Waiting are the messages still waiting for their pointer.
	Waiting []int64
}

// RelayStation is track id's whole relay, oldest first, and which of
// its messages wait for a pointer; what it returns is marked seen by
// the user, never read by any agent.
func (s *Service) RelayStation(ctx context.Context, id string) (RelayLog, error) {
	if _, err := s.relayTrack(ctx, id); err != nil {
		return RelayLog{}, err
	}
	msgs, err := s.Store.RelayLog(ctx, id)
	if err != nil || len(msgs) == 0 {
		return RelayLog{}, err
	}
	unread, err := s.Store.RelayUnread(ctx, id, relay.TrackHandle)
	if err != nil {
		return RelayLog{}, err
	}
	s.mu.Lock()
	pointed := s.pointed[id]
	s.mu.Unlock()
	if _, err := s.Store.MarkRelayRead(ctx, id, relay.UserHandle, msgs[len(msgs)-1].ID); err != nil {
		return RelayLog{}, err
	}
	return RelayLog{Messages: msgs, Waiting: relay.Waiting(unread, pointed)}, nil
}

// RelayNew counts the messages on track id's relay the user hasn't
// seen in the Relay Station.
func (s *Service) RelayNew(ctx context.Context, id string) (int, error) {
	if _, err := s.relayTrack(ctx, id); err != nil {
		return 0, err
	}
	return s.Store.RelayUnseen(ctx, id, relay.UserHandle)
}
