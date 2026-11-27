// Package plugins installs, updates and removes plugins: each pinned
// to a commit in a folder of its own, or linked from a local folder
// while it's being written, and kept with what the user approved.
package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/plugin"
	"github.com/bluegardenproject/tracks/internal/store"
)

// Store is where plugins are kept.
type Store interface {
	Plugins(ctx context.Context) ([]store.Plugin, error)
	Plugin(ctx context.Context, name string) (store.Plugin, error)
	AddPlugin(ctx context.Context, p store.Plugin) (store.Plugin, error)
	UpdatePlugin(ctx context.Context, p store.Plugin) (store.Plugin, error)
	SetPluginEnabled(ctx context.Context, name string, enabled bool) error
	DeletePlugin(ctx context.Context, name string) error
}

// Service applies the rules for plugins.
type Service struct {
	Store Store
	// Dir holds the installed plugins: Dir/<name>/<commit>, or
	// Dir/<name>/local, a link to a local plugin's folder.
	Dir string
}

// ErrDeclined is returned when the user doesn't approve.
var ErrDeclined = errors.New("not approved; nothing changed")

// localDir is a local plugin's link in its folder.
const localDir = "local"

// Entry is an installed plugin with what it was approved for.
type Entry struct {
	store.Plugin
	Approval plugin.Approval
}

// List returns the installed plugins by name.
func (s Service) List(ctx context.Context) ([]Entry, error) {
	all, err := s.Store.Plugins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, len(all))
	for i, p := range all {
		out[i] = Entry{Plugin: p}
		if err := json.Unmarshal([]byte(p.Approved), &out[i].Approval); err != nil {
			return nil, fmt.Errorf("%s's approval: %w", p.Name, err)
		}
	}
	return out, nil
}

// Proposal is a plugin about to be installed, for the user to approve.
type Proposal struct {
	Manifest plugin.Manifest
	Approval plugin.Approval
	Warnings []string
	Source   Source
	Commit   string // "" for a local plugin
}

// Add installs the plugin src names once approve says yes. A local
// folder's path must be absolute.
func (s Service) Add(ctx context.Context, src Source, approve func(Proposal) (bool, error)) (store.Plugin, error) {
	if src.Local != "" {
		return s.addLocal(ctx, src, approve)
	}
	staging, err := s.staging()
	if err != nil {
		return store.Plugin{}, err
	}
	defer os.RemoveAll(staging)
	commit, err := fetch(ctx, src, staging)
	if err != nil {
		return store.Plugin{}, err
	}
	m, warnings, err := plugin.Load(staging)
	if err != nil {
		return store.Plugin{}, err
	}
	if err := s.free(ctx, m.Name); err != nil {
		return store.Plugin{}, err
	}
	approval := plugin.ApprovalOf(m)
	if ok, err := approve(Proposal{m, approval, warnings, src, commit}); err != nil || !ok {
		return store.Plugin{}, declined(err)
	}
	dir, err := s.place(staging, m.Name, commit)
	if err != nil {
		return store.Plugin{}, err
	}
	p := store.Plugin{Name: m.Name, Source: src.URL, Ref: src.Ref, Pinned: commit, Dir: dir, Enabled: true}
	return s.record(ctx, p, approval)
}

func (s Service) addLocal(ctx context.Context, src Source, approve func(Proposal) (bool, error)) (store.Plugin, error) {
	if !filepath.IsAbs(src.Local) {
		return store.Plugin{}, fmt.Errorf("%s isn't an absolute path", src.Local)
	}
	folder, err := filepath.EvalSymlinks(src.Local)
	if err != nil {
		return store.Plugin{}, fmt.Errorf("no folder at %s", src.Local)
	}
	m, warnings, err := plugin.Load(folder)
	if err != nil {
		return store.Plugin{}, err
	}
	if err := s.free(ctx, m.Name); err != nil {
		return store.Plugin{}, err
	}
	approval := plugin.ApprovalOf(m)
	src.Local = folder
	if ok, err := approve(Proposal{m, approval, warnings, src, ""}); err != nil || !ok {
		return store.Plugin{}, declined(err)
	}
	home, err := s.home(m.Name)
	if err != nil {
		return store.Plugin{}, err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return store.Plugin{}, err
	}
	link := filepath.Join(home, localDir)
	if err := os.RemoveAll(link); err != nil {
		return store.Plugin{}, err
	}
	if err := os.Symlink(folder, link); err != nil {
		return store.Plugin{}, err
	}
	p := store.Plugin{Name: m.Name, Source: folder, Local: true, Dir: link, Enabled: true}
	return s.record(ctx, p, approval)
}

// record stores a new plugin, and removes what was placed for it
// (its commit's folder, or its link) when that fails.
func (s Service) record(ctx context.Context, p store.Plugin, a plugin.Approval) (store.Plugin, error) {
	approved, err := json.Marshal(a)
	if err != nil {
		return store.Plugin{}, err
	}
	p.Approved = string(approved)
	added, err := s.Store.AddPlugin(ctx, p)
	if err != nil {
		_ = os.RemoveAll(p.Dir)
		if errors.Is(err, store.ErrNameTaken) {
			return store.Plugin{}, fmt.Errorf("%s is installed already; run tracks plugin update %s", p.Name, p.Name)
		}
		return store.Plugin{}, err
	}
	return added, nil
}

// free fails when a plugin called name is installed.
func (s Service) free(ctx context.Context, name string) error {
	_, err := s.Store.Plugin(ctx, name)
	switch {
	case err == nil:
		return fmt.Errorf("%s is installed already; run tracks plugin update %s", name, name)
	case errors.Is(err, store.ErrNotFound):
		return nil
	}
	return err
}

// Remove uninstalls the plugin called name and deletes its folder; a
// local plugin's own folder stays.
func (s Service) Remove(ctx context.Context, name string) error {
	if _, err := s.find(ctx, name); err != nil {
		return err
	}
	home, err := s.home(name)
	if err != nil {
		return err
	}
	if err := s.Store.DeletePlugin(ctx, name); err != nil {
		return err
	}
	return os.RemoveAll(home)
}

// SetEnabled enables or disables the plugin called name.
func (s Service) SetEnabled(ctx context.Context, name string, enabled bool) error {
	if _, err := s.find(ctx, name); err != nil {
		return err
	}
	return s.Store.SetPluginEnabled(ctx, name, enabled)
}

// find returns the plugin called name, with an error that says so when
// there's none.
func (s Service) find(ctx context.Context, name string) (store.Plugin, error) {
	p, err := s.Store.Plugin(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return store.Plugin{}, fmt.Errorf("no plugin called %s; tracks plugin list shows them", name)
	}
	return p, err
}

// home is the folder of the plugin called name.
func (s Service) home(name string) (string, error) {
	if !plugin.ValidName(name) {
		return "", fmt.Errorf("%q isn't a plugin's name", name)
	}
	return filepath.Join(s.Dir, name), nil
}

// staging makes an empty folder to fetch a plugin into, beside the
// installed ones so it moves into place with a rename.
func (s Service) staging() (string, error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(s.Dir, ".staging-")
}

// place moves a fetched plugin to its folder for commit.
func (s Service) place(staging, name, commit string) (string, error) {
	home, err := s.home(name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", err
	}
	dir := filepath.Join(home, commit)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	return dir, os.Rename(staging, dir)
}

// fetch clones src into dir and checks out the commit its ref names.
func fetch(ctx context.Context, src Source, dir string) (string, error) {
	if err := clone(ctx, src.URL, dir); err != nil {
		return "", err
	}
	commit, err := resolve(ctx, dir, src.Ref)
	if err != nil {
		return "", err
	}
	return commit, checkout(ctx, dir, commit)
}

func declined(err error) error {
	if err != nil {
		return err
	}
	return ErrDeclined
}
