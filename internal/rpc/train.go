package rpc

import "context"

// TrainList lists a track's trains; TrainEnd ends one of them.
const (
	TrainList = "train-list"
	TrainEnd  = "train-end"
)

// TrainListParams names the track whose trains to list.
type TrainListParams struct {
	ID string `json:"id"`
}

// Train is one of a track's trains, as tracks train list shows it.
type Train struct {
	Handle string `json:"handle"`
	Agent  string `json:"agent"`
	// Engine is the agent CLI and model it runs on, "" when unknown.
	Engine string `json:"engine,omitempty"`
	// Status and Outcome are badge labels; Outcome "" for none.
	// StatusID and OutcomeID name them, for drawing their badges.
	Status    string `json:"status"`
	Outcome   string `json:"outcome,omitempty"`
	StatusID  string `json:"status_id,omitempty"`
	OutcomeID string `json:"outcome_id,omitempty"`
}

// TrainListResult is a track's trains.
type TrainListResult struct {
	Trains []Train `json:"trains"`
}

// TrainEndParams names the train to end, by its track and handle.
type TrainEndParams struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
}

// TrainList is track id's trains.
func (c Client) TrainList(ctx context.Context, id string) ([]Train, error) {
	var r TrainListResult
	return r.Trains, c.Call(ctx, TrainList, TrainListParams{ID: id}, &r, nil)
}

// TrainEnd ends track id's train handle.
func (c Client) TrainEnd(ctx context.Context, id, handle string) error {
	return c.Call(ctx, TrainEnd, TrainEndParams{ID: id, Handle: handle}, nil, nil)
}
