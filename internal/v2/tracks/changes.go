package tracks

import (
	"context"
	"sync"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Changes tells its subscribers that the tracks changed. The zero value
// is ready to use; a nil one tells nobody.
type Changes struct {
	mu     sync.Mutex
	subs   map[chan struct{}]bool
	closed bool
}

// Subscribe returns a channel that receives once right away and once
// after each change; changes close together arrive as one. It is closed
// by Close. cancel ends the subscription.
func (c *Changes) Subscribe() (changed <-chan struct{}, cancel func()) {
	ch := make(chan struct{}, 1)
	if c == nil {
		close(ch)
		return ch, func() {}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		close(ch)
		return ch, func() {}
	}
	if c.subs == nil {
		c.subs = map[chan struct{}]bool{}
	}
	c.subs[ch] = true
	ch <- struct{}{}
	return ch, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.subs, ch)
	}
}

// Notify tells every subscriber that the tracks changed.
func (c *Changes) Notify() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for ch := range c.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Close ends every subscription, and those made later.
func (c *Changes) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for ch := range c.subs {
		close(ch)
	}
	c.subs, c.closed = nil, true
}

// Watched is st, telling changes after each write that changed
// something.
func Watched(st Store, changes *Changes) Store {
	return watched{st, changes}
}

type watched struct {
	Store
	changes *Changes
}

func (w watched) notify(err error) error {
	if err == nil {
		w.changes.Notify()
	}
	return err
}

func (w watched) AddTrack(ctx context.Context, t track.Track) error {
	return w.notify(w.Store.AddTrack(ctx, t))
}

func (w watched) SetFilter(ctx context.Context, f track.Filter) error {
	return w.notify(w.Store.SetFilter(ctx, f))
}

func (w watched) SetState(ctx context.Context, id string, st track.State) error {
	return w.notify(w.Store.SetState(ctx, id, st))
}

func (w watched) DeleteTrack(ctx context.Context, id string) error {
	return w.notify(w.Store.DeleteTrack(ctx, id))
}

func (w watched) Rename(ctx context.Context, id, name string) error {
	return w.notify(w.Store.Rename(ctx, id, name))
}

func (w watched) SetCost(ctx context.Context, id string, cost float64) error {
	return w.notify(w.Store.SetCost(ctx, id, cost))
}

func (w watched) SetBranch(ctx context.Context, id string, position int, branch string) error {
	return w.notify(w.Store.SetBranch(ctx, id, position, branch))
}

func (w watched) AddTrackRepo(ctx context.Context, id string, r track.Repo) error {
	return w.notify(w.Store.AddTrackRepo(ctx, id, r))
}

func (w watched) Promote(ctx context.Context, t track.Track) error {
	return w.notify(w.Store.Promote(ctx, t))
}

func (w watched) SaveDraft(ctx context.Context, d store.Draft) error {
	return w.notify(w.Store.SaveDraft(ctx, d))
}

func (w watched) DeleteDraft(ctx context.Context, id string) (bool, error) {
	deleted, err := w.Store.DeleteDraft(ctx, id)
	if deleted {
		w.changes.Notify()
	}
	return deleted, err
}

func (w watched) AddPR(ctx context.Context, id string, pr track.PR, at time.Time) (bool, error) {
	added, err := w.Store.AddPR(ctx, id, pr, at)
	if added {
		w.changes.Notify()
	}
	return added, err
}

func (w watched) SavePR(ctx context.Context, id string, pr track.PR, at time.Time) (track.PRState, error) {
	was, err := w.Store.SavePR(ctx, id, pr, at)
	if err == nil && was != pr.State {
		w.changes.Notify()
	}
	return was, err
}
