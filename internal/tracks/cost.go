package tracks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/usage"
)

// Costs records what the open Claude tracks have cost so far, read from
// their transcripts as v1 does: a rough figure, at list prices. A
// track's transcripts are read again only once they changed, and a
// last time after its window closed.
func (s *Service) Costs(ctx context.Context) error {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	isOpen := map[string]bool{}
	for _, t := range open {
		isOpen[t.ID] = true
	}
	for id := range s.transcripts {
		if isOpen[id] {
			continue
		}
		t, err := s.Store.Track(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		} else if err != nil {
			return err
		}
		open = append(open, t)
	}
	seen := map[string]string{}
	var errs []error
	for _, t := range open {
		if t.Engine != agents.Claude.ID || t.Session == "" {
			continue
		}
		sig, err := s.cost(ctx, t)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
		}
		if isOpen[t.ID] {
			seen[t.ID] = sig
		}
	}
	s.transcripts = seen
	return errors.Join(errs...)
}

// cost records what t has cost when its transcripts changed since the
// last Costs, and returns their signature. One that can't be read
// keeps the cost recorded before, and is tried again next time.
func (s *Service) cost(ctx context.Context, t track.Track) (string, error) {
	paths := usage.Locate(t.Session, "")
	sig := signature(paths)
	if sig == "" || sig == s.transcripts[t.ID] {
		return sig, nil
	}
	total, err := usage.ParseFiles(paths)
	if err != nil {
		return s.transcripts[t.ID], err
	}
	if c := total.Usage.CostUSD; c > 0 && c != t.Cost {
		if err := s.Store.SetCost(ctx, t.ID, c); err != nil {
			return s.transcripts[t.ID], err
		}
	}
	return sig, nil
}

// signature changes when a transcript is added or written: each one's
// path, size and modification time; "" when there are none yet.
func signature(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", p, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
}
