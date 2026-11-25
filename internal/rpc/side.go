package rpc

import "context"

// SideNew makes a side track, as Create makes a track: a method of its
// own, so a daemon from before side tracks refuses it rather than make
// a track that isn't one. SideList lists a main track's side tracks.
const (
	SideNew  = "side-new"
	SideList = "side-list"
)

// SideTrack is one of a main track's side tracks, as tracks side list
// shows it.
type SideTrack struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Title  string `json:"title,omitempty"` // what the user calls it, "" for its name
	Handle string `json:"handle"`
	// Status and PR are badge labels; PR "" for none. StatusID and PRID
	// name them, for the Platform's badges.
	Status   string `json:"status"`
	PR       string `json:"pr,omitempty"`
	StatusID string `json:"status_id,omitempty"`
	PRID     string `json:"pr_id,omitempty"`
	// PRs are its pull requests' links.
	PRs []string `json:"prs,omitempty"`
}

// SideListResult is a main track's side tracks.
type SideListResult struct {
	Sides []SideTrack `json:"sides"`
}

// SideNew makes the side track p asks for; p.Request.Side must be set.
func (c Client) SideNew(ctx context.Context, p CreateParams, progress func(string)) (CreateResult, error) {
	var r CreateResult
	return r, c.Call(ctx, SideNew, p, &r, progress)
}

// SideList is main track id's side tracks.
func (c Client) SideList(ctx context.Context, id string) ([]SideTrack, error) {
	var r SideListResult
	return r.Sides, c.Call(ctx, SideList, TrainListParams{ID: id}, &r, nil)
}
