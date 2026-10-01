package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/update"
)

func TestVersion(t *testing.T) {
	var out strings.Builder
	c := newVersionCmd("2.0.0", "2026-10-01T12:00:00Z")
	c.SetOut(&out)
	if err := c.Execute(); err != nil || out.String() != "tracks 2.0.0 (built 2026-10-01T12:00:00Z)\n" {
		t.Errorf("version printed %q, %v", out.String(), err)
	}
}

func TestUpdate(t *testing.T) {
	rel := update.Release{Tag: "v2.1.0", Version: "2.1.0", PageURL: "https://example.com/v2.1.0"}
	var applied []string
	r := releases{
		latest: func(context.Context) (update.Release, error) { return rel, nil },
		apply: func(_ context.Context, got update.Release) (string, error) {
			applied = append(applied, got.Version)
			return "/usr/local/bin/tracks", nil
		},
	}
	run := func(version string, checkOnly bool) string {
		t.Helper()
		var out strings.Builder
		if err := runUpdate(context.Background(), &out, r, version, checkOnly); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}

	if out := run("2.1.0", false); !strings.Contains(out, "2.1.0 is the latest release") || len(applied) != 0 {
		t.Errorf("the latest version: %q, applied %v", out, applied)
	}
	if out := run("2.0.0", true); !strings.Contains(out, "2.0.0 → 2.1.0") || !strings.Contains(out, rel.PageURL) || len(applied) != 0 {
		t.Errorf("--check: %q, applied %v", out, applied)
	}
	out := run("2.0.0", false)
	if len(applied) != 1 || !strings.Contains(out, "installed tracks 2.1.0 at /usr/local/bin/tracks") || !strings.Contains(out, restartHint) {
		t.Errorf("an update: %q, applied %v", out, applied)
	}

	r.latest = func(context.Context) (update.Release, error) { return update.Release{}, errors.New("offline") }
	if err := runUpdate(context.Background(), &strings.Builder{}, r, "2.0.0", false); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Errorf("a failed check returns %v", err)
	}
}
