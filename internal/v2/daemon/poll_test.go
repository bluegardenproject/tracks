package daemon

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestPoller(t *testing.T) {
	ctx := context.Background()
	var logs bytes.Buffer
	p := &poller{log: log.New(&logs, "", 0)}

	release := make(chan struct{})
	runs := 0
	slow := func(context.Context) error { runs++; <-release; return nil }
	p.start(ctx, slow)
	p.start(ctx, slow)
	close(release)
	p.wait()
	if runs != 1 {
		t.Errorf("%d polls ran; one still running keeps the next from starting", runs)
	}

	noGH := func(context.Context) error { return errors.New("gh isn't installed") }
	for range 3 {
		p.start(ctx, noGH)
		p.wait()
	}
	p.start(ctx, func(context.Context) error { return nil })
	p.wait()
	p.start(ctx, noGH)
	p.wait()
	if n := strings.Count(logs.String(), "gh isn't installed"); n != 2 {
		t.Errorf("logged %d times:\n%s\nwant once, and again after a pass that worked", n, logs.String())
	}
}
