package agents

// PRCommentsSteps is how an agent works through the open review
// comments on its track's PRs: Claude's tracks-pr-comments skill and
// Cursor's rule of that name, under their own frontmatter. The footer's
// button sends a prompt naming them.
const PRCommentsSteps = `# tracks-pr-comments

You are running inside a ` + "`tracks`" + ` track. Work through the open review
comments on its pull requests: fix what should be fixed, then answer each
comment briefly.

## 1. Find the pull requests

In each of the track's worktrees:

` + "```bash" + `
gh pr list --head "$(git branch --show-current)" --state open --json number,url,baseRefName
` + "```" + `

When you were given a pull request's link, keep only that one: work in the
worktree it was found in, and leave the other PRs alone.

## 2. Collect the open comments

For each PR, read its review threads (OWNER, NAME and NUMBER come from its
URL):

` + "```bash" + `
gh api graphql -f owner=OWNER -f name=NAME -F number=NUMBER -f query='
query($owner: String!, $name: String!, $number: Int!) {
  viewer { login }
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100) {
        nodes {
          id isResolved isOutdated path line
          comments(first: 50) { nodes { author { __typename login } body url } }
        }
      }
    }
  }
}'
` + "```" + `

A thread is open when it isn't resolved and its last comment isn't the
viewer's. Also read the PR's top-level comments
(` + "`gh pr view <url> --comments`" + `) and keep those that ask for a change and
have no answer from the viewer yet.

Sort the authors: a **bot** has ` + "`__typename`" + ` ` + "`Bot`" + ` or a login ending in
` + "`[bot]`" + ` (Copilot, CodeRabbit, …); everyone else is a **person**.

## 3. Decide, then fix

For each open comment, decide: fix it, or leave the code as it is and say
why. Don't take a suggestion only because a bot made it, and don't refuse
one only because it's a bot's.

Make the fixes and run the tests. Fold each fix into the commit it belongs
to instead of adding a new one. After ` + "`git fetch origin`" + `, the branch's own
commits are those in ` + "`git log origin/<baseRefName>..HEAD`" + `, with the PR's
` + "`baseRefName`" + ` from step 1: on a stacked PR that's the branch below, not
` + "`main`" + `.

- One commit of its own: ` + "`git commit --amend --no-edit`" + `.
- Several: ` + "`git commit --fixup=<commit>`" + ` for each fix, then
  ` + "`GIT_SEQUENCE_EDITOR=: git rebase -i --autosquash --keep-base origin/<baseRefName>`" + `.
  If the rebase stops on a conflict, resolve it and ` + "`git rebase --continue`" + `,
  or ` + "`git rebase --abort`" + ` and tell the user.

## 4. Push before answering

Run the pre-push review your track's instructions ask for, then
` + "`git push --force-with-lease --force-if-includes`" + `. Never use plain ` + "`--force`" + `.
**Answer nothing until the push succeeded.** A reply that promises a fix the
branch doesn't have gets flagged again by the next review. If the push is
refused (a permission, a hook, branch protection), show the user the exact
command, wait until they have run it, and check that the remote branch has your
commits before you answer. If it fails for another reason, stop and tell
the user why.

## 5. Answer

Write one reply per comment, one or two sentences: what changed, or why
nothing did. Don't cite commit hashes: amending and rebasing change them.
If ` + "`${XDG_CONFIG_HOME:-~/.config}/tracks/comment-style.md`" + ` exists, read it first
and write every reply in the style it describes: it's how the user writes.

End every comment you post with a blank line and ` + "`" + `<!-- tracks -->` + "`" + `. GitHub hides
it; it tells Tracks you wrote the comment, not the user, so learning their
style leaves it out.

- **Bots:** post the reply, then resolve the thread.
- **People:** don't post yet. Show the user every draft in a table (author,
  comment, your reply) and post only those they approve, as they worded
  them. Leave people's threads for them to resolve.

Reply to a review thread, then resolve it:

` + "```bash" + `
gh api graphql -f thread=THREAD_ID -f body='…' -f query='
mutation($thread: ID!, $body: String!) {
  addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: $thread, body: $body}) { comment { url } }
}'
gh api graphql -f thread=THREAD_ID -f query='
mutation($thread: ID!) { resolveReviewThread(input: {threadId: $thread}) { thread { isResolved } } }'
` + "```" + `

Answer a top-level comment with ` + "`gh pr comment <url> --body '…'`" + `, quoting
the line you answer.

## 6. Report

End with a table: comment (author, file:line), what you did (fixed /
answered / left open), and whether the reply is posted.
`
