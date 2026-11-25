package tracks

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/workspace"
)

// Request is a new track as the form enters it.
type Request struct {
	Kind track.Kind `json:"kind"`
	Name string     `json:"name,omitempty"`
	// Title is the name the track is shown under when it isn't Name,
	// such as a Fast Review's; its window is named from the rest. It's
	// kept whole: the repos' prefixes go before Name only.
	Title string `json:"title,omitempty"`
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
	// Engine is the engine picked for this track, "" for the type's.
	// Model goes with it: "" is the engine's default from the Engines
	// tab.
	Engine string `json:"engine,omitempty"`
	Model  string `json:"model,omitempty"`
	// Draft is the draft a failure keeps the request as, and a success
	// deletes; "" is a new one.
	Draft string `json:"draft,omitempty"`
	// NoDraft keeps no draft when the track can't be made: the caller,
	// such as an agent running tracks new, reports the error itself.
	NoDraft bool `json:"no_draft,omitempty"`
	// Parent is the ID of the track tracks new ran in, "" for none.
	Parent string `json:"parent,omitempty"`
	// Side makes the track a side track of Parent, its main track.
	Side bool `json:"side,omitempty"`
	// On stacks a side track on its main track ("track") or one of its
	// side tracks, by handle: its shared repos branch from that one's.
	On string `json:"on,omitempty"`
}

// check turns req into a track, checking it again as the form does:
// the repos exist, the kind's rules hold and the document can be read.
// It also returns the repos that open draft pull requests, and the
// prefix its name is shown under.
func (s *Service) check(ctx context.Context, req Request) (track.Track, []string, string, error) {
	t := track.Track{
		Kind: req.Kind, Name: strings.TrimSpace(req.Name), Title: cmp.Or(strings.TrimSpace(req.Title), strings.TrimSpace(req.Name)), Prompt: req.Prompt,
		Opinion: true, ClaimCheck: true,
	}
	if !t.Kind.Valid() {
		return t, nil, "", Problem(fmt.Sprintf("There's no track type %q.", req.Kind))
	}
	if t.Kind != track.Review && strings.TrimSpace(req.Prompt) == "" {
		return t, nil, "", Problem("Enter a prompt.")
	}

	all, err := s.Store.Repos(ctx)
	if err != nil {
		return t, nil, "", err
	}
	var drafts, prefixes []string
	seen := map[int64]bool{}
	for _, name := range req.Repos {
		i := -1
		for j, r := range all {
			if strings.EqualFold(r.Name, name) {
				i = j
			}
		}
		if i < 0 {
			return t, nil, "", Problem(fmt.Sprintf("There's no repo %s on the Repositories tab. Repos: %s.", name, knownRepos(all)))
		}
		r := all[i]
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if info, err := os.Stat(r.Path); err != nil || !info.IsDir() {
			return t, nil, "", Problem(fmt.Sprintf("The folder of %s, %s, is gone.", r.Name, r.Path))
		}
		t.Repos = append(t.Repos, track.Repo{RepoID: r.ID, Name: r.Name, Path: r.Path, Base: r.BaseBranch})
		if r.Prefix != "" && !slices.Contains(prefixes, r.Prefix) {
			prefixes = append(prefixes, r.Prefix)
		}
		if r.DraftPRs {
			drafts = append(drafts, r.Name)
		}
	}

	switch t.Kind {
	case track.Work:
		if len(t.Repos) == 0 {
			return t, nil, "", Problem("Pick at least one repo.")
		}
	case track.Review:
		if len(t.Repos) != 1 {
			return t, nil, "", Problem("Pick the one repo to review.")
		}
		t.ReviewRef = strings.TrimSpace(req.ReviewRef)
		if t.ReviewRef == "" {
			return t, nil, "", Problem("Enter a pull request link or a branch name.")
		}
		if _, err := workspace.ParseReview(t.ReviewRef); err != nil {
			return t, nil, "", Problem(t.ReviewRef + " isn't a GitHub pull request link or a branch name.")
		}
	case track.Doc:
		if t.Document, err = ResolveDocument(req.Document); err != nil {
			return t, nil, "", err
		}
		t.Opinion, t.ClaimCheck = req.Opinion, req.ClaimCheck
	}

	if t.Kind == track.Review || t.Kind == track.Doc {
		switch {
		case req.Candor == 0:
			t.Candor = track.DefaultCandor
		case req.Candor < track.MinCandor || req.Candor > track.MaxCandor:
			return t, nil, "", Problem(fmt.Sprintf("Candor goes from %d to %d.", track.MinCandor, track.MaxCandor))
		default:
			t.Candor = req.Candor
		}
	}
	t.Terminal = req.Terminal && t.Kind.Worktrees()
	t.Parent = strings.TrimSpace(req.Parent)
	prefix := ""
	if strings.TrimSpace(req.Title) == "" && len(t.Repos) <= track.MaxPrefixed {
		prefix = strings.Join(prefixes, " ")
	}
	return t, drafts, prefix, nil
}

// prefixed is name shown under prefix, the repos' prefixes.
func prefixed(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + " " + name
}

// knownRepos lists the Repositories tab's repos, for a message.
func knownRepos(repos []store.Repo) string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	if len(names) == 0 {
		return "none yet"
	}
	return strings.Join(names, ", ")
}
