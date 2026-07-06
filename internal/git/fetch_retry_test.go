package git

import (
	"context"
	"errors"
	"testing"
)

// scriptedRunner returns a canned (stdout, stderr, err) per call from
// results, in order, and records how many times Run was invoked.
type scriptedRunner struct {
	results []error
	calls   int
}

func (r *scriptedRunner) Run(ctx context.Context, args ...string) (string, string, error) {
	i := r.calls
	r.calls++
	if i < len(r.results) {
		return "", "", r.results[i]
	}
	return "", "", nil
}

func TestIsTransientNetwork(t *testing.T) {
	transient := []string{
		"fatal: unable to access 'https://github.com/x/y.git/': Could not resolve host: github.com",
		"ssh: connect to host github.com port 22: Operation timed out",
		"fatal: Could not read from remote repository.",
		"error: RPC failed; curl 56 Recv failure: Connection reset by peer",
		"fetch-pack: unexpected disconnect while reading sideband packet\nfatal: early EOF",
	}
	for _, m := range transient {
		if !isTransientNetwork(errors.New(m)) {
			t.Errorf("expected transient-network for %q", m)
		}
	}
	notTransient := []string{
		"fatal: couldn't find remote ref refs/heads/nope",
		"fatal: Authentication failed for 'https://github.com/x/y.git/'",
		"",
	}
	for _, m := range notTransient {
		if isTransientNetwork(errors.New(m)) {
			t.Errorf("did not expect transient-network for %q", m)
		}
	}
	if isTransientNetwork(nil) {
		t.Error("nil error must not be transient")
	}
}

func TestFetchWithRetrySucceedsAfterTransient(t *testing.T) {
	r := &scriptedRunner{results: []error{
		errors.New("fatal: unable to access 'https://...': Could not resolve host: github.com"),
		nil, // second attempt succeeds
	}}
	c := &PrimaryRepoClient{Path: "/x", Runner: r}
	if err := c.FetchWithRetry(context.Background(), "origin", "main"); err != nil {
		t.Fatalf("expected success after one transient failure, got %v", err)
	}
	if r.calls != 2 {
		t.Errorf("expected 2 attempts, got %d", r.calls)
	}
}

func TestFetchWithRetryStopsOnNonTransient(t *testing.T) {
	r := &scriptedRunner{results: []error{
		errors.New("fatal: couldn't find remote ref refs/heads/nope"),
	}}
	c := &PrimaryRepoClient{Path: "/x", Runner: r}
	if err := c.FetchWithRetry(context.Background(), "origin", "nope"); err == nil {
		t.Fatal("expected a non-transient error to be returned")
	}
	if r.calls != 1 {
		t.Errorf("expected exactly 1 attempt (no retry on a real error), got %d", r.calls)
	}
}
