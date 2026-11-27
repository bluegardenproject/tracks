package plugin

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxManifest is the largest plugin.yaml read.
const maxManifest = 1 << 20

// Load reads and checks the manifest in dir, and checks that every path
// it names is there and, symlinks followed, inside dir.
func Load(dir string) (Manifest, []string, error) {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("no plugin folder at %s", dir)
	}
	manifest := filepath.Join(root, File)
	info, err := os.Lstat(manifest)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Manifest{}, nil, fmt.Errorf("no %s in %s: a plugin's folder holds one", File, dir)
	case err != nil:
		return Manifest{}, nil, err
	case !info.Mode().IsRegular():
		return Manifest{}, nil, fmt.Errorf("%s in %s isn't a plain file", File, dir)
	case info.Size() > maxManifest:
		return Manifest{}, nil, fmt.Errorf("%s in %s is larger than %d bytes", File, dir, maxManifest)
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		return Manifest{}, nil, err
	}
	m, warnings, err := Parse(data)
	if err != nil {
		return Manifest{}, warnings, err
	}
	if problems := checkFiles(root, m); len(problems) > 0 {
		return Manifest{}, warnings, problems
	}
	return m, warnings, nil
}

// checkFiles finds the manifest's paths that are missing or lead out of
// root.
func checkFiles(root string, m Manifest) Problems {
	var ps Problems
	check := func(field, p string, wantFile bool) {
		resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			ps = append(ps, Problem{field, fmt.Sprintf("%s isn't in the plugin's folder", p)})
			return
		}
		if rel, err := filepath.Rel(root, resolved); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			ps = append(ps, Problem{field, fmt.Sprintf("%s leads out of the plugin's folder", p)})
			return
		}
		if info, err := os.Stat(resolved); err == nil && wantFile && !info.Mode().IsRegular() {
			ps = append(ps, Problem{field, fmt.Sprintf("%s should be a file", p)})
		}
	}
	for i, c := range m.Components {
		field := fmt.Sprintf("components[%d]", i)
		check(field+".brief", c.Brief, true)
		for j, p := range c.Context {
			check(fmt.Sprintf("%s.context[%d]", field, j), p, false)
		}
		for j, p := range c.MCP {
			check(fmt.Sprintf("%s.mcp[%d]", field, j), p, false)
		}
	}
	return ps
}
