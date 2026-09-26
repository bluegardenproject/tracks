package repos

import (
	"context"
	"net/url"
	"strings"
)

// remote is the web address of dir's origin, "" without one.
func (s Service) remote(ctx context.Context, dir string) string {
	raw, err := s.Git.Remote(ctx, dir)
	if err != nil {
		return ""
	}
	return webURL(raw)
}

// webURL turns a git remote into the address of its web page, without
// credentials. Remotes that aren't on a host, such as local paths, give
// "".
func webURL(remote string) string {
	remote = strings.TrimSpace(remote)
	scheme, host, path := "https", "", ""
	if !strings.Contains(remote, "://") {
		// scp-like: [user@]host:path
		before, after, ok := strings.Cut(remote, ":")
		if !ok || strings.Contains(before, "/") {
			return ""
		}
		_, host, _ = strings.Cut(before, "@")
		if host == "" {
			host = before
		}
		path = after
	} else {
		u, err := url.Parse(remote)
		if err != nil {
			return ""
		}
		switch u.Scheme {
		case "http", "https":
			scheme, host = u.Scheme, u.Host
		case "ssh", "git", "git+ssh":
			host = u.Hostname()
		default:
			return ""
		}
		path = u.Path
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return ""
	}
	return scheme + "://" + host + "/" + path
}
