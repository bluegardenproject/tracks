package tracks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/track"
)

// BranchQuery asks for the pull requests from a branch: Repo is the
// GitHub repo, owner/name.
type BranchQuery struct{ Repo, Branch string }

// Query is what one poll asks GitHub: the PRs from branches, and PRs by
// their links.
type Query struct {
	Branches []BranchQuery
	PRs      []track.PR
}

// PRFacts is a PR as GitHub has it, with its health, and Threads, its
// review threads that wait on the gh user: unresolved, with someone
// else having the last word. Threads are read only for the PRs asked
// for by their links.
type PRFacts struct {
	track.PR
	Threads int
}

// Answer is GitHub's answer to a Query: each branch's PRs, and each PR
// asked for, by its link. What couldn't be read is missing.
type Answer struct {
	Branches map[BranchQuery][]PRFacts
	PRs      map[string]PRFacts
}

// pollChunk is how many branches and PRs one GraphQL query asks for.
const pollChunk = 25

// prFragments are what a poll reads of a PR: pr its state and health,
// prThreads its review threads too. GitHub charges a query by the most
// it could return, so a branch's PRs, up to 10 each with up to 100
// threads, read no threads: those would cost ten times as much. The PRs
// found there are asked for by their links from the next poll on.
const prFragments = `fragment pr on PullRequest {
  url state isDraft mergeable baseRefName headRefOid
  commits(last: 1) { nodes { commit { statusCheckRollup { contexts(first: 100) { nodes {
    __typename ... on CheckRun { name status conclusion } ... on StatusContext { context state } } } } } } }
}
fragment prThreads on PullRequest {
  ...pr
  reviewThreads(first: 100) { nodes { isResolved comments(last: 1) { nodes { author { login } } } } }
}`

// pollQuery is the GraphQL query for branches and prs, an alias each:
// b0, b1, … and p0, p1, …. Names go in as JSON strings, which GraphQL
// reads the same.
func pollQuery(branches []BranchQuery, prs []track.PR) string {
	var b strings.Builder
	b.WriteString("query {\n  viewer { login }\n")
	for i, q := range branches {
		owner, name, _ := strings.Cut(q.Repo, "/")
		fmt.Fprintf(&b, "  b%d: repository(owner: %s, name: %s) { pullRequests(headRefName: %s, first: 10, "+
			"orderBy: {field: CREATED_AT, direction: DESC}) { nodes { ...pr } } }\n", i, quote(owner), quote(name), quote(q.Branch))
	}
	for i, pr := range prs {
		owner, name, _ := strings.Cut(pr.Repo, "/")
		fmt.Fprintf(&b, "  p%d: repository(owner: %s, name: %s) { pullRequest(number: %d) { ...prThreads } }\n", i, quote(owner), quote(name), pr.Number)
	}
	b.WriteString("}\n")
	b.WriteString(prFragments)
	return b.String()
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// gqlPR is a PR in a poll's answer.
type gqlPR struct {
	URL       string `json:"url"`
	State     string `json:"state"`
	IsDraft   bool   `json:"isDraft"`
	Mergeable string `json:"mergeable"`
	Base      string `json:"baseRefName"`
	Head      string `json:"headRefOid"`
	Commits   struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct{ Nodes []ghCheck }
				}
			}
		}
	}
	ReviewThreads struct {
		Nodes []struct {
			IsResolved bool
			Comments   struct {
				Nodes []struct {
					Author *struct{ Login string }
				}
			}
		}
	}
}

// facts reads p, whose threads wait on viewer when they're someone
// else's.
func (p gqlPR) facts(viewer string) (PRFacts, bool) {
	var checks []ghCheck
	if n := p.Commits.Nodes; len(n) > 0 && n[0].Commit.StatusCheckRollup != nil {
		checks = n[0].Commit.StatusCheckRollup.Contexts.Nodes
	}
	pr, ok := ghPR{URL: p.URL, State: p.State, IsDraft: p.IsDraft, Base: p.Base, Head: p.Head,
		Mergeable: p.Mergeable, Checks: checks}.pr()
	if !ok {
		return PRFacts{}, false
	}
	f := PRFacts{PR: pr}
	for _, t := range p.ReviewThreads.Nodes {
		if t.IsResolved || len(t.Comments.Nodes) == 0 {
			continue
		}
		if a := t.Comments.Nodes[len(t.Comments.Nodes)-1].Author; a == nil || a.Login != viewer {
			f.Threads++
		}
	}
	return f, true
}

// pollAnswer reads a pollQuery's answer for branches and prs into a,
// and says what it couldn't read.
func pollAnswer(out []byte, branches []BranchQuery, prs []track.PR, a Answer) error {
	var r struct {
		Data   map[string]json.RawMessage
		Errors []struct{ Path []any }
	}
	if err := json.Unmarshal(out, &r); err != nil || r.Data == nil {
		return fmt.Errorf("gh api graphql: no answer to read")
	}
	var viewer struct{ Login string }
	if json.Unmarshal(r.Data["viewer"], &viewer) != nil || viewer.Login == "" {
		// Every thread would seem to wait on the user.
		return fmt.Errorf("gh api graphql: the answer doesn't say who asks")
	}
	// An alias with an error anywhere in it may be missing a field,
	// which would read as a change: it counts as unanswered.
	failed := map[string]bool{}
	for _, e := range r.Errors {
		if len(e.Path) > 0 {
			if alias, ok := e.Path[0].(string); ok {
				failed[alias] = true
			}
		}
	}
	answer := func(alias string) json.RawMessage {
		if failed[alias] {
			return nil
		}
		return r.Data[alias]
	}
	var missing []string
	for i, q := range branches {
		var repo *struct {
			PullRequests struct{ Nodes []gqlPR }
		}
		if json.Unmarshal(answer(fmt.Sprintf("b%d", i)), &repo) != nil || repo == nil {
			missing = append(missing, q.Repo+" "+q.Branch)
			continue
		}
		list := []PRFacts{}
		for _, p := range repo.PullRequests.Nodes {
			if f, ok := p.facts(viewer.Login); ok {
				list = append(list, f)
			}
		}
		a.Branches[q] = list
	}
	for i, pr := range prs {
		var repo *struct{ PullRequest *gqlPR }
		if json.Unmarshal(answer(fmt.Sprintf("p%d", i)), &repo) != nil || repo == nil || repo.PullRequest == nil {
			missing = append(missing, pr.URL)
			continue
		}
		f, ok := repo.PullRequest.facts(viewer.Login)
		if !ok {
			missing = append(missing, pr.URL)
			continue
		}
		// The link asked for, which the store knows it by.
		f.URL = pr.URL
		a.PRs[pr.URL] = f
	}
	if len(missing) > 0 {
		return fmt.Errorf("GitHub didn't answer for %s", strings.Join(missing, ", "))
	}
	return nil
}

// Poll asks GitHub about q in GraphQL queries of pollChunk branches and
// PRs each. It answers what it could read, and says what it couldn't.
func (g GH) Poll(ctx context.Context, q Query) (Answer, error) {
	a := Answer{Branches: map[BranchQuery][]PRFacts{}, PRs: map[string]PRFacts{}}
	branches, prs := q.Branches, q.PRs
	var errs []error
	for len(branches)+len(prs) > 0 {
		nb := min(len(branches), pollChunk)
		np := min(len(prs), pollChunk-nb)
		out, err := g.run(ctx, "", "api", "graphql", "-f", "query="+pollQuery(branches[:nb], prs[:np]))
		if errors.Is(err, ErrNoGH) || ctx.Err() != nil {
			return a, errors.Join(err, ctx.Err())
		}
		if readErr := pollAnswer(out, branches[:nb], prs[:np], a); err != nil || readErr != nil {
			// gh's own error says why, when it has one.
			errs = append(errs, errors.Join(err, readErr))
		}
		branches, prs = branches[nb:], prs[np:]
	}
	return a, errors.Join(errs...)
}

// RepoOf is the GitHub repo, owner/name, that gh takes the checkout at
// dir for: where its branches' PRs are.
func (g GH) RepoOf(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, "repo", "view", "--json", "nameWithOwner")
	if err != nil {
		return "", err
	}
	var r struct{ NameWithOwner string }
	if err := json.Unmarshal(out, &r); err != nil || !strings.Contains(r.NameWithOwner, "/") {
		return "", fmt.Errorf("gh repo view in %s: no repo to read", dir)
	}
	return r.NameWithOwner, nil
}
