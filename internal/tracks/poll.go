package tracks

import (
	"context"
	"errors"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/side"
	"github.com/bluegardenproject/tracks/internal/track"
)

// PollPRs asks GitHub about the tracks' pull requests, in one poll:
// the PRs from the branches of Work tracks that aren't closed, which
// finds those opened by hand, and every PR not yet merged or closed,
// whatever its track's status, with their review threads, which
// CountComments counts. What changed on an open Work track's PR goes
// to its relay as @github's events. ErrNoGH means nothing was asked.
func (s *Service) PollPRs(ctx context.Context) error {
	if s.GitHub == nil {
		return nil
	}
	tracks, err := s.branchTracks(ctx)
	if err != nil {
		return err
	}
	var errs []error
	type branch struct {
		t track.Track
		q BranchQuery
	}
	var branches []branch
	var q Query
	asking := map[BranchQuery]bool{}
	// A checkout gh couldn't place is asked about once a poll.
	unplaced := map[string]bool{}
	for _, t := range tracks {
		for _, r := range t.Repos {
			if r.Branch == "" || unplaced[r.Path] {
				continue
			}
			repo, err := s.gitHubRepo(ctx, r.Path)
			if errors.Is(err, ErrNoGH) || ctx.Err() != nil {
				return errors.Join(err, ctx.Err())
			} else if err != nil {
				unplaced[r.Path] = true
				errs = append(errs, fmt.Errorf("%s in %s: %w", t.Name, r.Name, err))
				continue
			}
			b := branch{t, BranchQuery{Repo: repo, Branch: r.Branch}}
			branches = append(branches, b)
			if !asking[b.q] {
				asking[b.q] = true
				q.Branches = append(q.Branches, b.q)
			}
		}
	}
	unsettled, err := s.Store.UnsettledPRs(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, p := range unsettled {
		if !seen[p.URL] {
			seen[p.URL] = true
			q.PRs = append(q.PRs, p.PR)
		}
	}
	if len(q.Branches)+len(q.PRs) == 0 {
		return errors.Join(errs...)
	}
	answer, err := s.GitHub.Poll(ctx, q)
	if errors.Is(err, ErrNoGH) || ctx.Err() != nil {
		return errors.Join(err, ctx.Err())
	} else if err != nil {
		errs = append(errs, err)
	}
	s.keepThreads(answer)

	// Read after GitHub's answer, which takes a while: a track opened
	// meanwhile gets its events, one ended doesn't.
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	current := map[string]track.Track{}
	for _, t := range open {
		current[t.ID] = t
	}
	asked := map[[2]string]bool{}
	for _, b := range branches {
		// As it is now: resumed or ended during the poll.
		if now, ok := current[b.t.ID]; ok {
			b.t = now
		} else if b.t.Open() {
			if now, err := s.Store.Track(ctx, b.t.ID); err == nil {
				b.t = now
			}
		}
		for _, f := range answer.Branches[b.q] {
			pr := f.PR
			pr.CheckedAt = s.now()
			was, err := s.Store.SavePR(ctx, b.t.ID, pr, pr.CheckedAt)
			if err != nil {
				return err
			}
			s.notify(prNotice(b.t.ID, b.t.Name, pr, was))
			var post *track.Track
			if b.t.Open() || side.Is(b.t) {
				post = &b.t
			}
			if err := s.prHealth(ctx, b.t.ID, pr, was, post); err != nil {
				return err
			}
			asked[[2]string{b.t.ID, pr.URL}] = true
		}
	}
	working := map[string]*track.Track{}
	for _, t := range open {
		if t.Kind == track.Work {
			working[t.ID] = &t
		}
	}
	// An ended side track's PRs still tell its main track.
	sides := map[string]*track.Track{}
	for _, p := range unsettled {
		f, ok := answer.PRs[p.URL]
		if !ok || asked[[2]string{p.TrackID, p.URL}] {
			continue
		}
		post := working[p.TrackID]
		if post == nil {
			t, seen := sides[p.TrackID]
			if !seen {
				if found, err := s.Store.Track(ctx, p.TrackID); err == nil && found.Kind == track.Work && side.Is(found) {
					t = &found
				}
				sides[p.TrackID] = t
			}
			post = t
		}
		p.State, p.Health, p.CheckedAt = f.State, f.Health, s.now()
		was, err := s.Store.SavePR(ctx, p.TrackID, p.PR, p.CheckedAt)
		if err != nil {
			return err
		}
		if err := s.notifyPR(ctx, p.TrackID, p.PR, was); err != nil {
			return err
		}
		if err := s.prHealth(ctx, p.TrackID, p.PR, was, post); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

// keepThreads keeps the open threads a poll read on each PR not
// merged or closed, for CountComments; a PR it couldn't read keeps the
// last count.
func (s *Service) keepThreads(a Answer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.threads == nil {
		s.threads = map[string]int{}
	}
	for _, list := range a.Branches {
		for _, f := range list {
			// Read without their threads.
			if f.State.Settled() {
				delete(s.threads, f.URL)
			}
		}
	}
	for _, f := range a.PRs {
		if f.State.Settled() {
			delete(s.threads, f.URL)
		} else {
			s.threads[f.URL] = f.Threads
		}
	}
}

// gitHubRepo is the GitHub repo of the checkout at path, asked of gh once.
func (s *Service) gitHubRepo(ctx context.Context, path string) (string, error) {
	s.mu.Lock()
	repo, ok := s.repoNames[path]
	s.mu.Unlock()
	if ok {
		return repo, nil
	}
	if s.GitHub == nil {
		return "", ErrNoGH
	}
	repo, err := s.GitHub.RepoOf(ctx, path)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	if s.repoNames == nil {
		s.repoNames = map[string]string{}
	}
	s.repoNames[path] = repo
	s.mu.Unlock()
	return repo, nil
}

// prHealth records pr's health, as track id's, which was in state was,
// drops the instructions it made moot, and posts @github's events on
// what changed when post is the track: an open Work track, or a side
// track that ended with its PR open, whose main track hears them.
func (s *Service) prHealth(ctx context.Context, id string, pr track.PR, was track.PRState, post *track.Track) error {
	if err := s.dropDone(ctx, pr); err != nil {
		return err
	}
	if pr.State == track.PRMerged && (was == track.PROpen || was == track.PRDraft) {
		if err := s.restack(ctx, id, pr); err != nil {
			return err
		}
	}
	before := pr.Health
	if !pr.State.Settled() {
		var err error
		if before, err = s.Store.SavePRHealth(ctx, id, pr.URL, pr.Health); err != nil {
			return err
		}
	}
	if post == nil {
		return nil
	}
	for _, e := range prEvents(pr, was, before) {
		if err := s.githubEvent(ctx, *post, e); err != nil {
			return err
		}
	}
	return nil
}

// branchTracks are the Work tracks whose branches may get PRs: those
// not closed, among the open and the last ended ones.
func (s *Service) branchTracks(ctx context.Context) ([]track.Track, error) {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return nil, err
	}
	done, err := s.Store.EndedTracks(ctx, ended)
	if err != nil {
		return nil, err
	}
	var out []track.Track
	for _, t := range append(open, done...) {
		if t.Kind == track.Work && !t.Archived() {
			out = append(out, t)
		}
	}
	return out, nil
}
