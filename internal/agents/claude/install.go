package claude

import (
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/agents"
)

// InstallHelpers writes what the prompts rely on into home's .claude:
// the docs reviewer subagent, and the add-repo and PR comments skills.
// A file there without the marker is the user's and stays untouched;
// its path is returned in skipped. The code reviewer subagent of
// earlier versions is removed: `tracks review` runs the reviewer now.
func InstallHelpers(home string) (skipped []string, err error) {
	dir := filepath.Join(home, ".claude")
	if err := agents.RemoveManaged(filepath.Join(dir, "agents", "tracks-reviewer.md")); err != nil {
		return nil, err
	}
	for _, f := range []struct{ name, content string }{
		{"agents/tracks-docs-reviewer.md", docsReviewerAgent},
		{"skills/tracks-add-repo/SKILL.md", addRepoSkill},
		{"skills/tracks-pr-comments/SKILL.md", prCommentsSkill},
	} {
		path := filepath.Join(dir, filepath.FromSlash(f.name))
		ok, err := agents.WriteManaged(path, []byte(f.content))
		if err != nil {
			return skipped, err
		}
		if !ok {
			skipped = append(skipped, path)
		}
	}
	return skipped, nil
}
