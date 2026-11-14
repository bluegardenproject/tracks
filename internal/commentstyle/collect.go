package commentstyle

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Limits on what's collected: enough to see a style, few enough to read
// in a minute or two and to fit an engine's prompt.
const (
	maxPRs      = 60
	maxComments = 300
	maxBody     = 1500       // runes kept of a comment
	maxPrompt   = 120 * 1024 // bytes of comments an engine is given
)

// API asks GitHub's REST API, as the user is logged in to it.
type API interface {
	// Get returns the JSON at path, with query's parameters.
	Get(ctx context.Context, path string, query ...string) ([]byte, error)
}

type ghUser struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

func (u ghUser) bot() bool { return u.Type == "Bot" || strings.HasSuffix(u.Login, "[bot]") }

type ghComment struct {
	ID        int64     `json:"id"`
	InReplyTo int64     `json:"in_reply_to_id"`
	User      ghUser    `json:"user"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// skipped counts the user's comments left out, and why.
type skipped struct{ tracks, bots int }

// keep picks login's comments since since from one PR's list. Those
// carrying Marker were written by Tracks; replies in a thread a bot
// started are left out too, as Tracks wrote those before the marker.
func keep(list []ghComment, login string, since time.Time) (kept []ghComment, s skipped) {
	byID := map[int64]ghComment{}
	for _, c := range list {
		byID[c.ID] = c
	}
	for _, c := range list {
		if c.User.Login != login || c.CreatedAt.Before(since) || strings.TrimSpace(c.Body) == "" {
			continue
		}
		switch root, ok := byID[c.InReplyTo]; {
		case strings.Contains(c.Body, Marker):
			s.tracks++
		case c.InReplyTo != 0 && ok && root.User.bot():
			s.bots++
		default:
			kept = append(kept, c)
		}
	}
	return kept, s
}

// collect gathers login's PR comments since since, newest first, telling
// log each request and answer.
func collect(ctx context.Context, api API, login string, since time.Time, log func(Line)) ([]Comment, error) {
	q := "involves:" + login + " is:pr updated:>=" + since.Format("2006-01-02")
	log(Line{Request, "GET search/issues  q=" + q})
	out, err := api.Get(ctx, "search/issues", "q="+q, "per_page="+strconv.Itoa(maxPRs), "sort=updated")
	if err != nil {
		return nil, err
	}
	var found struct {
		Total int `json:"total_count"`
		Items []struct {
			Number int    `json:"number"`
			Repo   string `json:"repository_url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("reading the search: %w", err)
	}
	log(Line{Answer, fmt.Sprintf("%d PRs, reading the latest %d", found.Total, len(found.Items))})

	var comments []Comment
	var left skipped
	for _, it := range found.Items {
		parts := strings.Split(it.Repo, "/") // https://api.github.com/repos/<owner>/<name>
		if len(parts) < 2 {
			continue
		}
		repo := parts[len(parts)-2] + "/" + parts[len(parts)-1]
		pr := repo + "#" + strconv.Itoa(it.Number)
		n := 0
		for _, kind := range []struct{ name, path string }{
			{"review", "repos/" + repo + "/pulls/" + strconv.Itoa(it.Number) + "/comments"},
			{"pr", "repos/" + repo + "/issues/" + strconv.Itoa(it.Number) + "/comments"},
		} {
			log(Line{Request, "GET " + kind.path})
			out, err := api.Get(ctx, kind.path, "per_page=100")
			if err != nil {
				log(Line{Fail, err.Error()})
				continue
			}
			var list []ghComment
			if err := json.Unmarshal(out, &list); err != nil {
				log(Line{Fail, "reading " + kind.path + ": " + err.Error()})
				continue
			}
			kept, s := keep(list, login, since)
			left.tracks, left.bots = left.tracks+s.tracks, left.bots+s.bots
			for _, c := range kept {
				comments = append(comments, Comment{PR: pr, Kind: kind.name, Body: clip(c.Body), At: c.CreatedAt})
			}
			n += len(kept)
		}
		log(Line{Answer, fmt.Sprintf("%s: %d of yours", pr, n)})
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	slices.SortFunc(comments, func(a, b Comment) int { return b.At.Compare(a.At) })
	size := 0
	for i, c := range comments {
		if size += len(c.Body); i == maxComments || size > maxPrompt {
			comments = comments[:i]
			break
		}
	}
	log(Line{Note, fmt.Sprintf("kept %d comments; left out %d written by Tracks and %d in bot threads", len(comments), left.tracks, left.bots)})
	return comments, nil
}

// clip cuts a comment to maxBody runes.
func clip(body string) string {
	r := []rune(strings.TrimSpace(body))
	if len(r) <= maxBody {
		return string(r)
	}
	return string(r[:maxBody]) + " …"
}

// login asks GitHub who the user is.
func login(ctx context.Context, api API, log func(Line)) (string, error) {
	log(Line{Request, "GET user"})
	out, err := api.Get(ctx, "user")
	if err != nil {
		return "", err
	}
	var u ghUser
	if err := json.Unmarshal(out, &u); err != nil || u.Login == "" {
		return "", fmt.Errorf("reading who you are: %w", cmp.Or(err, fmt.Errorf("no login")))
	}
	log(Line{Answer, "you're " + u.Login})
	return u.Login, nil
}
