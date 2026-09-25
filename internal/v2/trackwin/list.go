package trackwin

// Info describes a track by its window.
type Info struct {
	Number int // the window index, which navigation keys use
	Window string
	Name   string
	Kind   string
	Repo   string
	Dir    string // the working directory, a worktree of Repo
}

// List returns the tracks of session in window order. The Tracks
// window isn't one.
func List(t Tmux, session string) ([]Info, error) {
	windows, err := t.ListWindows(session)
	if err != nil {
		return nil, err
	}
	var tracks []Info
	for _, w := range windows {
		if w.Index == 0 {
			continue
		}
		tracks = append(tracks, Info{Number: w.Index, Window: w.ID, Name: w.Name, Kind: w.Kind, Repo: w.Repo, Dir: w.Dir})
	}
	return tracks, nil
}
