package review

import (
	"slices"
	"testing"
)

// The cases follow GitHub's own CODEOWNERS examples.
func TestCodeowners(t *testing.T) {
	c := ParseCodeowners(`# Comments and blank lines are skipped.

*       @global-owner1 @global-owner2
*.js    @js-owner #This is an inline comment.
*.go docs@example.com
*.txt @octo-org/octocats
/build/logs/ @doctocat
docs/*  docs@example.com
apps/ @octocat
/docs/ @doctocat
/scripts/ @doctocat @octocat
**/logs @octocat
/apps/ @octocat
/apps/github
/a\ b.md @spaced
!negated.md @nobody
[ab].md @nobody
`)
	for path, want := range map[string][]string{
		"README.md":                         {"@global-owner1", "@global-owner2"},
		"src/app.js":                        {"@js-owner"},
		"main.go":                           {"docs@example.com"},
		"notes/a.txt":                       {"@octo-org/octocats"},
		"build/logs/today.log":              {"@octocat"}, // **/logs comes later
		"build/x.md":                        {"@global-owner1", "@global-owner2"},
		"docs/getting-started.md":           {"@doctocat"},
		"docs/build-app/troubleshooting.md": {"@doctocat"},
		"src/docs/x.md":                     {"@global-owner1", "@global-owner2"}, // docs/* is anchored
		"scripts/run.sh":                    {"@doctocat", "@octocat"},
		"deep/logs/x.md":                    {"@octocat"},
		"apps/web/x.md":                     {"@octocat"},
		"apps/github/x.md":                  nil,
		"a b.md":                            {"@spaced"},
		"negated.md":                        {"@global-owner1", "@global-owner2"},
		"a.md":                              {"@global-owner1", "@global-owner2"},
	} {
		if got := c.Owners(path); !slices.Equal(got, want) {
			t.Errorf("%s: owners %q, want %q", path, got, want)
		}
	}
	if !c.Owns("JS-Owner", "src/app.js") || c.Owns("@octocat", "src/app.js") || !c.Owns("docs@example.com", "main.go") {
		t.Error("an owner wasn't matched by handle, without its @ and in any case")
	}
	for pattern, path := range map[string]string{
		"/packages/web-* @o": "packages/web-ui/src/a.ts",
		"*.js @o":            "lib.js/x",
		"\\*.md @o":          "*.md",
		"a\\ b/ @o":          "x/a b/c",
		"/café/ @o":          "café/menü.md",
	} {
		if !ParseCodeowners(pattern).Owns("@o", path) {
			t.Errorf("%q doesn't own %s", pattern, path)
		}
	}
	if ParseCodeowners("\\*.md @o").Owns("@o", "a.md") || ParseCodeowners("\\#x @o").Owners("#x") != nil {
		t.Error("an escape was taken for a wildcard, or a \\# line wasn't skipped")
	}
	root := ParseCodeowners("/* @root")
	if !root.Owns("@root", "a.go") || root.Owns("@root", "src/a.go") {
		t.Error("/* should match only the files at the root")
	}
	star := ParseCodeowners("docs/* @d")
	if !star.Owns("@d", "docs/a.md") || star.Owns("@d", "docs/a/b.md") || star.Owns("@d", "src/docs/a.md") {
		t.Error("docs/* should match only the files directly in the root's docs")
	}
	if ParseCodeowners("").Owners("x") != nil {
		t.Error("an empty CODEOWNERS owns something")
	}
}
