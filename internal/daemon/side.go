package daemon

import (
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
	for _, t := range sides {
		status, pr := t.Status(), track.PRStatus(t.PRs)
		st := rpc.SideTrack{ID: t.ID, Name: t.Name, Handle: t.Side, Status: status.Label, PR: pr.Label,
			StatusID: status.ID, PRID: pr.ID}
		for _, pr := range t.PRs {
			st.PRs = append(st.PRs, pr.URL)
		}
		out.Sides = append(out.Sides, st)
	}
	return out, nil
}
