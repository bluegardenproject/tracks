package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteManagedFileCreatesWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tracks-reviewer.md")
	wrote, err := writeManagedFile(path, []byte("---\nx-tracks-managed: \"1\"\n---\nbody"))
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Error("did not write to an empty path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// Overwriting our own file is intended — it is how an upgrade reaches
// an existing install.
func TestWriteManagedFileReplacesItsOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.md")
	old := "---\nx-tracks-managed: \"1\"\n---\nversion one"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, err := writeManagedFile(path, []byte("---\nx-tracks-managed: \"1\"\n---\nversion two"))
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("refused to replace a tracks-managed file")
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "version two") {
		t.Errorf("upgrade did not land: %q", got)
	}
}

// The requirement: never destroy a file the user wrote. The paths
// tracks installs to are ones users populate themselves, and a name
// collision is entirely plausible — someone who wrote their own
// tracks-reviewer is exactly the person who would.
func TestWriteManagedFileRefusesSomebodyElsesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracks-reviewer.md")
	mine := "---\nname: tracks-reviewer\n---\nmy own agent, please do not delete"
	if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	wrote, err := writeManagedFile(path, []byte("---\nx-tracks-managed: \"1\"\n---\ntracks' version"))
	if err != nil {
		t.Fatalf("refusing should not be an error: %v", err)
	}
	if wrote {
		t.Error("reported writing over a file it should have left alone")
	}
	got, _ := os.ReadFile(path)
	if string(got) != mine {
		t.Errorf("user's file was modified:\n got: %q\nwant: %q", got, mine)
	}
}

// The marker has to be found where it actually lives, and not matched
// somewhere it doesn't count.
func TestHasManagedMarker(t *testing.T) {
	cases := map[string]struct {
		content string
		want    bool
	}{
		"frontmatter":                 {"---\nx-tracks-managed: \"1\"\nname: x\n---\n", true},
		"after name":                  {"---\nname: x\nx-tracks-managed: \"1\"\n---\n", true},
		"absent":                      {"---\nname: x\n---\nbody", false},
		"empty":                       {"", false},
		"mentioned in prose far down": {"---\nname: x\n---\n" + strings.Repeat("filler\n", 2000) + "x-tracks-managed: \"1\"\n", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := hasManagedMarker([]byte(c.content)); got != c.want {
				t.Errorf("hasManagedMarker = %v, want %v", got, c.want)
			}
		})
	}
}

// Every template tracks ships must carry the marker, or the very first
// upgrade would see its own file as somebody else's and stop updating
// it forever.
func TestShippedTemplatesCarryTheMarker(t *testing.T) {
	for name, tpl := range map[string]string{
		"skillTemplate":             skillTemplate,
		"reviewerAgentTemplate":     reviewerAgentTemplate,
		"docsReviewerAgentTemplate": docsReviewerAgentTemplate,
		"cursorRuleTemplate":        cursorRuleTemplate,
	} {
		if !hasManagedMarker([]byte(tpl)) {
			t.Errorf("%s has no %s marker — tracks would stop updating it after the first install", name, managedMarker)
		}
	}
}

// The release that introduces the marker must adopt the files tracks
// already installed. They carry no marker, so without this they read
// as the user's and tracks stops updating them forever — a silent,
// permanent regression on every existing install.
func TestWriteManagedFileAdoptsItsOwnPreMarkerFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracks-reviewer.md")
	want := "---\nx-tracks-managed: \"1\"\nname: tracks-reviewer\n---\nbody"
	preMarker := "---\nname: tracks-reviewer\n---\nbody" // what the old code wrote

	if err := os.WriteFile(path, []byte(preMarker), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, err := writeManagedFile(path, []byte(want))
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("did not adopt a file identical to the shipped one minus the marker")
	}
	got, _ := os.ReadFile(path)
	if string(got) != want {
		t.Errorf("adopted file not updated:\n got: %q\nwant: %q", got, want)
	}
}

// Adoption must be exact. A file that merely looks similar is the
// user's, and guessing is how their work gets destroyed.
func TestAdoptionRequiresAnExactMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracks-reviewer.md")
	want := "---\nx-tracks-managed: \"1\"\nname: tracks-reviewer\n---\nbody"
	nearly := "---\nname: tracks-reviewer\n---\nbody, with one word changed"

	if err := os.WriteFile(path, []byte(nearly), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, err := writeManagedFile(path, []byte(want))
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Error("adopted a file that was not an exact pre-marker copy")
	}
	got, _ := os.ReadFile(path)
	if string(got) != nearly {
		t.Error("a near-match was overwritten; adoption must be exact")
	}
}
