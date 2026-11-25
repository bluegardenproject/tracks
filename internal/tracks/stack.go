package tracks

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/side"
	"github.com/bluegardenproject/tracks/internal/store"
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
// to origin, where t's worktrees start from. lower's branches are read
// from its worktrees, which its agent may have renamed, and kept.
func (s *Service) stack(ctx context.Context, t *track.Track, lower track.Track) error {
	if lower.Kind != track.Work {
		// A Review track's checkout is someone else's branch, a Plan's
		// or an Ask's none: nothing Tracks restacks on.
		return Problem(shown(lower) + " is " + article(string(lower.Kind)) + " " + string(lower.Kind) + " track, with no branch of its own to build on.")
	}
	live := lower.Repos
	if !lower.Cleaned() {
		live = s.Worktrees.Branches(ctx, lower)
		for i, r := range live {
			if i < len(lower.Repos) && r.Branch != "" && r.Branch != lower.Repos[i].Branch {
				if err := s.Store.SetBranch(ctx, lower.ID, i, r.Branch); err != nil {
					return err
				}
			}
		}
	}
	stacked := false
	for i, r := range t.Repos {
		for _, lr := range live {
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
		if r.Base == defaults[r.Name] {
			// On the repo's base: not stacked, whoever else is on it.
			continue
		}
		for _, other := range family {
			if other.ID == t.ID || other.Kind != track.Work {
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

// restack tells the side tracks stacked on the branch of track id's
// PR pr, which merged, to rebase their own commits onto what it merged
// into, and records that as their base: those of id's family whose repo
// in pr's GitHub repo builds on pr's head branch. Always, whatever the
// policies: it's mechanical. A side track that's archived, or whose PRs
// there are merged or closed, as a fan-in's into its main track's
// branch, has nothing to restack; one that ended is its main track's to
// decide.
func (s *Service) restack(ctx context.Context, id string, pr track.PR) error {
	if pr.Health.HeadBranch == "" || pr.Health.Base == "" {
		return nil
	}
	from := pr.Health.HeadBranch
	lower, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	mainID := lower.ID
	if side.Is(lower) {
		mainID = lower.Parent
	}
	family, err := s.family(ctx, mainID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	// The base and the commit go into commands the agent runs: ones
	// that aren't safe there leave it to the main track.
	base, okBase := gitHubRef(pr.Health.Base)
	head := strings.ToLower(pr.Health.Head[:min(7, len(pr.Health.Head))])
	safe := okBase && shortSHA.MatchString(head)
	ref := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
	defaults := map[string]string{}
	if repos, err := s.Store.Repos(ctx); err == nil {
		for _, r := range repos {
			defaults[r.Name] = r.BaseBranch
		}
	}
	var errs []error
	for _, upper := range family {
		if upper.ID == lower.ID || !side.Is(upper) {
			continue
		}
		for _, r := range upper.Repos {
			// On the repo's base it isn't stacked, whatever merged from
			// that branch (a release PR, say).
			if r.Base != from || r.Base == defaults[r.Name] {
				continue
			}
			if repo, err := s.gitHubRepo(ctx, r.Path); err != nil || !strings.EqualFold(repo, pr.Repo) {
				continue
			}
			// One failing leaves the others to restack.
			errs = append(errs, s.restackOne(ctx, upper.ID, r.Name, mainID, pr, ref, shown(lower), base, head, safe))
		}
	}
	return errors.Join(errs...)
}

// restackOne restacks side track id's repo name, under Report's lock and
// read again, so one ending meanwhile hands the instruction over.
func (s *Service) restackOne(ctx context.Context, id, name, mainID string, pr track.PR, ref, lowerName, base, head string, safe bool) error {
	s.reporting.Lock()
	defer s.reporting.Unlock()
	upper, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if upper.Archived() || settledIn(upper, pr.Repo) {
		return nil
	}
	named := fmt.Sprintf("@%s, side track %s: the PR below it, %s, was merged into %s.", upper.Side, gitHubText(shown(upper)), prName(pr), gitHubText(pr.Health.Base))
	// What waits for it about the branch below is moot, ended or not.
	if err := s.dropBeforeRestack(ctx, upper, mainID, pr.Repo); err != nil {
		return err
	}
	if !safe {
		to := mainID
		if main, err := s.Store.Track(ctx, mainID); err == nil && !main.Open() && upper.Open() {
			// No main track to decide: the side track hears it.
			to = upper.ID
		}
		return s.postGitHub(ctx, to, named+" Its branch or commit isn't one Tracks puts into a command: you decide.", relay.Event)
	}
	if err := s.Store.SetBase(ctx, upper.ID, name, base); err != nil {
		return err
	}
	if !upper.Open() {
		return s.postGitHub(ctx, mainID, named+" It has ended: you decide.", relay.Event)
	}
	if err := s.instruct(ctx, upper.ID, instructionRestack, pr.URL, actions.SideRestack(ref, gitHubText(lowerName), base, head)); err != nil {
		return err
	}
	return s.postGitHub(ctx, mainID, named+" Tracks tells it to restack, once its agent is free.", relay.Notice)
}

// dropBeforeRestack drops what waits for side track upper that the
// restack about to be queued makes moot, as it would be pasted first: an
// earlier restack, and a rebase onto the branch below, its own or its
// main track's question about it, for its PRs in GitHub repo.
func (s *Service) dropBeforeRestack(ctx context.Context, upper track.Track, mainID, repo string) error {
	waiting, err := s.Store.WaitingInstructions(ctx)
	if err != nil {
		return err
	}
	mine := map[string]bool{}
	for _, pr := range upper.PRs {
		if strings.EqualFold(pr.Repo, repo) {
			mine[pr.URL] = true
		}
	}
	for _, in := range waiting {
		below, _ := track.ParsePR(in.PR)
		drop := in.TrackID == upper.ID && in.Kind == instructionRestack && strings.EqualFold(below.Repo, repo) ||
			in.TrackID == upper.ID && in.Kind == settings.EventConflicts && mine[in.PR] ||
			in.TrackID == mainID && in.Kind == askKind(settings.EventConflicts) && mine[in.PR]
		if drop {
			if err := s.Store.DropInstruction(ctx, in.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// settledIn reports whether t has PRs in GitHub repo and all of them
// are merged or closed.
func settledIn(t track.Track, repo string) bool {
	some := false
	for _, pr := range t.PRs {
		if !strings.EqualFold(pr.Repo, repo) {
			continue
		}
		if !pr.State.Settled() {
			return false
		}
		some = true
	}
	return some
}

// shown is t's name as the Station shows it.
func shown(t track.Track) string { return cmp.Or(t.Title, t.Name) }

// restackWaits reports whether a restack waits for side track id in
// GitHub repo: a conflicts event there is the one the restack solves.
func (s *Service) restackWaits(ctx context.Context, id, repo string) (bool, error) {
	waiting, err := s.Store.WaitingInstructions(ctx)
	if err != nil {
		return false, err
	}
	for _, in := range waiting {
		if below, ok := track.ParsePR(in.PR); ok && in.TrackID == id && in.Kind == instructionRestack && strings.EqualFold(below.Repo, repo) {
			return true, nil
		}
	}
	return false, nil
}

// article is "a" or "an" before word.
func article(word string) string {
	if word != "" && strings.ContainsRune("aeiou", rune(word[0])) {
		return "an"
	}
	return "a"
}
