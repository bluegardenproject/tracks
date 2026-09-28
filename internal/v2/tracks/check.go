package tracks

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

// Request is a new track as the form enters it.
type Request struct {
	Kind track.Kind `json:"kind"`
	Name string     `json:"name,omitempty"`
	// Repos are the repos' names, in the order picked.
	Repos     []string `json:"repos,omitempty"`
	ReviewRef string   `json:"review_ref,omitempty"`
	// Document is the path to review, absolute: the daemon runs in
	// another folder than the form.
	Document   string `json:"document,omitempty"`
	Prompt     string `json:"prompt"`
	Candor     int    `json:"candor,omitempty"`
	Opinion    bool   `json:"opinion,omitempty"`
	ClaimCheck bool   `json:"claim_check,omitempty"`
	Terminal   bool   `json:"terminal,omitempty"`
}

// check turns req into a track, checking it again as the form does:
// the repos exist, the kind's rules hold and the document can be read.
// It also returns the repos that open draft pull requests.
func (s *Service) check(ctx context.Context, req Request) (track.Track, []string, error) {
	t := track.Track{
		Kind: req.Kind, Name: strings.TrimSpace(req.Name), Prompt: req.Prompt,
		Opinion: true, ClaimCheck: true,
	}
	if !t.Kind.Valid() {
		return t, nil, Problem(fmt.Sprintf("There's no track type %q.", req.Kind))
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return t, nil, Problem("Enter a prompt.")
	}

	all, err := s.Store.Repos(ctx)
	if err != nil {
		return t, nil, err
	}
	var drafts []string
	seen := map[int64]bool{}
	for _, name := range req.Repos {
		i := -1
		for j, r := range all {
			if strings.EqualFold(r.Name, name) {
				i = j
			}
		}
		if i < 0 {
			return t, nil, Problem(fmt.Sprintf("There's no repo %s on the Repositories tab.", name))
		}
		r := all[i]
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if info, err := os.Stat(r.Path); err != nil || !info.IsDir() {
			return t, nil, Problem(fmt.Sprintf("The folder of %s, %s, is gone.", r.Name, r.Path))
		}
		t.Repos = append(t.Repos, track.Repo{RepoID: r.ID, Name: r.Name, Path: r.Path, Base: r.BaseBranch})
		if r.DraftPRs {
			drafts = append(drafts, r.Name)
		}
	}

	switch t.Kind {
	case track.Work:
		if len(t.Repos) == 0 {
			return t, nil, Problem("Pick at least one repo.")
		}
	case track.Review:
		if len(t.Repos) != 1 {
			return t, nil, Problem("Pick the one repo to review.")
		}
		t.ReviewRef = strings.TrimSpace(req.ReviewRef)
		if t.ReviewRef == "" {
			return t, nil, Problem("Enter a pull request link or a branch name.")
		}
		if _, err := workspace.ParseReview(t.ReviewRef); err != nil {
			return t, nil, Problem(t.ReviewRef + " isn't a GitHub pull request link or a branch name.")
		}
	case track.Doc:
		if t.Document, err = ResolveDocument(req.Document); err != nil {
			return t, nil, err
		}
		t.Opinion, t.ClaimCheck = req.Opinion, req.ClaimCheck
	}

	if t.Kind == track.Review || t.Kind == track.Doc {
		switch {
		case req.Candor == 0:
			t.Candor = track.DefaultCandor
		case req.Candor < track.MinCandor || req.Candor > track.MaxCandor:
			return t, nil, Problem(fmt.Sprintf("Candor goes from %d to %d.", track.MinCandor, track.MaxCandor))
		default:
			t.Candor = req.Candor
		}
	}
	t.Terminal = req.Terminal && t.Kind.Worktrees()
	return t, drafts, nil
}
