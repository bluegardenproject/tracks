package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/charmbracelet/lipgloss"
)

// info is the live data shown by the dashboard's per-track info
// popup. Populated by openInfo when the user hits `i`; refreshed
// in-place when they hit `r` inside the modal.
type info struct {
	track   state.Track
	files   []string // "<status>\t<path>"
	commits []string // "<sha7> <subject>"
	err     error
}

// openInfo collects everything the modal needs for the supplied
// track. Falls back to partial data on git failure — the modal
// surfaces whatever it could grab.
func openInfo(cfg config.Config, t state.Track) *info {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	i := &info{track: t}
	for _, tr := range t.Repos {
		repo, ok := cfg.RepoByName(tr.Name)
		if !ok {
			continue
		}
		c := git.NewWorktreeClient(tr.Path)
		base := "origin/" + repo.Base
		if files, err := c.ChangedFiles(ctx, base); err == nil {
			for _, f := range files {
				i.files = append(i.files, tr.Name+": "+f)
			}
		} else {
			i.err = err
		}
		if commits, err := c.CommitLog(ctx, base); err == nil {
			for _, line := range commits {
				i.commits = append(i.commits, tr.Name+": "+line)
			}
		}
	}
	return i
}

// renderInfo draws the modal as a single block string. Centered
// horizontally on the screen so it stands out from the table.
func (m *model) renderInfo(i *info) string {
	width := m.width - 8
	if width < 50 {
		width = 50
	}
	t := i.track

	var b strings.Builder
	b.WriteString(m.styles.header.Render(fmt.Sprintf("Track  %s", t.ID)))
	b.WriteString("\n\n")

	field := func(label, value string) {
		if value == "" {
			return
		}
		b.WriteString(m.styles.dim.Render(label + ": "))
		b.WriteString(value)
		b.WriteString("\n")
	}
	field("branch", t.Branch)
	field("slug", t.Slug)
	field("repos", joinRepos(t.Repos))
	b.WriteString(m.styles.dim.Render("status: "))
	b.WriteString(m.styles.status[t.Status].Render(string(t.Status)))
	b.WriteString("\n")
	field("changes", renderChanges(t.Changes))
	field("idle", renderIdle(t))
	if t.PRURL != "" {
		field("pr", t.PRURL)
	}
	if !t.CreatedAt.IsZero() {
		field("created", t.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	}
	if t.ExitedAt != nil {
		field("exited", t.ExitedAt.Local().Format("2006-01-02 15:04:05"))
	}

	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("task prompt:\n"))
	b.WriteString(wrapInfoText(t.TaskPrompt, width-4))
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render(fmt.Sprintf("commits (%d):", len(i.commits))))
	b.WriteString("\n")
	if len(i.commits) == 0 {
		b.WriteString(m.styles.dim.Render("  (none yet)\n"))
	} else {
		for _, c := range i.commits {
			b.WriteString("  ")
			b.WriteString(c)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render(fmt.Sprintf("changed files (%d):", len(i.files))))
	b.WriteString("\n")
	if len(i.files) == 0 {
		b.WriteString(m.styles.dim.Render("  (none yet)\n"))
	} else {
		for _, f := range i.files {
			b.WriteString("  ")
			b.WriteString(f)
			b.WriteString("\n")
		}
	}

	if i.err != nil {
		b.WriteString("\n")
		b.WriteString(m.styles.dim.Render("(some details unavailable: " + i.err.Error() + ")\n"))
	}

	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("enter attach window   r refresh   esc back"))

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2).
		Width(width)
	return border.Render(b.String())
}

// wrapInfoText soft-wraps s at width, preserving paragraph
// breaks. Used only by the modal's task-prompt area; no need for
// runewidth precision since prompts are ascii in practice.
func wrapInfoText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out.WriteString("\n")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			if line == "" {
				line = word
				continue
			}
			if len(line)+1+len(word) > width {
				out.WriteString(line)
				out.WriteString("\n")
				line = word
				continue
			}
			line += " " + word
		}
		if line != "" {
			out.WriteString(line)
			out.WriteString("\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}
