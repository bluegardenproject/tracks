package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
)

// sendSibling sends m from side track t to sib, another side track of
// its main track, on the main track's relay, where the main track reads
// it: a question or an answer, within their pair's limit.
func (s *Service) sendSibling(ctx context.Context, t, sib track.Track, m relay.Message) (relay.Message, error) {
	// One send at a time counts a pair's messages and adds one.
	s.siblingMu.Lock()
	defer s.siblingMu.Unlock()
	log, err := s.Store.RelayLog(ctx, t.Parent)
	if err != nil {
		return relay.Message{}, err
	}
	if err := relay.CheckPeerSend(t.Side, sib.Side, m.Kind, m.Body, relay.Between(log, t.Side, sib.Side)); err != nil {
		return relay.Message{}, Problem(err.Error() + ".")
	}
	if !sib.Open() {
		return relay.Message{}, Problem("@" + sib.Side + ", side track " + sib.Name + ", has ended.")
	}
	m.From, m.To = t.Side, sib.Side
	return s.Store.AddRelayMessage(ctx, t.Parent, m)
}
