package side

import "strings"

// Opened is what tracks side new tells the main track's agent: the side
// track's name, its handle and repos, and that its pull request opens
// as a draft for the main track to check and mark ready, but for those
// in keepDrafts, the repos whose Draft PRs rule keeps them drafts for
// the user. With no repos, from a daemon older than the CLI, it names
// only the side track.
func Opened(name, handle string, repos, keepDrafts []string) string {
	out := "Opened side track " + name + " (@" + handle + ")"
	if len(repos) == 0 {
		return out + ". It reports to you on the relay."
	}
	out += ", a Work track on " + strings.Join(repos, ", ") + ". " +
		"It reports to you on the relay. It opens its pull request as a draft: when it reports one, check the PR " +
		"against the task you gave"
	if len(keepDrafts) == len(repos) {
		return out + ", and leave it a draft: the repo's rule keeps it one for the user."
	}
	out += ", then mark it ready with `gh pr ready <url>`."
	if len(keepDrafts) > 0 {
		out += " PRs in " + strings.Join(keepDrafts, ", ") + " stay drafts by the repo's rule: leave those for the user."
	}
	return out
}
