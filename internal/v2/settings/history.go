package settings

// History is Settings → Tracks → Tracks History: whether old tracks are
// archived on their own.
type History struct {
	AutoArchive bool `yaml:"auto_archive,omitempty"`
	// Unsaved is what auto-archive does with a track whose worktrees
	// hold unsaved work: UnsavedSkip ("" too) or UnsavedKeep.
	Unsaved string `yaml:"unsaved,omitempty"`
}

// What auto-archive does with a track that has unsaved work: leave it
// in Station, or archive it and keep its worktrees.
const (
	UnsavedSkip = "skip"
	UnsavedKeep = "keep"
)

// KeepUnsaved reports whether auto-archive archives a track with
// unsaved work, keeping its worktrees.
func (h History) KeepUnsaved() bool { return h.Unsaved == UnsavedKeep }
