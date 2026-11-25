package tracks

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/side"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/train"
)

// instructionRestack is a stacked side track's instruction to rebase
// once the PR below it merged.
const instructionRestack = "restack"

// stackOn is the track a side track of main track main is stacked on:
// on is "track", the main track itself, or one of its side tracks by
// handle or prefix. One whose PRs are all merged or closed is refused:
// there's nothing left to build on.
func (s *Service) stackOn(ctx context.Context, main track.Track, on string) (track.Track, error) {
	if on == relay.TrackHandle || on == "@"+relay.TrackHandle {
		return main, nil
	}
	sides, err := s.Store.SideTracks(ctx, main.ID)
	if err != nil {
		return track.Track{}, err
	}
	handles := make([]string, len(sides))
	for i, t := range sides {
		handles[i] = t.Side
	}
	handle, err := train.Resolve(on, handles)
	if err != nil {
		return track.Track{}, Problem("--on: " + err.Error() + ": tracks side list lists the side tracks, and track is this one.")
	}
	lower := sides[slices.Index(handles, handle)]
	if len(lower.PRs) > 0 && !slices.ContainsFunc(lower.PRs, func(pr track.PR) bool { return !pr.State.Settled() }) {
		return track.Track{}, Problem(shown(lower) + "'s pull requests are merged or closed: start the side track from the repo's base, without --on.")
	}
	return lower, nil
}

// stack builds t's repos that lower has too on lower's branches, pushed
// to origin, where t's worktrees start from.
func (s *Service) stack(ctx context.Context, t *track.Track, lower track.Track) error {
	stacked := false
	for i, r := range t.Repos {
		for _, lr := range lower.Repos {
			if lr.Name != r.Name || lr.Branch == "" {
				continue
			}
			pushed, err := s.Worktrees.Pushed(ctx, r.Path, lr.Branch)
			if err != nil {
				return err
			}
			if !pushed {
				return Problem(fmt.Sprintf("Push %s's branch %s in %s first: a stacked side track starts from it on origin.",
					shown(lower), lr.Branch, r.Name))
			}
			t.Repos[i].Base, stacked = lr.Branch, true
		}
	}
	if !stacked {
		return Problem(shown(lower) + " has none of the side track's repos to build on.")
	}
	return nil
}

// sideSpec is what side track t's prompt names: its main track, its
// handle, and its repos built on a branch of its main track's or of a
// sibling's, the PRs they go against.
func (s *Service) sideSpec(ctx context.Context, t track.Track) *agents.SideTrack {
	if !side.Is(t) {
		return nil
	}
	spec := &agents.SideTrack{Main: t.ParentName, Handle: t.Side}
	family, err := s.family(ctx, t.Parent)
	if err != nil {
		return spec
	}
	defaults := map[string]string{}
	if repos, err := s.Store.Repos(ctx); err == nil {
		for _, r := range repos {
			defaults[r.Name] = r.BaseBranch
		}
	}
	for _, r := range t.Repos {
		for _, other := range family {
			if other.ID == t.ID {
				continue
			}
			if i := slices.IndexFunc(other.Repos, func(o track.Repo) bool { return o.Name == r.Name && o.Branch != "" && o.Branch == r.Base }); i >= 0 {
				spec.Stacked = append(spec.Stacked, agents.Stack{Repo: r.Name, On: shown(other), Branch: r.Base,
					Default: cmp.Or(defaults[r.Name], "the repo's base")})
				break
			}
		}
	}
	return spec
}

// family is main track id and its side tracks.
func (s *Service) family(ctx context.Context, id string) ([]track.Track, error) {
	main, err := s.Store.Track(ctx, id)
	if err != nil {
		return nil, err
	}
	sides, err := s.Store.SideTracks(ctx, id)
	if err != nil {
		return nil, err
	}
	return append([]track.Track{main}, sides...), nil
}

// restack tells the side tracks stacked on track id's branches, whose
// PR pr merged, to rebase their own commits onto what it merged into,
// and records that as their base. Always, whatever the policies: it's
// mechanical. One that ended is its main track's to decide.
func (s *Service) restack(ctx context.Context, id string, pr track.PR) error {
	lower, err := s.Store.Track(ctx, id)
	if err != nil {
		return nil
	}
	mainID := lower.ID
	if side.Is(lower) {
		mainID = lower.Parent
	}
	family, err := s.family(ctx, mainID)
	if err != nil {
		return nil
	}
	base := gitHubText(pr.Health.Base)
	if base == "" || len(pr.Health.Head) < 7 {
		return nil
	}
	head := pr.Health.Head[:7]
	ref := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
	for _, upper := range family {
		if upper.ID == lower.ID || !side.Is(upper) {
			continue
		}
		for _, r := range upper.Repos {
			if !slices.ContainsFunc(lower.Repos, func(lr track.Repo) bool {
				return lr.Name == r.Name && lr.Branch != "" && lr.Branch == r.Base
			}) {
				continue
			}
			if err := s.Store.SetBase(ctx, upper.ID, r.Name, base); err != nil {
				return err
			}
			named := fmt.Sprintf("@%s, side track %s: the PR below its, %s, was merged into %s.", upper.Side, gitHubText(shown(upper)), prName(pr), base)
			if !upper.Open() {
				if err := s.postGitHub(ctx, mainID, named+" It has ended: you decide.", relay.Event); err != nil {
					return err
				}
				continue
			}
			if err := s.instruct(ctx, upper.ID, instructionRestack, pr.URL, actions.SideRestack(ref, gitHubText(shown(lower)), base, head)); err != nil {
				return err
			}
			if err := s.postGitHub(ctx, mainID, named+" Tracks tells it to restack, once its agent is free.", relay.Notice); err != nil {
				return err
			}
		}
	}
	return nil
}

// shown is t's name as the Station shows it.
func shown(t track.Track) string { return cmp.Or(t.Title, t.Name) }
