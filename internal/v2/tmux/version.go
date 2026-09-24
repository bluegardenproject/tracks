package tmux

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Version is a tmux release number. Zero means it couldn't be parsed
// (for example a build from master), which is treated as recent.
type Version struct{ Major, Minor int }

// MinVersion is the oldest supported tmux: it's the first with
// extended keys and terminal-features.
var MinVersion = Version{3, 2}

// AtLeast reports whether v is want or newer. An unknown version
// counts as new enough.
func (v Version) AtLeast(want Version) bool {
	if v == (Version{}) {
		return true
	}
	return v.Major > want.Major || v.Major == want.Major && v.Minor >= want.Minor
}

func (v Version) String() string {
	if v == (Version{}) {
		return "unknown"
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)`)

// ParseVersion reads the output of `tmux -V`, such as "tmux 3.5a" or
// "tmux next-3.6".
func ParseVersion(out string) Version {
	m := versionPattern.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return Version{}
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return Version{major, minor}
}

// InstalledVersion finds tmux on PATH and checks it's supported.
func InstalledVersion() (Version, error) {
	out, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		return Version{}, fmt.Errorf("tmux not found; Tracks needs tmux %s or newer", MinVersion)
	}
	v := ParseVersion(string(out))
	if !v.AtLeast(MinVersion) {
		return v, fmt.Errorf("tmux %s is too old; Tracks needs %s or newer", v, MinVersion)
	}
	return v, nil
}
