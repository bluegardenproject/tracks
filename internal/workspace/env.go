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
// node_modules aren't searched, and a file already in worktree is left
// alone.
func CopyEnv(ctx context.Context, primary, worktree string) ([]string, error) {
	// --directory lists an ignored folder as one entry instead of
	// everything in it.
	out, _, err := git.ExecRunner{Dir: primary}.Run(ctx, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, err
	}
	var copied []string
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" || strings.HasSuffix(rel, "/") || !isEnvFile(filepath.Base(rel)) {
			continue
		}
		ok, err := copyNew(filepath.Join(primary, rel), filepath.Join(worktree, rel))
		if err != nil {
			return copied, err
		}
		if ok {
			copied = append(copied, rel)
		}
	}
	return copied, nil
}

// copyNew copies the regular file from to to, keeping its mode, unless
// to exists. It reports whether it copied.
func copyNew(from, to string) (bool, error) {
	info, err := os.Lstat(from)
	if err != nil || !info.Mode().IsRegular() {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return false, err
	}
	dst, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if errors.Is(err, os.ErrExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	src, err := os.Open(from)
	if err != nil {
		_ = dst.Close()
		return false, err
	}
	defer src.Close()
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return false, err
	}
	return true, dst.Close()
}
