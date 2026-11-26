// Package ignore manages ignore.json, the list of paths cln must leave alone.
package ignore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/adhemukhlis/cln/internal/jsonfile"
)

// FileName is the file created by `cln ignore`.
const FileName = "ignore.json"

// File is the content of ignore.json.
type File struct {
	Ignore []string `json:"ignore"`
}

// Create writes an empty ignore file ({"ignore": []}) at path. It never
// overwrites: if path already exists it returns an error matching fs.ErrExist.
func Create(path string) error {
	return jsonfile.Create(path, File{Ignore: []string{}})
}

// Load reads the ignore file at path and returns its entries as cleaned
// absolute paths. Entries are "~/..." paths (relative to home) or absolute
// paths. A missing file is not an error: it means nothing is ignored.
func Load(path, home string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the fixed FileName or a test file
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	paths := make([]string, 0, len(f.Ignore))
	for _, entry := range f.Ignore {
		abs, err := resolve(entry, home)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		paths = append(paths, abs)
	}
	return paths, nil
}

// resolve turns an entry into a cleaned absolute path.
func resolve(entry, home string) (string, error) {
	switch {
	case entry == "~":
		return filepath.Clean(home), nil
	case strings.HasPrefix(entry, "~/"):
		return filepath.Join(home, filepath.FromSlash(entry[2:])), nil
	case filepath.IsAbs(entry):
		return filepath.Clean(entry), nil
	default:
		return "", fmt.Errorf("invalid entry %q: use a \"~/...\" or absolute path", entry)
	}
}

// Covers reports whether deleting path would touch an ignored path: path is
// ignored itself, lies inside an ignored directory, or contains an ignored
// path. All paths must be cleaned absolute paths.
func Covers(ignored []string, path string) bool {
	for _, ig := range ignored {
		if ig == path || within(path, ig) || within(ig, path) {
			return true
		}
	}
	return false
}

// within reports whether child lies strictly inside dir.
func within(child, dir string) bool {
	return strings.HasPrefix(child, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}
