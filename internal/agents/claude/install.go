package claude

import (
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/agents"
)

// InstallHelpers writes what the prompts rely on into home's .claude:
// the reviewer subagents, and the add-repo skill. A file there without
// the marker is the user's and stays untouched; its path is returned
// in skipped.
func InstallHelpers(home string) (skipped []string, err error) {
	dir := filepath.Join(home, ".claude")
	for _, f := range []struct{ name, content string }{
		{"agents/tracks-v2-reviewer.md", reviewerAgent},
		{"agents/tracks-v2-docs-reviewer.md", docsReviewerAgent},
		{"skills/tracks-v2-add-repo/SKILL.md", addRepoSkill},
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
