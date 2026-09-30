package addtrack

import (
	"slices"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// draftDiscardText replaces discardText once a draft keeps the track.
const draftDiscardText = "Your edits are lost. The draft stays in Station."

// Fill fills the form with req, a draft's request, to start it again.
// Creating from it replaces the draft. Repos no longer on the
// Repositories tab are left out, and the notice says which.
func (m Model) Fill(req tracks.Request) Model {
	for k, tk := range trackKinds {
		if tk == req.Kind {
			m = m.setKind(k)
		}
	}
	m.name.SetValue(req.Name)
	m.setPrompt(req.Prompt)
	m.terminal = req.Terminal
	m.picked = map[string]bool{}
	var gone []string
	for _, r := range req.Repos {
		if !slices.Contains(m.repos, r) {
			gone = append(gone, r)
			continue
		}
		m.picked[r] = true
		if m.kind == Review {
			m.repo = slices.Index(m.repos, r)
		}
	}
	m.target.SetValue(req.ReviewRef)
	m.document.SetValue(req.Document)
	if m.kind == Review || m.kind == Doc {
		m.candor = req.Candor
	}
	if m.kind == Doc {
		m.sections = []bool{req.Opinion, req.ClaimCheck}
	}
	if req.Engine != "" {
		on := m.runsOn[req.Kind]
		m.engine, m.model = req.Engine, req.Model
		m.chosen = on.Engine != req.Engine || on.Model != req.Model
	}
	m.draft, m.drafted = req.Draft, req.Draft != ""
	if len(gone) > 0 {
		m.notice = strings.Join(gone, ", ") + " isn't on the Repositories tab any more."
	}
	return m
}

// Tell shows text in the form's hint row, until the next key.
func (m Model) Tell(text string) Model {
	m.notice = text
	return m
}

// draftID is the ID a failure keeps the form's track under: the same
// for each try, so a retry replaces the draft.
func (m Model) draftID() string {
	if m.draft != "" {
		return m.draft
	}
	return tracks.NewID(time.Now())
}
