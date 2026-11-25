package workspace

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/git"
)

// isEnvFile reports whether name is a .env file: .env or .env.<any>.
func isEnvFile(name string) bool { return name == ".env" || strings.HasPrefix(name, ".env.") }

// CopyEnv copies primary's ignored .env files into worktree, at the
// same relative paths, and returns those paths. Ignored folders such as
// node_modules aren't searched, symlinks in either tree are refused, and
// a file already in worktree is left alone.
func CopyEnv(ctx context.Context, primary, worktree string) ([]string, error) {
	// --directory lists an ignored folder as one entry instead of
	// everything in it.
	out, _, err := git.ExecRunner{Dir: primary}.Run(ctx, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, err
	}
	srcRoot, err := os.OpenRoot(primary)
	if err != nil {
		return nil, err
	}
	defer srcRoot.Close()
	dstRoot, err := os.OpenRoot(worktree)
	if err != nil {
		return nil, err
	}
	defer dstRoot.Close()

	var copied []string
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" || strings.HasSuffix(rel, "/") || !isEnvFile(filepath.Base(rel)) {
			continue
		}
		clean := filepath.Clean(rel)
		if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			continue
		}
		ok, err := copyNew(srcRoot, dstRoot, clean)
		if err != nil {
			return copied, err
		}
		if ok {
			copied = append(copied, rel)
		}
	}
	return copied, nil
}

// copyNew copies the regular file rel from srcRoot into dstRoot, keeping
// its mode, unless it exists or any intermediate folder is a symlink.
// It reports whether it copied.
func copyNew(srcRoot, dstRoot *os.Root, rel string) (bool, error) {
	if sym, err := hasSymlinkParent(srcRoot, rel); err != nil || sym {
		return false, err
	}
	info, err := srcRoot.Lstat(rel)
	if err != nil || !info.Mode().IsRegular() {
		return false, nil
	}
	if sym, err := hasSymlinkParent(dstRoot, rel); err != nil {
		return false, err
	} else if sym {
		return false, nil
	}
	if dir := filepath.Dir(rel); dir != "." {
		if err := dstRoot.MkdirAll(dir, 0o755); err != nil {
			return false, err
		}
	}
	dst, err := dstRoot.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if errors.Is(err, os.ErrExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	src, err := srcRoot.Open(rel)
	if err != nil {
		_ = dst.Close()
		_ = dstRoot.Remove(rel)
		return false, err
	}
	defer src.Close()
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = dstRoot.Remove(rel)
		return false, err
	}
	return true, dst.Close()
}

// hasSymlinkParent reports whether any existing ancestor directory of
// rel inside root is a symbolic link.
func hasSymlinkParent(root *os.Root, rel string) (bool, error) {
	dir := filepath.Dir(rel)
	if dir == "." {
		return false, nil
	}
	var cur string
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		if cur == "" {
			cur = part
		} else {
			cur = filepath.Join(cur, part)
		}
		info, err := root.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}
