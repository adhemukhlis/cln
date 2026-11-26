package scan

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

// ErrNoResult is returned by LoadResult when no result file exists yet.
var ErrNoResult = errors.New("result.json not found; run `cln scan` first")

// ResultFileName is the file written by `cln scan`.
const ResultFileName = "result.json"

// Result is the content of the result file: cleanup candidates as "~/" paths.
type Result struct {
	NodeModules []string `json:"node_modules"`
	Misc        []string `json:"misc"`
	Framework   []string `json:"framework"`
}

// NewResult converts the absolute paths in f to "~/" paths relative to home.
// Empty categories are kept as empty lists so they encode as [] not null.
func NewResult(home string, f Found) (Result, error) {
	nodeModules, err := tildeAll(home, f.NodeModules)
	if err != nil {
		return Result{}, err
	}
	misc, err := tildeAll(home, f.Misc)
	if err != nil {
		return Result{}, err
	}
	framework, err := tildeAll(home, f.Framework)
	if err != nil {
		return Result{}, err
	}
	return Result{NodeModules: nodeModules, Misc: misc, Framework: framework}, nil
}

func tildeAll(home string, paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		short, err := Tilde(home, p)
		if err != nil {
			return nil, err
		}
		out = append(out, short)
	}
	return out, nil
}

// WriteResult saves r to path, overwriting any existing file.
func WriteResult(path string, r Result) error {
	return jsonfile.Write(path, r)
}

// LoadResult reads the result file at path. It returns ErrNoResult when the
// file does not exist.
func LoadResult(path string) (Result, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the fixed ResultFileName or a test file
	if errors.Is(err, fs.ErrNotExist) {
		return Result{}, ErrNoResult
	}
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", path, err)
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return r, nil
}

// Expand is the inverse of Tilde: it turns "~/<rel>" into an absolute path
// strictly inside home. Anything else, including home itself and paths that
// escape home with "..", is rejected.
func Expand(home, short string) (string, error) {
	rel, ok := strings.CutPrefix(short, "~/")
	if !ok {
		return "", fmt.Errorf("%q: %w", short, errNotUnderHome)
	}
	path := filepath.Join(home, filepath.FromSlash(rel))
	if !strings.HasPrefix(path, filepath.Clean(home)+string(filepath.Separator)) {
		return "", fmt.Errorf("%q: %w", short, errNotUnderHome)
	}
	return path, nil
}
