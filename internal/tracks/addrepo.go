package tracks

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/track"
)

// AddRepo gives work track id a worktree of the repo called name, from
// the Repositories tab, on the branch the track's first worktree is on
// now, since the agent may have renamed the one it started on. The
// worktree is removed again when saving it fails.
func (s *Service) AddRepo(ctx context.Context, id, name string, progress func(string)) (track.Repo, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return track.Repo{}, err
	}
	defer release()
	switch {
	case t.Kind == track.Ask || t.Kind == track.Plan:
		return track.Repo{}, Problem(t.Name + " has no worktrees: promote it first.")
	case t.Kind != track.Work:
		return track.Repo{}, Problem("Only Work tracks can add repos.")
	case t.Archived():
		return track.Repo{}, Problem(t.Name + " is archived: its worktrees are gone.")
	}

	all, err := s.Store.Repos(ctx)
	if err != nil {
		return track.Repo{}, err
	}
	var r track.Repo
	names := make([]string, 0, len(all))
	for _, known := range all {
		names = append(names, known.Name)
		if strings.EqualFold(known.Name, name) {
			r = track.Repo{RepoID: known.ID, Name: known.Name, Path: known.Path, Base: known.BaseBranch}
		}
	}
	if r.Name == "" {
		return track.Repo{}, Problem(fmt.Sprintf("No repo named %s on the Repositories tab. Repos: %s.", name, strings.Join(names, ", ")))
	}
	for _, in := range t.Repos {
		if in.RepoID == r.RepoID || strings.EqualFold(in.Name, r.Name) {
			return track.Repo{}, Problem(fmt.Sprintf("%s is already in %s.", r.Name, t.Name))
		}
	}
	if info, err := os.Stat(r.Path); err != nil || !info.IsDir() {
		return track.Repo{}, Problem(fmt.Sprintf("The folder of %s, %s, is gone.", r.Name, r.Path))
	}

	var branch string
	for _, in := range s.Worktrees.Branches(ctx, t) {
		if in.Branch != "" {
			branch = in.Branch
			break
		}
	}
	if branch == "" {
		return track.Repo{}, Problem(t.Name + " has no branch to add " + r.Name + " on.")
	}
	added, err := s.Worktrees.AddRepo(ctx, t.ID, r, branch, progress)
	if err != nil {
		return track.Repo{}, err
	}
	if err := s.Store.AddTrackRepo(ctx, t.ID, added); err != nil {
		_ = s.Worktrees.Remove(context.WithoutCancel(ctx), track.Track{ID: t.ID, Kind: t.Kind, Repos: []track.Repo{added}})
		return track.Repo{}, fmt.Errorf("save the repo: %w", err)
	}
	s.prepare(ctx, []track.Repo{added})
	t.Repos = append(t.Repos, added)
	s.startEager(context.WithoutCancel(ctx), t)
	return added, nil
}
