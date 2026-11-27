package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/bluegardenproject/tracks/internal/plugin"
	"github.com/bluegardenproject/tracks/internal/store"
)

// Change is what an update brings, for the user to see and, when Ask,
// approve again.
type Change struct {
	Name     string
	Local    bool
	From, To string // the commits; "" for a local plugin
	// Current is true when there's nothing to update.
	Current bool
	// ManifestDiff is plugin.yaml's diff; Code the other files that
	// changed, briefs aside. Without a diff to show (the old commit is
	// gone), Unknown is set.
	ManifestDiff string
	Code         []string
	Unknown      bool
	Widens       []string
	Warnings     []string
	Approval     plugin.Approval
	// Ask is true when the code or the permissions changed: then the
	// update waits for a yes.
	Ask bool
}

// Update brings the plugin called name to its ref's newest commit, or
// for a local plugin, rereads its manifest. confirm sees every change
// before it's made, and must say yes to it.
func (s Service) Update(ctx context.Context, name string, confirm func(Change) (bool, error)) (Change, error) {
	p, err := s.find(ctx, name)
	if err != nil {
		return Change{}, err
	}
	var prev plugin.Approval
	if err := json.Unmarshal([]byte(p.Approved), &prev); err != nil {
		return Change{}, fmt.Errorf("%s's approval: %w", name, err)
	}
	if p.Local {
		return s.updateLocal(ctx, p, prev, confirm)
	}
	staging, err := s.staging()
	if err != nil {
		return Change{}, err
	}
	defer os.RemoveAll(staging)
	commit, err := fetch(ctx, Source{URL: p.Source, Ref: p.Ref}, staging)
	if err != nil {
		return Change{}, err
	}
	c := Change{Name: name, From: p.Pinned, To: commit}
	if commit == p.Pinned {
		c.Current = true
		return c, nil
	}
	m, warnings, err := plugin.Load(staging)
	if err != nil {
		return Change{}, err
	}
	if m.Name != name {
		return Change{}, fmt.Errorf("the plugin is now called %s; remove %s and add it again", m.Name, name)
	}
	c.Warnings, c.Approval = warnings, plugin.ApprovalOf(m)
	diff, changed, err := compare(ctx, staging, p.Dir, p.Pinned, commit)
	if err != nil {
		c.Unknown = true
	}
	c.ManifestDiff, c.Code = diff, code(changed, m, s.manifestAt(p.Dir))
	c.Widens = plugin.Widens(prev, c.Approval)
	c.Ask = c.Unknown || len(c.Code) > 0 || !prev.Equal(c.Approval)
	if ok, err := confirm(c); err != nil || !ok {
		return c, declined(err)
	}
	dir, err := s.place(staging, name, commit)
	if err != nil {
		return c, err
	}
	if err := s.save(ctx, p, commit, dir, c.Approval); err != nil {
		return c, err
	}
	if p.Dir != dir && filepath.Dir(p.Dir) == filepath.Dir(dir) {
		_ = os.RemoveAll(p.Dir)
	}
	return c, nil
}

// updateLocal rereads a local plugin's manifest. Its code isn't
// pinned, so only a change to what it's approved for asks again.
func (s Service) updateLocal(ctx context.Context, p store.Plugin, prev plugin.Approval, confirm func(Change) (bool, error)) (Change, error) {
	m, warnings, err := plugin.Load(p.Dir)
	if err != nil {
		return Change{}, err
	}
	if m.Name != p.Name {
		return Change{}, fmt.Errorf("the plugin is now called %s; remove %s and add it again", m.Name, p.Name)
	}
	c := Change{Name: p.Name, Local: true, Warnings: warnings, Approval: plugin.ApprovalOf(m)}
	if prev.Equal(c.Approval) {
		c.Current = true
		return c, nil
	}
	c.Widens = plugin.Widens(prev, c.Approval)
	c.Ask = true
	if ok, err := confirm(c); err != nil || !ok {
		return c, declined(err)
	}
	return c, s.save(ctx, p, "", p.Dir, c.Approval)
}

func (s Service) save(ctx context.Context, p store.Plugin, commit, dir string, a plugin.Approval) error {
	approved, err := json.Marshal(a)
	if err != nil {
		return err
	}
	p.Pinned, p.Dir, p.Approved = commit, dir, string(approved)
	_, err = s.Store.UpdatePlugin(ctx, p)
	return err
}

// manifestAt is the manifest installed in dir; nil when it can't be
// read.
func (s Service) manifestAt(dir string) *plugin.Manifest {
	m, _, err := plugin.Load(dir)
	if err != nil {
		return nil
	}
	return &m
}

// code is the changed files that count as code: all but plugin.yaml
// and the briefs, which are prompts, of either version.
func code(changed []string, next plugin.Manifest, prev *plugin.Manifest) []string {
	text := []string{plugin.File}
	for _, m := range []*plugin.Manifest{&next, prev} {
		if m == nil {
			continue
		}
		for _, c := range m.Components {
			text = append(text, path.Clean(c.Brief))
		}
	}
	var out []string
	for _, f := range changed {
		if !slices.Contains(text, f) {
			out = append(out, f)
		}
	}
	return out
}
