package theme

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// ErrExists is returned when a new theme's id is taken.
var ErrExists = errors.New("a theme with this name exists")

const ext = ".yaml"

// ValidHex reports whether s is a colour value themes accept.
func ValidHex(s string) bool { return hexColor.MatchString(s) }

// With returns a copy of t with token set to value.
func (t Theme) With(token Token, value string) Theme {
	values := make(map[Token]string, len(t.values))
	for k, old := range t.values {
		values[k] = old
	}
	values[token] = value
	t.values = values
	return t
}

// YAML writes t as a theme file, tokens in the order of All.
func (t Theme) YAML() []byte {
	width := 0
	for _, token := range All {
		width = max(width, len(token))
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "display_name: %s\ntokens:\n", strconv.Quote(t.DisplayName))
	for _, token := range All {
		fmt.Fprintf(&b, "  %s: %s%q\n", token, strings.Repeat(" ", width-len(token)), t.values[token])
	}
	return b.Bytes()
}

// Entry is a theme as listed: valid, or a file with the reason it
// isn't.
type Entry struct {
	Theme
	Path string // the file, "" for built-ins
	Err  error  // why the file isn't a valid theme
}

// List returns the built-in themes, then the files in dir by file name.
// A missing dir has no files.
func List(dir string) ([]Entry, error) {
	var entries []Entry
	for _, t := range builtins() {
		entries = append(entries, Entry{Theme: t})
	}
	files, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return entries, err
	}
	var own []Entry
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ext || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		own = append(own, read(dir, strings.TrimSuffix(f.Name(), ext)))
	}
	slices.SortFunc(own, func(a, b Entry) int { return strings.Compare(a.ID, b.ID) })
	return append(entries, own...), nil
}

// read loads the file of the theme id in dir.
func read(dir, id string) Entry {
	path := filepath.Join(dir, id+ext)
	e := Entry{Theme: Theme{ID: id, DisplayName: id}, Path: path}
	if _, ok := builtin(id); ok {
		e.Err = fmt.Errorf("%s is a built-in theme's id; rename the file", id+ext)
		return e
	}
	data, err := os.ReadFile(path)
	if err != nil {
		e.Err = err
		return e
	}
	def := Default()
	t, err := parse(data, &def)
	if err != nil {
		e.Err = err
		return e
	}
	t.ID = id
	if t.DisplayName == "" {
		t.DisplayName = id
	}
	e.Theme = t
	return e
}

// Load returns the theme id: a built-in, or the file in dir. Tokens a
// file lacks, such as ones added since it was saved, come from the
// default theme. When it can't be read, Load returns the default theme
// along with the error.
func Load(dir, id string) (Theme, error) {
	if id == "" {
		return Default(), nil
	}
	if t, ok := builtin(id); ok {
		return t, nil
	}
	if !validID(id) {
		return Default(), fmt.Errorf("no theme %q", id)
	}
	e := read(dir, id)
	if e.Err != nil {
		return Default(), fmt.Errorf("theme %s: %w", id, e.Err)
	}
	return e.Theme, nil
}

// Save writes t over its file in dir. Built-ins can't be saved.
func (t Theme) Save(dir string) error {
	if t.BuiltIn || !validID(t.ID) {
		return fmt.Errorf("theme %s can't be saved; use Save as new", t.DisplayName)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, t.ID+ext)
	tmp, err := os.CreateTemp(dir, "."+t.ID+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(t.YAML()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Create writes t as a new theme called name in dir, its id made from
// the name. It fails with ErrExists when the id is taken.
func Create(dir, name string, t Theme) (Theme, error) {
	name = strings.TrimSpace(name)
	id := IDFrom(name)
	if id == "" {
		return Theme{}, errors.New("enter a name with a letter or digit")
	}
	if _, ok := builtin(id); ok {
		return Theme{}, ErrExists
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Theme{}, err
	}
	t.ID, t.DisplayName, t.BuiltIn = id, name, false
	f, err := os.OpenFile(filepath.Join(dir, id+ext), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return Theme{}, ErrExists
	}
	if err != nil {
		return Theme{}, err
	}
	if _, err := f.Write(t.YAML()); err != nil {
		f.Close()
		return Theme{}, err
	}
	return t, f.Close()
}

// IDFrom turns a theme name into a file name: lowercase letters and
// digits, other runs of characters as one "_", like the built-ins'.
func IDFrom(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return b.String()
}

// validID rejects ids that would leave the themes folder.
func validID(id string) bool {
	return id != "" && !strings.HasPrefix(id, ".") && !strings.ContainsAny(id, `/\`)
}
