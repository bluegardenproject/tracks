package tracks

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/termtext"
	"github.com/bluegardenproject/tracks/internal/track"
)

// addPR adds the pull request a Review track reviews to p, from
// GitHub. A worktree behind the PR is refused, unless stale: no stale
// head is reviewed without saying so. What GitHub can't give goes in
// p's notes, and the review goes on without it.
func (s *Service) addPR(ctx context.Context, t track.Track, p review.Packet, stale bool) (review.Packet, error) {
	pr, ok := track.ParsePR(t.ReviewRef)
	if t.Kind != track.Review || !ok || s.GitHub == nil {
		return p, nil
	}
	got, head, notes, err := s.GitHub.ReviewPR(ctx, pr)
	if err != nil {
		p.Notes = append(p.Notes, "the pull request on GitHub, so whether this worktree is at its head wasn't "+
			"checked either ("+err.Error()+")")
		return p, nil
	}
	if head != "" && p.Head != "" && head != p.Head {
		if !stale {
			return p, Problem(fmt.Sprintf("The pull request moved on: GitHub has %s, this worktree is at %s. "+
				"Ask the user whether to review the new head: git fetch origin pull/%d/head && git checkout --detach "+
				"FETCH_HEAD here, then tracks review again. tracks review --stale reviews what's here.",
				short(head), short(p.Head), pr.Number))
		}
		p.Notes = append(p.Notes, fmt.Sprintf("the pull request's head: GitHub has %s, the worktree and this "+
			"review are at %s, so say the review is of an older head", short(head), short(p.Head)))
	}
	p.PR, p.Notes = &got, append(p.Notes, notes...)
	return p, nil
}

// short is a commit's short form.
func short(sha string) string { return sha[:min(len(sha), 8)] }

// maxLogs bounds how many failed checks' logs a review fetches, and
// maxLogFetch how much of the end of each is read, more than the
// packet keeps.
const (
	maxLogs     = 3
	maxLogFetch = 64_000
)

// ReviewPR is what a review of pr is given from GitHub, and the commit
// its head is at. What couldn't be fetched beside the pull request
// itself is in notes. It runs gh with the PR's URL only, never in a
// worktree, so no repo's config is read.
func (g GH) ReviewPR(ctx context.Context, pr track.PR) (out review.PR, head string, notes []string, err error) {
	view, err := g.run(ctx, "", "pr", "view", pr.URL, "--json", "url,title,body,headRefOid,statusCheckRollup")
	if err != nil {
		return out, "", nil, err
	}
	out, head, err = parsePRView(view)
	if err != nil {
		return out, "", nil, fmt.Errorf("gh pr view %s: %w", pr.URL, err)
	}
	owner, name, _ := strings.Cut(pr.Repo, "/")
	threads, err := g.run(ctx, "", "api", "graphql", "-f", "query="+reviewThreadsQuery,
		"-f", "owner="+owner, "-f", "name="+name, "-F", "number="+strconv.Itoa(pr.Number))
	if err == nil {
		out.Threads, out.MoreThreads, err = parseThreads(threads)
	}
	if err != nil {
		notes = append(notes, "the review threads ("+err.Error()+")")
	}
	logs := 0
	for i, c := range out.Checks {
		repo, job := actionsJobOf(c.Link)
		if c.State != review.CheckFail || job == "" || !strings.EqualFold(repo, pr.Repo) || logs == maxLogs {
			continue
		}
		logs++
		log, err := g.runTail(ctx, "", maxLogFetch, "run", "view", "--job", job, "--log-failed", "-R", repo)
		if err != nil {
			notes = append(notes, "the log of "+c.Name+" ("+err.Error()+")")
			continue
		}
		out.Checks[i].Log = plain(string(log))
	}
	return out, head, notes, nil
}

// reviewThreadsQuery asks for a PR's review threads with their
// comments.
const reviewThreadsQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100) {
        totalCount
        nodes {
          isResolved isOutdated path line
          comments(last: 20) { totalCount nodes { author { login } body } }
        }
      }
    }
  }
}`

// parsePRView reads gh pr view's answer: the PR, its checks and its
// head.
func parsePRView(answer []byte) (review.PR, string, error) {
	var v struct {
		URL               string
		Title             string
		Body              string
		HeadRefOid        string
		StatusCheckRollup []struct {
			Typename   string `json:"__typename"`
			Name       string // a check run
			Status     string
			Conclusion string
			DetailsURL string `json:"detailsUrl"`
			Context    string // a commit status
			State      string
			TargetURL  string `json:"targetUrl"`
		}
	}
	if err := json.Unmarshal(answer, &v); err != nil {
		return review.PR{}, "", err
	}
	pr := review.PR{URL: v.URL, Title: plain(v.Title), Body: plain(v.Body)}
	for _, c := range v.StatusCheckRollup {
		check := review.Check{Name: plain(c.Name), Link: plain(c.DetailsURL)}
		if c.Typename == "StatusContext" {
			check = review.Check{Name: plain(c.Context), Link: plain(c.TargetURL)}
		}
		check.State = checkState(c.Status, c.Conclusion, c.State)
		pr.Checks = append(pr.Checks, check)
	}
	return pr, v.HeadRefOid, nil
}

// checkState is a check run's status and conclusion, or a commit
// status's state, as one of review's check states.
func checkState(status, conclusion, state string) string {
	switch {
	case state == "SUCCESS" || conclusion == "SUCCESS":
		return review.CheckPass
	case state == "FAILURE" || state == "ERROR" ||
		conclusion == "FAILURE" || conclusion == "TIMED_OUT" || conclusion == "CANCELLED" || conclusion == "ACTION_REQUIRED" || conclusion == "STARTUP_FAILURE":
		return review.CheckFail
	case conclusion == "SKIPPED" || conclusion == "NEUTRAL" || conclusion == "STALE":
		return review.CheckSkipped
	}
	return review.CheckPending
}

// parseThreads reads the unresolved threads of a reviewThreadsQuery
// answer, and how many threads it didn't hold, resolved or not.
func parseThreads(answer []byte) ([]review.Thread, int, error) {
	var a struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						TotalCount int
						Nodes      []struct {
							IsResolved, IsOutdated bool
							Path                   string
							Line                   int
							Comments               struct {
								TotalCount int
								Nodes      []struct {
									Author *struct{ Login string }
									Body   string
								}
							}
						}
					}
				}
			}
		}
	}
	if err := json.Unmarshal(answer, &a); err != nil {
		return nil, 0, err
	}
	all := a.Data.Repository.PullRequest.ReviewThreads
	var threads []review.Thread
	for _, n := range all.Nodes {
		if n.IsResolved {
			continue
		}
		t := review.Thread{Path: plain(n.Path), Line: n.Line, Outdated: n.IsOutdated}
		for _, c := range n.Comments.Nodes {
			author := "ghost"
			if c.Author != nil {
				author = plain(c.Author.Login)
			}
			t.Comments = append(t.Comments, review.Comment{Author: author, Body: plain(c.Body)})
		}
		t.EarlierComments = max(0, n.Comments.TotalCount-len(n.Comments.Nodes))
		threads = append(threads, t)
	}
	return threads, max(0, all.TotalCount-len(all.Nodes)), nil
}

// plain is text from GitHub without terminal escape sequences, control
// characters other than tabs and line ends, or invisible format
// characters: it goes into the reviewer's brief, and from its report to
// a terminal.
func plain(text string) string {
	return termtext.Multiline(text)
}

// actionsJob matches a GitHub Actions job's link. Owner must start with
// an alphanumeric or underscore so CLI flags are excluded; repo names
// may start with a dot (such as .github), except literal "." and "..".
var actionsJob = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_][\w.-]*/[\w.-]+)/actions/runs/\d+/job/(\d+)(?:$|[/?#])`)

// actionsJobOf is the repo and Actions job a check's link is to, "" for
// another link.
func actionsJobOf(link string) (repo, job string) {
	if m := actionsJob.FindStringSubmatch(link); m != nil {
		_, name, _ := strings.Cut(m[1], "/")
		if name == "." || name == ".." {
			return "", ""
		}
		return m[1], m[2]
	}
	return "", ""
}
