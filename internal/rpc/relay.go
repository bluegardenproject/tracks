package rpc

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/relay"
)

// The relay's calls, for the track's agent: send to a train, read what
// came, show one message, wait for messages, and list who's there. For
// the user: RelayStation is the whole relay, now seen; RelayNew counts
// what they haven't seen.
const (
	RelaySend    = "relay-send"
	RelayRead    = "relay-read"
	RelayShow    = "relay-show"
	RelayWait    = "relay-wait"
	RelayWho     = "relay-who"
	RelayStation = "relay-station"
	RelayNew     = "relay-new"
)

// RelayIDParams names the track whose relay to read, wait on or list.
type RelayIDParams struct {
	ID string `json:"id"`
}

// RelaySendParams is a message from track ID to To.
type RelaySendParams struct {
	ID   string     `json:"id"`
	To   string     `json:"to"`
	Kind relay.Kind `json:"kind"`
	Body string     `json:"body"`
}

// RelayShowParams names one message of track ID's relay.
type RelayShowParams struct {
	ID  string `json:"id"`
	Msg int64  `json:"msg"`
}

// RelayMessages are messages from a relay.
type RelayMessages struct {
	Messages []relay.Message `json:"messages"`
}

// RelayStationResult is a track's whole relay, oldest first, and the
// IDs of the messages still waiting for their pointer.
type RelayStationResult struct {
	Messages []relay.Message `json:"messages"`
	Waiting  []int64         `json:"waiting,omitempty"`
}

// RelayNewResult counts the messages the user hasn't seen.
type RelayNewResult struct {
	New int `json:"new"`
}

// Member is one of those on a track's relay; Status and Outcome are
// badge labels.
type Member struct {
	Handle  string `json:"handle"`
	Agent   string `json:"agent"`
	Engine  string `json:"engine,omitempty"`
	Status  string `json:"status"`
	Outcome string `json:"outcome,omitempty"`
}

// RelayWhoResult is who's on a track's relay.
type RelayWhoResult struct {
	Members []Member `json:"members"`
}

// RelaySend sends body, of kind, from track id to to, and returns the
// message as sent.
func (c Client) RelaySend(ctx context.Context, id, to string, kind relay.Kind, body string) (relay.Message, error) {
	var r RelayMessages
	err := c.Call(ctx, RelaySend, RelaySendParams{ID: id, To: to, Kind: kind, Body: body}, &r, nil)
	if err != nil || len(r.Messages) == 0 {
		return relay.Message{}, err
	}
	return r.Messages[0], nil
}

// RelayRead is what track id hasn't read on its relay, now read.
func (c Client) RelayRead(ctx context.Context, id string) ([]relay.Message, error) {
	var r RelayMessages
	return r.Messages, c.Call(ctx, RelayRead, RelayIDParams{ID: id}, &r, nil)
}

// RelayShow is message msg of track id's relay.
func (c Client) RelayShow(ctx context.Context, id string, msg int64) (relay.Message, error) {
	var r RelayMessages
	err := c.Call(ctx, RelayShow, RelayShowParams{ID: id, Msg: msg}, &r, nil)
	if err != nil || len(r.Messages) == 0 {
		return relay.Message{}, err
	}
	return r.Messages[0], nil
}

// RelayWait waits for messages for track id and returns them, read.
func (c Client) RelayWait(ctx context.Context, id string) ([]relay.Message, error) {
	var r RelayMessages
	return r.Messages, c.Call(ctx, RelayWait, RelayIDParams{ID: id}, &r, nil)
}

// RelayWho is who's on track id's relay.
func (c Client) RelayWho(ctx context.Context, id string) ([]Member, error) {
	var r RelayWhoResult
	return r.Members, c.Call(ctx, RelayWho, RelayIDParams{ID: id}, &r, nil)
}

// RelayStation is track id's whole relay, now seen by the user.
func (c Client) RelayStation(ctx context.Context, id string) (RelayStationResult, error) {
	var r RelayStationResult
	return r, c.Call(ctx, RelayStation, RelayIDParams{ID: id}, &r, nil)
}

// RelayNew counts the messages on track id's relay the user hasn't seen.
func (c Client) RelayNew(ctx context.Context, id string) (int, error) {
	var r RelayNewResult
	return r.New, c.Call(ctx, RelayNew, RelayIDParams{ID: id}, &r, nil)
}
