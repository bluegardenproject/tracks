package tracks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/track"
)

// FastReviewPrefix starts a Fast Review's name.
const FastReviewPrefix = "[Fast Review] "

// FastReview opens a Review track on the pull request at link, as
// Station's Fast Track does: on the repo of the Repositories tab whose
// origin is the PR's repo, with the Review type's agent, named after
// the PR's title. The window is named from the PR's number.
func (s *Service) FastReview(ctx context.Context, link string, progress func(string)) (Created, error) {
	pr, ok := track.ParsePR(strings.TrimSpace(link))
	if !ok {
		return Created{}, Problem("Paste a GitHub pull request link, such as https://github.com/owner/repo/pull/123.")
	}
	if progress == nil {
		progress = func(string) {}
	}
	repo, err := s.repoOf(ctx, pr.Repo)
	if err != nil {
		return Created{}, err
	}
	title := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
	if s.GitHub != nil {
		progress("Reading the pull request's title from GitHub…")
		got, err := s.GitHub.PRTitle(ctx, pr)
		if got = strings.Join(strings.Fields(plain(got)), " "); err == nil && got != "" {
			title = got
		}
	}
	return s.Create(ctx, Request{
		Kind: track.Review, Title: FastReviewPrefix + title, Repos: []string{repo},
		ReviewRef: pr.URL, NoDraft: true,
	}, progress)
}

// repoOf is the name of the Repositories tab's repo whose origin is
// the GitHub repo owner/name.
func (s *Service) repoOf(ctx context.Context, ownerName string) (string, error) {
	all, err := s.Store.Repos(ctx)
	if err != nil {
		return "", err
	}
	for _, r := range all {
		origin, err := git.NewPrimaryRepoClient(r.Path).Origin(ctx)
		if err == nil && strings.EqualFold(githubRepo(origin), ownerName) {
			return r.Name, nil
		}
	}
	return "", Problem("No repo on the Repositories tab has " + ownerName + " as its origin: add it there first.")
}

// githubRepo is owner/name of a git remote on github.com, "" for any
// other remote.
func githubRepo(remote string) string {
	remote = strings.TrimSpace(remote)
	var host, path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return ""
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp-like: [user@]host:path
		before, after, ok := strings.Cut(remote, ":")
		if !ok || strings.Contains(before, "/") {
			return ""
		}
		_, host, _ = strings.Cut(before, "@")
		if host == "" {
			host = before
		}
		path = after
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if !strings.EqualFold(host, "github.com") || strings.Count(path, "/") != 1 {
		return ""
	}
	return path
}

// PRTitle is pr's title, from gh pr view.
func (g GH) PRTitle(ctx context.Context, pr track.PR) (string, error) {
	out, err := g.run(ctx, "", "pr", "view", pr.URL, "--json", "title")
	if err != nil {
		return "", err
	}
	var v struct{ Title string }
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("gh pr view %s: %w", pr.URL, err)
	}
	return v.Title, nil
}
