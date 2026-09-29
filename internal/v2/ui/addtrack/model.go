// Package addtrack is the New track form, a popup over the current
// window: a Type row, and the fields of the type picked below it.
package addtrack

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// Config is what the form starts from.
type Config struct {
	Theme theme.Theme
	// Repos are the names of the Repositories tab's repos; ReposErr is
	// why they couldn't be read.
	Repos    []string
	ReposErr error
	// RunsOn is what each type's tracks run on; a type that's missing
	// has no engine added.
	RunsOn map[track.Kind]RunsOn
	Create CreateFunc
}

// RunsOn is an engine's name and its model, "" for its own default.
// Missing says the engine isn't added, so Create would refuse.
type RunsOn struct {
	Engine, Model string
	Missing       bool
}

// Model is the form.
type Model struct {
	palette  style.Palette
	repos    []string
	reposErr error

	kind     Kind
	picked   map[string]bool // the repos ticked, kept across kinds
	repo     int             // Review's one repo
	target   textinput.Model // Review's pull request or branch
	document textinput.Model
	name     textinput.Model
	terminal bool
	sections []bool
	candor   int
	prompt   textarea.Model

	focus  control
	item   int // the cursor in a list, or on Repos: 0 the selector, then the picked repos
	hover  hit
	errs   map[control]string
	notice string
	picker *widget.Picker
	// pickerFor is the field the picker is open for.
	pickerFor control
	// discard is the open "Discard this track?" question.
	discard *question

	runsOn   map[track.Kind]RunsOn
	create   CreateFunc
	creating *creation
	failure  string // why the last Create failed
	made     *Created

	width, height, offset int
}

// question is a yes-or-no box over the form; focus 0 is yes.
type question struct{ focus, hover int }

// New returns the form for a Work track.
func New(c Config) Model {
	m := Model{
		palette:  style.New(c.Theme),
		repos:    c.Repos,
		reposErr: c.ReposErr,
		picked:   map[string]bool{},
		target:   widget.NewInput(500, placeholders[ctlTarget]),
		document: widget.NewInput(1000, placeholders[ctlDocument]),
		name:     widget.NewInput(60, namePlaceholder(Work)),
		sections: []bool{true, true},
		candor:   track.DefaultCandor,
		prompt:   newPrompt(),
		hover:    noHit,
		errs:     map[control]string{},
		runsOn:   c.RunsOn,
		create:   c.Create,
	}
	m.setPrompt(kinds[Work].prompt)
	return m
}

func newPrompt() textarea.Model {
	t := textarea.New()
	t.Prompt = ""
	t.ShowLineNumbers = false
	t.CharLimit = 8192
	t.MaxHeight = 0
	return t
}

// setPrompt fills the prompt with text, shown from its start.
func (m *Model) setPrompt(text string) {
	m.prompt.SetValue(text)
	m.prompt.MoveToBegin()
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case createEvent:
		m, cmd = m.created(msg)
	case tea.KeyPressMsg:
		if m.creating != nil {
			return m.creatingKey(msg.String())
		}
		m, cmd = m.key(msg)
	case tea.MouseClickMsg:
		if m.creating != nil {
			return m, nil
		}
		m, cmd = m.click(msg.Mouse())
	case tea.MouseMotionMsg:
		m = m.motion(msg.Mouse())
		return m, nil
	case tea.MouseWheelMsg:
		m = m.wheel(msg.Mouse())
		return m, nil
	default:
		if m.picker == nil && m.discard == nil && m.creating == nil {
			m, cmd = m.edit(msg)
		}
	}
	return m.fit(), cmd
}

func (m Model) key(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	key := msg.String()
	switch {
	case m.discard != nil:
		return m.discardKey(key)
	case m.picker != nil:
		return m.pickerKey(key), nil
	}
	m.notice = ""
	switch key {
	case "esc", "ctrl+c":
		return m.close()
	case "tab":
		return m.move(1, true)
	case "shift+tab":
		return m.move(-1, true)
	}
	switch m.focus {
	case ctlType:
		switch key {
		case "left", "h":
			m = m.setKind(max(Work, m.kind-1))
		case "right", "l":
			m = m.setKind(min(Doc, m.kind+1))
		case "down", "enter":
			return m.move(1, false)
		}
	case ctlRepos:
		return m.reposKey(key)
	case ctlSections:
		return m.listKey(key)
	case ctlTerminal:
		switch key {
		case "space", "x":
			m.terminal = !m.terminal
		case "up":
			return m.move(-1, false)
		case "down", "enter":
			return m.move(1, false)
		}
	case ctlRepo:
		switch key {
		case "enter", "space":
			m = m.openRepos()
		case "up":
			return m.move(-1, false)
		case "down":
			return m.move(1, false)
		}
	case ctlCandor:
		switch key {
		case "enter", "space":
			m = m.openCandor()
		case "left":
			m.candor = max(track.MinCandor, m.candor-1)
		case "right":
			m.candor = min(track.MaxCandor, m.candor+1)
		case "up":
			return m.move(-1, false)
		case "down":
			return m.move(1, false)
		}
	case ctlTarget, ctlDocument, ctlName:
		switch key {
		case "up":
			return m.move(-1, false)
		case "down", "enter":
			return m.move(1, false)
		}
		return m.edit(msg)
	case ctlPrompt:
		return m.edit(msg)
	case ctlCreate, ctlCancel:
		switch key {
		case "enter", "space":
			return m.press(m.focus)
		case "left":
			m = m.setFocus(ctlCreate)
		case "right":
			m = m.setFocus(ctlCancel)
		case "up":
			return m.move(-1, false)
		}
	}
	return m, nil
}

// listKey handles a key on the sections: the arrows walk the items and
// then leave, Space ticks one.
func (m Model) listKey(key string) (Model, tea.Cmd) {
	n := m.listLen(m.focus)
	switch key {
	case "up", "k":
		if m.item <= 0 {
			return m.move(-1, false)
		}
		m.item--
	case "down", "j":
		if m.item >= n-1 {
			return m.move(1, false)
		}
		m.item++
	case "space", "x":
		m = m.toggle(m.focus, m.item)
	case "enter":
		return m.move(1, false)
	}
	return m, nil
}

func (m Model) listLen(c control) int {
	if c == ctlSections {
		return len(m.sections)
	}
	return 0
}

// toggle ticks item i of the sections.
func (m Model) toggle(c control, i int) Model {
	if c == ctlSections && i < len(m.sections) {
		m.sections = slices.Clone(m.sections)
		m.sections[i] = !m.sections[i]
	}
	return m
}

// edit gives msg, a key or a paste, to the focused text field.
func (m Model) edit(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case ctlTarget:
		m.target, cmd = m.target.Update(msg)
	case ctlDocument:
		m.document, cmd = m.document.Update(msg)
	case ctlName:
		m.name, cmd = m.name.Update(msg)
	case ctlPrompt:
		m.prompt, cmd = m.prompt.Update(msg)
	default:
		return m, nil
	}
	if _, ok := msg.(tea.KeyPressMsg); ok || isPaste(msg) {
		delete(m.errs, m.focus)
	}
	return m, cmd
}

func isPaste(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.PasteMsg, tea.ClipboardMsg:
		return true
	}
	return false
}

// move steps the focus through the kind's controls; wrap goes round
// from the last to the first. Entering a list puts its cursor on the
// side it was entered from.
func (m Model) move(step int, wrap bool) (Model, tea.Cmd) {
	order := controls(m.kind)
	i := slices.Index(order, m.focus) + step
	switch {
	case wrap:
		i = (i + len(order)) % len(order)
	default:
		i = max(0, min(len(order)-1, i))
	}
	m = m.setFocus(order[i])
	if step < 0 {
		m.item = max(0, m.listLen(m.focus)-1)
	}
	return m, nil
}

// setFocus moves the focus to c, focusing its text field.
func (m Model) setFocus(c control) Model {
	m.focus, m.item = c, 0
	m.target.Blur()
	m.document.Blur()
	m.name.Blur()
	m.prompt.Blur()
	switch c {
	case ctlTarget:
		m.target.Focus()
	case ctlDocument:
		m.document.Focus()
	case ctlName:
		m.name.Focus()
	case ctlPrompt:
		m.prompt.Focus()
	}
	return m
}

// setKind switches the form to k. Values carry over, and a prompt still
// as the old kind's template becomes the new one's.
func (m Model) setKind(k Kind) Model {
	if k == m.kind {
		return m
	}
	if strings.TrimSpace(m.prompt.Value()) == strings.TrimSpace(kinds[m.kind].prompt) {
		m.setPrompt(kinds[k].prompt)
	}
	if picked := m.pickedRepos(); k == Review && len(picked) == 1 {
		m.repo = slices.Index(m.repos, picked[0])
	}
	m.kind = k
	m.name.Placeholder = namePlaceholder(k)
	m.errs = map[control]string{}
	return m
}

// pickedRepos are the ticked repos in the tab's order.
func (m Model) pickedRepos() []string {
	var out []string
	for _, r := range m.repos {
		if m.picked[r] {
			out = append(out, r)
		}
	}
	return out
}

// repoControl is the kind's repo field.
func (m Model) repoControl() control {
	if m.kind == Review {
		return ctlRepo
	}
	return ctlRepos
}

// press presses a button.
func (m Model) press(c control) (Model, tea.Cmd) {
	m = m.setFocus(c)
	if c == ctlCancel {
		return m.close()
	}
	m.errs = m.problems()
	if len(m.errs) > 0 {
		for _, c := range controls(m.kind) {
			if m.errs[c] != "" {
				return m.setFocus(c), nil
			}
		}
	}
	return m.startCreate()
}

// close quits, asking first when something was entered.
func (m Model) close() (Model, tea.Cmd) {
	if m.dirty() {
		m.discard = &question{focus: 1, hover: -1}
		return m, nil
	}
	return m, tea.Quit
}

// dirty reports whether closing would lose something entered.
func (m Model) dirty() bool {
	prompt := strings.TrimSpace(m.prompt.Value())
	return m.target.Value() != "" || m.document.Value() != "" || m.name.Value() != "" ||
		len(m.pickedRepos()) > 0 || m.terminal || m.candor != track.DefaultCandor || slices.Contains(m.sections, false) ||
		prompt != "" && prompt != strings.TrimSpace(kinds[m.kind].prompt)
}

func (m Model) discardKey(key string) (Model, tea.Cmd) {
	q := *m.discard
	switch key {
	case "left", "right", "tab", "shift+tab":
		q.focus = 1 - q.focus
	case "enter", "space":
		return m.answer(q.focus)
	case "esc":
		return m.answer(1)
	}
	m.discard = &q
	return m, nil
}

// answer closes the question: 0 discards the track, 1 keeps editing.
func (m Model) answer(choice int) (Model, tea.Cmd) {
	m.discard = nil
	if choice == 0 {
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) openCandor() Model {
	items := make([]widget.PickerItem, 0, track.MaxCandor)
	for level := track.MinCandor; level <= track.MaxCandor; level++ {
		label := track.CandorLabel(level)
		if level == track.DefaultCandor {
			label += " (default)"
		}
		items = append(items, widget.PickerItem{Label: strconv.Itoa(level), Detail: label})
	}
	p := widget.NewPicker("Candor", items, m.candor-track.MinCandor)
	m.picker, m.pickerFor = &p, ctlCandor
	return m
}

func (m Model) pickerKey(key string) Model {
	p := *m.picker
	m.picker = &p
	return m.pickerDone(p.Key(key))
}

// pickerDone follows up on the picker closing. The repos ticked stay
// ticked however it closed.
func (m Model) pickerDone(r widget.PickerResult) Model {
	if r != widget.PickerChosen && r != widget.PickerClosed {
		return m
	}
	p := m.picker
	m.picker = nil
	switch {
	case m.pickerFor == ctlRepos:
		m.picked = map[string]bool{}
		for i, on := range p.Ticked {
			if on {
				m.picked[m.repos[i]] = true
			}
		}
		delete(m.errs, ctlRepos)
	case r == widget.PickerClosed:
	case m.pickerFor == ctlRepo:
		m.repo = p.Cursor
		delete(m.errs, ctlRepo)
	case m.pickerFor == ctlCandor:
		m.candor = p.Cursor + track.MinCandor
	}
	return m
}
