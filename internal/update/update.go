// Package update checks GitHub for a newer tracks release and installs
// it over the running binary.
//
// Until now the only upgrade path was re-running scripts/install.sh from
// a shell outside the tmux session. Doing the same three steps in-process
// — read the latest release, pick the asset for this platform, swap the
// binary — lets the menu offer a "Check for updates" entry and keeps the
// asset naming in one place next to the installer's.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// releaseAPI is the unauthenticated "latest release" endpoint. The
	// repo is public, so no token is involved and the anonymous rate
	// limit is far above what a manual check costs.
	releaseAPI = "https://api.github.com/repos/bluegardenproject/tracks/releases/latest"

	// ReleasesPage is where callers point the user when the latest
	// release carries no asset for their platform.
	ReleasesPage = "https://github.com/bluegardenproject/tracks/releases"
)

// Release is the subset of a GitHub release the update flow needs.
type Release struct {
	Tag      string // as published, e.g. "v0.5.1"
	Version  string // Tag without the leading "v"
	PageURL  string // human-readable release page
	AssetURL string // download URL for this OS/arch; "" when absent
}

var client = &http.Client{Timeout: 5 * time.Minute}

// Latest returns the newest published release.
func Latest(ctx context.Context) (Release, error) {
	return latestFrom(ctx, releaseAPI)
}

func latestFrom(ctx context.Context, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github returned %s", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("decoding release: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("no published release found")
	}
	rel := Release{
		Tag:     payload.TagName,
		Version: strings.TrimPrefix(payload.TagName, "v"),
		PageURL: payload.HTMLURL,
	}
	want := AssetName()
	for _, a := range payload.Assets {
		if a.Name == want {
			rel.AssetURL = a.URL
			break
		}
	}
	return rel, nil
}

// AssetName is the release asset for the running platform. Must match
// what scripts/install.sh downloads and what the Makefile's build-all
// target produces.
func AssetName() string {
	return fmt.Sprintf("tracks-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// Newer reports whether latest is a strictly higher release than current.
//
// A current version we can't parse — "dev", or the `git describe` string
// a local `make build` stamps — counts as older than any real release:
// someone running an unidentifiable build should still be offered the
// update rather than told they're current. An unparseable *latest* is the
// other way round: we don't know what we'd be installing, so we don't
// offer it.
func Newer(current, latest string) bool {
	l, ok := parse(latest)
	if !ok {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return true
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// Drop any pre-release / build suffix (v0.5.0-3-gabc1234-dirty):
	// only the release triple in front is comparable.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// Apply downloads rel's binary for this platform and replaces the running
// executable with it, returning the path it replaced.
//
// The download lands in the target's own directory so the final swap is a
// same-filesystem rename — atomic, so an interrupted update can never
// leave a half-written binary on PATH. The new file is run once
// (`tracks version`) before the swap: a truncated download or an asset
// built for the wrong platform must not take the place of a working
// install.
func Apply(ctx context.Context, rel Release) (string, error) {
	if rel.AssetURL == "" {
		return "", fmt.Errorf("release %s has no %s binary — see %s", rel.Tag, AssetName(), ReleasesPage)
	}
	target, err := selfPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".tracks-update-*")
	if err != nil {
		return "", fmt.Errorf("cannot write to %s: %w — re-run scripts/install.sh instead", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := download(ctx, rel.AssetURL, tmp); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", err
	}
	if err := verify(ctx, tmpName); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", fmt.Errorf("replacing %s: %w", target, err)
	}
	return target, nil
}

func download(ctx context.Context, url string, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	n, err := io.Copy(dst, resp.Body)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	if n == 0 {
		return fmt.Errorf("downloading %s: empty response", url)
	}
	return nil
}

func verify(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, path, "version").CombinedOutput(); err != nil {
		return fmt.Errorf("downloaded binary does not run (%w): %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// selfPath resolves the running binary, following symlinks so an update
// replaces the real file rather than turning a symlink into a copy.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the running binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}
