package daemon

import (
	"context"
	"sync/atomic"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/commentstyle"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

// learning is set while the comment style is learned: one run at a
// time, as there's one daemon per user.
var learning atomic.Bool

// learnStyle learns the user's PR comment style, sending each log line
// as progress. Hanging up stops it.
func (c Config) learnStyle(ctx context.Context, call *rpc.Call) (any, error) {
	if !learning.CompareAndSwap(false, true) {
		return nil, tracks.Problem("Tracks is learning your comment style already.")
	}
	defer learning.Store(false)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-call.Gone:
			cancel()
		case <-ctx.Done():
		}
	}()

	engine, err := c.styleEngine(ctx)
	if err != nil {
		return nil, err
	}
	l := commentstyle.Learner{API: commentstyle.GH{}, Engine: engine}
	st, err := l.Learn(ctx, commentstyle.Path(c.Paths.ConfigDir), func(line commentstyle.Line) {
		call.Progress(line.String())
	})
	if err != nil {
		call.Progress(commentstyle.Line{Kind: commentstyle.Fail, Text: err.Error()}.String())
		return nil, err
	}
	c.Log.Printf("learned the comment style from %d comments", st.Comments)
	return rpc.StyleResult{At: st.At, Comments: st.Comments}, nil
}

// styleEngine is the engine new tracks run on, found on PATH, with its
// default model.
func (c Config) styleEngine(ctx context.Context) (commentstyle.Engine, error) {
	s, err := settings.Load(c.Paths.Settings)
	if err != nil {
		return commentstyle.Engine{}, err
	}
	id := s.Engines.DefaultID()
	e, ok := agents.ByID(id)
	conf := s.Engines.Get(id)
	if !ok || conf == nil {
		return commentstyle.Engine{}, tracks.ErrNoEngine
	}
	found, err := agents.Check(ctx, e)
	if err != nil {
		return commentstyle.Engine{}, tracks.Problem(err.Error())
	}
	return commentstyle.Engine{ID: id, Name: e.Name, Program: found.Path, Model: conf.Model}, nil
}
