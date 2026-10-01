package addtrack

import (
	"strings"

	"github.com/bluegardenproject/tracks/internal/tracks"
)

// title is c's heading on k's form.
func title(k Kind, c control) string {
	switch c {
	case ctlType:
		return "Type"
	case ctlRepos:
		switch k {
		case Work:
			return "Repos"
		case Doc:
			return "Repos for grounding (optional)"
		}
		return "Repos (optional)"
	case ctlRepo:
		return "Repo"
	case ctlTarget:
		return "PR or branch"
	case ctlDocument:
		return "Document"
	case ctlName:
		return "Name (optional)"
	case ctlTerminal:
		return "Terminal"
	case ctlSections:
		return "Sections"
	case ctlCandor:
		return "Candor"
	case ctlPrompt:
		if k == Ask {
			return "Question"
		}
		return "Prompt"
	case ctlEngine, ctlModel:
		return "Select engine"
	}
	return ""
}

// about is the text under c's heading on k's form.
func about(k Kind, c control) string {
	switch c {
	case ctlRepos:
		switch k {
		case Work:
			return "The repos this track starts with. The agent can add more later."
		case Doc:
			return "Attached read-only, so the reviewer can check the document's claims against the code."
		}
		return "Attached read-only for context. Leave them all off for a question not tied to a repo."
	case ctlRepo:
		return "The repo the PR or branch lives in."
	case ctlTarget:
		return "A GitHub pull request link (…/pull/123) or a branch on origin. It's checked out detached, to diff against its base."
	case ctlDocument:
		return "A file the agent can read (.md, .pdf, images, .csv, source) or a folder of them. ~ works."
	case ctlName:
		if k == Doc {
			return "Shown in Tracks and on the track's tab. Empty uses the document's file name."
		}
		return "Shown in Tracks and on the track's tab. Empty derives one from the prompt."
	case ctlSections:
		return "Findings and strengths always run. A section that's off is left out of the report."
	case ctlCandor:
		return "How bluntly the review is written. Wording only: it never changes the findings or the verdict."
	case ctlPrompt:
		switch k {
		case Work:
			return "What should the agent do? Mention a ticket (ABC-123) and it's used in the branch name."
		case Ask:
			return "Sent to the agent as it is, without extra framing."
		}
		return "Prefilled: sharpen it or leave it as it is."
	case ctlEngine:
		return "The agent and model, the type's from Settings → Tracks unless you pick others."
	}
	return ""
}

var placeholders = map[control]string{
	ctlTarget:   "https://github.com/org/repo/pull/123 or feat/foo",
	ctlDocument: "~/Downloads/architecture-review.pdf",
}

// namePlaceholder is v1's example name for k.
func namePlaceholder(k Kind) string {
	switch k {
	case Review:
		return "e.g. rate-bug-review"
	case Doc:
		return "e.g. q3-architecture-deck"
	}
	return "e.g. rate-bug-investigation"
}

// documentProblem is why path can't be reviewed, "" for a good path.
func documentProblem(path string) string {
	if _, err := tracks.ResolveDocument(path); err != nil {
		return err.Error()
	}
	return ""
}

// problems are what stops the form from creating a track, by control.
func (m Model) problems() map[control]string {
	out := map[control]string{}
	k := m.kind
	switch {
	case k == Work && len(m.repos) == 0, k == Review && len(m.repos) == 0:
		out[m.repoControl()] = noRepos
	case k == Work && len(m.pickedRepos()) == 0:
		out[ctlRepos] = "Pick at least one repo."
	}
	if k == Review && strings.TrimSpace(m.target.Value()) == "" {
		out[ctlTarget] = "Enter a pull request link or a branch name."
	}
	if k == Doc {
		if problem := documentProblem(m.document.Value()); problem != "" {
			out[ctlDocument] = problem
		}
	}
	if strings.TrimSpace(m.prompt.Value()) == "" {
		out[ctlPrompt] = "Enter a prompt."
		if k == Ask {
			out[ctlPrompt] = "Enter a question."
		}
	}
	return out
}

const noRepos = "Add a repo on the Repositories tab first."
