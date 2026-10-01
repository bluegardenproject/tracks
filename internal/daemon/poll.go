package daemon

import (
	"context"
	"log"
	"sync"
)

// poller runs one job at a time beside the daemon's loop, since gh or
// git can take seconds, and logs a failure only when it changes, so a
// missing gh is logged once. What names the job in the log.
type poller struct {
	log     *log.Logger
	what    string
	mu      sync.Mutex
	running bool
	last    string
	wg      sync.WaitGroup
}

// start runs poll unless one is still running.
func (p *poller) start(ctx context.Context, poll func(context.Context) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return
	}
	p.running = true
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		err := poll(ctx)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.running = false
		msg := ""
		if err != nil && ctx.Err() == nil {
			msg = err.Error()
		}
		if msg != p.last && msg != "" {
			p.log.Printf("%s: %s", p.what, msg)
		}
		p.last = msg
	}()
}

// wait returns once the running poll is done.
func (p *poller) wait() { p.wg.Wait() }
