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

// detail is the data the inline detail panel renders for the
// currently selected track. Populated by the dashboard's poll
// loop whenever the cursor lands on a row.
type detail struct {
	track   state.Track
	files   []string // "<repo>: <status>\t<path>"
	commits []string // "<repo>: <sha7> <subject>"
}

// gatherDetail walks the track's worktrees and pulls the changed
// files + commit log for each. Fast enough to run on every poll
// tick (~2s); the supervisor's poll picks up the same data into
// state.Changes for the table column, so we don't pay twice.
func gatherDetail(cfg config.Config, t state.Track) detail {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	d := detail{track: t}
	for _, tr := range t.Repos {
		repo, ok := cfg.RepoByName(tr.Name)
		if !ok {
			continue
		}
		c := git.NewWorktreeClient(tr.Path)
		base := "origin/" + repo.Base
		if files, err := c.ChangedFiles(ctx, base); err == nil {
			for _, f := range files {
				d.files = append(d.files, tr.Name+": "+f)
			}
		}
		if commits, err := c.CommitLog(ctx, base); err == nil {
			for _, line := range commits {
				d.commits = append(d.commits, tr.Name+": "+line)
			}
		}
	}
	return d
}

// renderDetail draws the inline detail panel below the table. Four
// horizontal sections: TASK, COMMITS, CHANGES, PR.
//
// width is the full dashboard width; we split it 4 ways with a
// small gutter between each column.
func (m *model) renderDetail(d detail, width int) string {
	if width < 60 {
		width = 60
	}
	gap := 2
	cols := 4
	colWidth := (width - 2 - gap*(cols-1)) / cols
	if colWidth < 14 {
		colWidth = 14
	}

	title := m.styles.panelTitle.Render("▍ Details") +
		"  " + m.styles.dim.Render(d.track.ID)
	if d.track.Slug != "" {
		title += "  " + m.styles.slug.Render(d.track.Slug)
	}

	taskCol := m.renderTaskSection(d.track, colWidth)
	commitsCol := m.renderCommitsSection(d.commits, colWidth)
	changesCol := m.renderChangesSection(d.track, d.files, colWidth)
	prCol := m.renderPRSection(d.track, colWidth)

	row := lipgloss.JoinHorizontal(lipgloss.Top,
		taskCol,
		strings.Repeat(" ", gap),
		commitsCol,
		strings.Repeat(" ", gap),
		changesCol,
		strings.Repeat(" ", gap),
		prCol,
	)
	body := lipgloss.JoinVertical(lipgloss.Left, title, "", row)
	return m.styles.panel.Width(width - 2).Render(body)
}

// renderTaskSection: task prompt + key metadata (status, branch,
// idle, last activity).
func (m *model) renderTaskSection(t state.Track, w int) string {
	lines := []string{
		m.styles.sectionHdr.Render("TASK"),
		m.styles.dim.Render("status ") + m.styles.status[t.Status].Render(string(t.Status)),
		m.styles.dim.Render("branch ") + m.styles.branch.Render(t.Branch),
	}
	if !t.UpdatedAt.IsZero() {
		lines = append(lines, m.styles.dim.Render("idle   ")+renderIdle(t))
	}
	lines = append(lines, "")
	if t.TaskPrompt != "" {
		for _, line := range wrapInfoText(t.TaskPrompt, w) {
			lines = append(lines, m.styles.dim.Render("│ ")+line)
		}
	}
	return strings.Join(lines, "\n")
}

// renderCommitsSection: short list of commits beyond base.
func (m *model) renderCommitsSection(commits []string, w int) string {
	lines := []string{
		m.styles.sectionHdr.Render(fmt.Sprintf("COMMITS (%d)", len(commits))),
	}
	if len(commits) == 0 {
		lines = append(lines, m.styles.dim.Render("  (none yet)"))
	} else {
		const max = 6
		shown := commits
		if len(shown) > max {
			shown = shown[:max]
		}
		for _, line := range shown {
			lines = append(lines, truncate(line, w))
		}
		if len(commits) > max {
			lines = append(lines, m.styles.dim.Render(fmt.Sprintf("  …%d more", len(commits)-max)))
		}
	}
	return strings.Join(lines, "\n")
}

// renderChangesSection: shortstat + first few changed files.
func (m *model) renderChangesSection(t state.Track, files []string, w int) string {
	lines := []string{
		m.styles.sectionHdr.Render(fmt.Sprintf("CHANGES (%d files)", t.Changes.Files)),
	}
	if !t.Changes.IsZero() {
		lines = append(lines,
			m.styles.insertions.Render(fmt.Sprintf("+%d", t.Changes.Insertions))+
				" "+m.styles.deletions.Render(fmt.Sprintf("-%d", t.Changes.Deletions)))
	}
	if len(files) == 0 {
		lines = append(lines, m.styles.dim.Render("  (no diff yet)"))
	} else {
		const max = 6
		shown := files
		if len(shown) > max {
			shown = shown[:max]
		}
		for _, line := range shown {
			lines = append(lines, truncate(line, w))
		}
		if len(files) > max {
			lines = append(lines, m.styles.dim.Render(fmt.Sprintf("  …%d more", len(files)-max)))
		}
	}
	return strings.Join(lines, "\n")
}

// renderPRSection: URL + state badge + comments + (optional) when
// the track has no PR yet, a small hint.
func (m *model) renderPRSection(t state.Track, w int) string {
	lines := []string{
		m.styles.sectionHdr.Render("PR"),
	}
	if t.PRURL == "" {
		lines = append(lines, m.styles.dim.Render("(no PR yet)"))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, m.styles.pr.Render(truncate(t.PRURL, w)))
	badge := prBadge(t)
	if badge != "" {
		lines = append(lines, m.prBadgeStyle(t).Render("● "+badge))
	}
	if t.PRComments > 0 {
		lines = append(lines, m.styles.count.Render(fmt.Sprintf("%d comments", t.PRComments)))
	}
	return strings.Join(lines, "\n")
}

// wrapInfoText soft-wraps s at width, preserving paragraph breaks.
// Used by the detail panel's task section; returns a slice for
// easy line-by-line rendering by the caller.
func wrapInfoText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			if line == "" {
				line = word
				continue
			}
			if len(line)+1+len(word) > width {
				out = append(out, line)
				line = word
				continue
			}
			line += " " + word
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
