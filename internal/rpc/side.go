package rpc

import "context"

// SideNew makes a side track, as Create makes a track: a method of its
// own, so a daemon from before side tracks refuses it rather than make
// a track that isn't one. SideList lists a main track's side tracks;
// SideEnd ends one of them.
const (
	SideNew  = "side-new"
	SideList = "side-list"
	SideEnd  = "side-end"
)

// SideEndParams names the side track to end: its main track, and its
// handle or a prefix only it has.
type SideEndParams struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
}

// SideEndResult is the side track that ended.
type SideEndResult struct {
	Name   string `json:"name"`
	Handle string `json:"handle"`
}

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

// SideListResult is a main track's side tracks. Main is a side track's
// main track instead, its ID and name alone; nil for a main track.
type SideListResult struct {
	Sides []SideTrack `json:"sides"`
	Main  *SideTrack  `json:"main,omitempty"`
}

// SideNew makes the side track p asks for; p.Request.Side must be set.
func (c Client) SideNew(ctx context.Context, p CreateParams, progress func(string)) (CreateResult, error) {
	var r CreateResult
	return r, c.Call(ctx, SideNew, p, &r, progress)
}

// SideList is main track id's side tracks.
func (c Client) SideList(ctx context.Context, id string) ([]SideTrack, error) {
	r, err := c.Sides(ctx, id)
	return r.Sides, err
}

// Sides is track id's side tracks, or its main track when it's a side
// track.
func (c Client) Sides(ctx context.Context, id string) (SideListResult, error) {
	var r SideListResult
	return r, c.Call(ctx, SideList, TrainListParams{ID: id}, &r, nil)
}

// SideEnd ends main track id's side track handle, and returns it.
func (c Client) SideEnd(ctx context.Context, id, handle string) (SideEndResult, error) {
	var r SideEndResult
	return r, c.Call(ctx, SideEnd, SideEndParams{ID: id, Handle: handle}, &r, nil)
}
