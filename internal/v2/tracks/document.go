package tracks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// unreadable are the file types an agent can't read, and what to call
// them.
var unreadable = map[string]string{
	".pptx": "PowerPoint", ".ppt": "PowerPoint", ".key": "Keynote", ".odp": "OpenDocument presentation",
	".docx": "Word", ".doc": "Word", ".odt": "OpenDocument text",
	".xlsx": "Excel", ".xls": "Excel", ".numbers": "Numbers", ".pages": "Pages",
}

// ResolveDocument is the absolute path of a document to review, as v1
// checks it: ~ is the home folder, and it has to exist and not be an
// office file.
func ResolveDocument(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", Problem("Enter the document's path.")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", Problem("Couldn't find your home folder for ~: " + err.Error())
		}
		path = filepath.Join(home, strings.TrimPrefix(path[1:], "/"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", Problem("Couldn't resolve the path: " + err.Error())
	}
	info, err := os.Stat(abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", Problem("There's no file or folder at " + abs + ".")
	case err != nil:
		return "", Problem("Couldn't read it: " + err.Error())
	}
	if format, bad := unreadable[strings.ToLower(filepath.Ext(abs))]; bad && !info.IsDir() {
		return "", Problem(format + " files can't be read directly. Export it to PDF and pick that.")
	}
	return abs, nil
}
