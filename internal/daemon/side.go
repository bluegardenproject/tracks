package daemon

import (
	"cmp"
	"context"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

// sideNew is create for a side track alone, which create takes too.
func (c Config) sideNew(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.CreateParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	if !p.Side {
		return nil, tracks.Problem("side-new makes side tracks only.")
	}
	return c.create(ctx, call)
}

func (c Config) sideList(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.TrainListParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	sides, err := c.Tracks.SideTracks(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := rpc.SideListResult{Sides: []rpc.SideTrack{}}
	if mainID, name, ok, err := c.Tracks.MainOf(ctx, p.ID); err != nil {
		return nil, err
	} else if ok && mainID != "" {
		out.Main = &rpc.SideTrack{ID: mainID, Name: name}
	}
	for _, t := range sides {
		status, pr := t.Status(), track.PRStatus(t.PRs)
		st := rpc.SideTrack{ID: t.ID, Name: t.Name, Title: t.Title, Handle: t.Side, Status: status.Label, PR: pr.Label,
			StatusID: status.ID, PRID: pr.ID}
		for _, pr := range t.PRs {
			st.PRs = append(st.PRs, pr.URL)
		}
		out.Sides = append(out.Sides, st)
	}
	return out, nil
}

func (c Config) sideEnd(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.SideEndParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	st, err := c.Tracks.EndSide(ctx, p.ID, p.Handle)
	if err != nil {
		return nil, err
	}
	return rpc.SideEndResult{Name: cmp.Or(st.Title, st.Name), Handle: st.Side}, nil
}
