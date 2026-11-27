package plugins

import (
	"errors"
	"strings"
)

// Source is where a plugin comes from: a git repository at a ref, or a
// local folder.
type Source struct {
	URL   string // a git URL; "" for a local folder
	Ref   string // a branch, tag or commit; "" for the remote's default
	Local string // a local folder's path, as given
}

// ParseSource reads `tracks plugin add`'s argument: a path (starting
// with /, ./, ../ or ~) is a local folder, anything else a git URL with
// an optional @ref.
func ParseSource(arg string) (Source, error) {
	arg = strings.TrimSpace(arg)
	switch {
	case arg == "":
		return Source{}, errors.New("give a git URL or a local folder")
	case arg == "." || arg == ".." || arg == "~" ||
		strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") || strings.HasPrefix(arg, "~/"):
		return Source{Local: arg}, nil
	case strings.HasPrefix(arg, "-"):
		return Source{}, errors.New("a git URL can't start with -")
	}
	url, ref := splitRef(arg)
	if url == "" {
		return Source{}, errors.New("give a git URL before @")
	}
	if ref != "" && (strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\n:^~?*[\\") || strings.Contains(ref, "..")) {
		return Source{}, errors.New("@" + ref + " isn't a branch, tag or commit")
	}
	return Source{URL: url, Ref: ref}, nil
}

// splitRef cuts the @ref off a git URL. The @ of a user (git@host:repo,
// https://user@host/repo) comes before the repo's path, a ref's after.
func splitRef(arg string) (url, ref string) {
	path := 0
	if scheme := strings.Index(arg, "://"); scheme >= 0 {
		if slash := strings.Index(arg[scheme+3:], "/"); slash >= 0 {
			path = scheme + 3 + slash
		} else {
			path = len(arg)
		}
	} else if colon := strings.Index(arg, ":"); colon >= 0 && !strings.Contains(arg[:colon], "/") {
		path = colon
	}
	at := strings.LastIndex(arg[path:], "@")
	if at < 0 {
		return arg, ""
	}
	return arg[:path+at], arg[path+at+1:]
}
