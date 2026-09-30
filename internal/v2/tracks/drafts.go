package tracks

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Draft is a creation that failed, listed in Station: what was asked
// for, and why it failed.
type Draft struct {
	Request Request `json:"request"`
	Error   string  `json:"error"`
}

// Failure is how a failed creation is told: a Problem as it is, any
// other error after "Couldn't create the track: ".
func Failure(err error) string {
	var p Problem
	if errors.As(err, &p) {
		return err.Error()
	}
	return "Couldn't create the track: " + err.Error()
}

// keepDraft saves req, which failed with err, as the draft req.Draft.
func (s *Service) keepDraft(ctx context.Context, req Request, err error) error {
	id := req.Draft
	req.Draft = ""
	data, jerr := json.Marshal(req)
	if jerr != nil {
		return jerr
	}
	return s.Store.SaveDraft(ctx, store.Draft{ID: id, Request: string(data), Error: Failure(err), FailedAt: s.now()})
}

// Draft is the request draft id keeps, to start it again.
func (s *Service) Draft(ctx context.Context, id string) (Request, error) {
	d, err := s.Store.Draft(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return Request{}, Problem("That draft is gone.")
	} else if err != nil {
		return Request{}, err
	}
	req, err := draftRequest(d)
	return req, err
}

// DiscardDraft deletes the draft id.
func (s *Service) DiscardDraft(ctx context.Context, id string) error {
	deleted, err := s.Store.DeleteDraft(ctx, id)
	if err == nil && !deleted {
		return Problem("That draft is gone.")
	}
	return err
}

func draftRequest(d store.Draft) (Request, error) {
	var req Request
	if err := json.Unmarshal([]byte(d.Request), &req); err != nil {
		return Request{}, err
	}
	req.Draft = d.ID
	return req, nil
}

// drafts are the drafts Station lists under f at now: all of them
// without a filter; under one, only when it picks the draft status.
func (s *Service) drafts(ctx context.Context, f track.Filter, now time.Time) ([]Listed, error) {
	if f.On() && !draftsPicked(f) {
		return nil, nil
	}
	drafts, err := s.Store.Drafts(ctx)
	if err != nil {
		return nil, err
	}
	var out []Listed
	for _, d := range drafts {
		req, err := draftRequest(d)
		if err != nil {
			return nil, err
		}
		from, to := f.Range(now)
		if f.On() && (!from.IsZero() && d.FailedAt.Before(from) || !to.IsZero() && !d.FailedAt.Before(to)) {
			continue
		}
		out = append(out, draftListed(d, req))
	}
	return out, nil
}

// draftsPicked reports whether filter f can pick drafts: they're
// never archived, and have no PRs.
func draftsPicked(f track.Filter) bool {
	return !f.Archived && slices.Contains(f.Statuses, track.Draft.ID) &&
		(len(f.PRStatuses) == 0 || slices.Contains(f.PRStatuses, track.NoPRs.ID))
}

// draftListed is how Station shows draft d of req: under the name
// typed, or else the prompt's first line.
func draftListed(d store.Draft, req Request) Listed {
	title := req.Name
	if title == "" {
		title, _, _ = strings.Cut(strings.TrimSpace(req.Prompt), "\n")
	}
	t := track.Track{ID: d.ID, Kind: req.Kind, Title: title, Engine: req.Engine, Model: req.Model, CreatedAt: d.FailedAt}
	for _, name := range req.Repos {
		t.Repos = append(t.Repos, track.Repo{Name: name})
	}
	return Listed{Track: t, Draft: &Draft{Request: req, Error: d.Error}}
}
