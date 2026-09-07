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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	// userAgent identifies these requests to GitHub, which asks callers
	// for one and answers rate-limit questions by it.
	userAgent = "tracks-updater"

	// ChecksumsName is the release asset listing the SHA-256 of every
	// binary in the release (`sha256sum` output, written by the release
	// workflow). Apply refuses to install an asset it doesn't vouch for.
	ChecksumsName = "SHA256SUMS"

	// assetHost is the only host release assets are fetched from.
	assetHost = "github.com"
)

// Release is the subset of a GitHub release the update flow needs.
type Release struct {
	Tag      string // as published, e.g. "v0.5.1"
	Version  string // Tag without the leading "v"
	PageURL  string // human-readable release page
	AssetURL string // download URL for this OS/arch; "" when absent
	// ChecksumsURL is the download URL of ChecksumsName; "" when the
	// release doesn't publish one, which Apply treats as fatal.
	ChecksumsURL string
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
	req.Header.Set("User-Agent", userAgent)
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
		switch a.Name {
		case want:
			rel.AssetURL = a.URL
		case ChecksumsName:
			rel.ChecksumsURL = a.URL
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
// leave a half-written binary on PATH.
//
// Integrity comes first, because the steps after the download are not
// safe to reach with a file we can't account for: the new binary is *run
// once* (`tracks version`) to prove it works on this platform, and then
// put on the user's PATH. So the release's published SHA-256 is fetched
// before anything is downloaded, and the download is discarded unless it
// hashes to that value — the exec below never sees an unverified file.
// A release with no ChecksumsName asset is refused outright rather than
// installed on trust; the only integrity signal left would be TLS to
// whatever host the release JSON happened to name.
func Apply(ctx context.Context, rel Release) (string, error) {
	if rel.AssetURL == "" {
		return "", fmt.Errorf("release %s has no %s binary — see %s", rel.Tag, AssetName(), ReleasesPage)
	}
	if err := verifyAssetURL(rel.AssetURL); err != nil {
		return "", err
	}
	if rel.ChecksumsURL == "" {
		return "", fmt.Errorf("release %s publishes no %s, so its binary can't be verified — install it by hand from %s",
			rel.Tag, ChecksumsName, ReleasesPage)
	}
	if err := verifyAssetURL(rel.ChecksumsURL); err != nil {
		return "", err
	}
	wantSum, err := expectedDigest(ctx, rel.ChecksumsURL, AssetName())
	if err != nil {
		return "", err
	}
	target, err := selfPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(target)
	sweepLeftovers(dir)
	tmp, err := os.CreateTemp(dir, tmpPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("cannot write to %s: %w — re-run scripts/install.sh instead", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	gotSum, err := download(ctx, rel.AssetURL, tmp)
	if err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if !strings.EqualFold(gotSum, wantSum) {
		return "", fmt.Errorf("%s does not match the checksum %s publishes for it (want %s, got %s) — not installing",
			AssetName(), ChecksumsName, wantSum, gotSum)
	}
	if err := os.Chmod(tmpName, targetMode(target)); err != nil {
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

// tmpPrefix names the in-flight download. Dotted so it stays out of the
// way on PATH, and fixed so an update killed mid-download can be swept up
// by the next one.
const tmpPrefix = ".tracks-update-"

func sweepLeftovers(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, tmpPrefix+"*"))
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// targetMode keeps the permissions of the binary being replaced, so a
// deliberately private install doesn't silently widen to 0755.
func targetMode(target string) os.FileMode {
	fi, err := os.Stat(target)
	if err != nil {
		return 0o755
	}
	mode := fi.Mode().Perm()
	if mode&0o100 == 0 {
		return 0o755
	}
	return mode
}

// download copies url into dst and returns the hex SHA-256 of everything
// written, hashed as it streams so the caller can check the file without
// reading it back.
func download(ctx context.Context, url string, dst io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, sum), resp.Body)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	if n == 0 {
		return "", fmt.Errorf("downloading %s: empty response", url)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// expectedDigest fetches the release's checksums file and returns the
// SHA-256 it records for name. The format is `sha256sum` output — digest,
// whitespace, file name, the name optionally marked `*` for binary mode.
//
// A file that doesn't mention name is an error, not a pass: "no line for
// this asset" and "this asset is vouched for" must not look alike.
func expectedDigest(ctx context.Context, url, name string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", ChecksumsName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: %s", ChecksumsName, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", ChecksumsName, err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if len(fields[0]) != hex.EncodedLen(sha256.Size) {
			return "", fmt.Errorf("%s lists a malformed digest for %s: %q", ChecksumsName, name, fields[0])
		}
		return fields[0], nil
	}
	return "", fmt.Errorf("%s does not list %s — not installing an unverified binary", ChecksumsName, name)
}

// verifyAssetURL rejects a download URL that isn't an HTTPS URL on
// GitHub. Both URLs Apply uses come out of the release JSON, so this is
// what keeps a tampered or redirected response from choosing the host
// that supplies the checksums — the file every other check trusts.
//
// A var so tests can serve a release from a local httptest server.
var verifyAssetURL = func(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("unusable download URL %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing to download %s over %q — https required", raw, u.Scheme)
	}
	if h := u.Hostname(); h != assetHost && !strings.HasSuffix(h, "."+assetHost) {
		return fmt.Errorf("refusing to fetch a release asset from %q — expected %s", h, assetHost)
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
// replaces the real file rather than turning a symlink into a copy. It's a
// var so tests can point Apply at a throwaway binary instead of the test
// process's own.
var selfPath = func() (string, error) {
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
