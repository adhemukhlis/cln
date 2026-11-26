// Package scan finds cleanable directories under a root directory.
package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Options controls which paths a scan skips.
type Options struct {
	// SkipDirs are absolute directories that are never descended into.
	SkipDirs []string
	// Ignore are absolute paths (files or directories) the user wants kept.
	// An ignored path is never reported, an ignored directory is never
	// descended into, and a candidate directory that contains an ignored path
	// is not reported either, since deleting it would delete the ignored path.
	Ignore []string
}

// isIgnored reports whether path is listed in Ignore.
func (o Options) isIgnored(path string) bool {
	return slices.Contains(o.Ignore, path)
}

// containsIgnored reports whether some ignored path lies inside dir.
func (o Options) containsIgnored(dir string) bool {
	prefix := dir + string(filepath.Separator)
	return slices.ContainsFunc(o.Ignore, func(p string) bool {
		return strings.HasPrefix(p, prefix)
	})
}

// DefaultSkipDirs returns directories under home that hold application data
// or package caches rather than user projects. Their contents look like
// projects (Electron apps ship node_modules, Go modules ship dist/ next to a
// go.mod) but deleting them breaks installed software. Hidden directories such
// as .config, .nvm and .vscode are always skipped as well.
func DefaultSkipDirs(home, goos string) []string {
	dirs := []string{filepath.Join(home, "go", "pkg", "mod")} // Go module cache (read-only sources).
	switch goos {
	case "darwin":
		dirs = append(dirs, filepath.Join(home, "Library"))
	case "windows":
		dirs = append(dirs, filepath.Join(home, "AppData"))
	}
	return dirs
}

// Found holds the absolute paths of every cleanup candidate found by Run.
type Found struct {
	// NodeModules are project node_modules directories: a node_modules that
	// sits next to a package.json.
	NodeModules []string
	// Misc are OS-generated metadata files and directories (see misc.go).
	Misc []string
	// Framework are build outputs and caches (see framework.go).
	Framework []string
}

// Run walks root once and returns, sorted, every cleanup candidate under it.
//
// A directory reported as a candidate is never descended into, so nothing
// inside it (dependencies, nested node_modules, .DS_Store files) is reported
// separately: deleting the directory removes it anyway. No node_modules is
// descended into, whether reported or not. Hidden directories, OS skip
// directories, symlinks and unreadable directories are skipped.
func Run(ctx context.Context, root string, opts Options) (Found, error) {
	var found Found
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("scan interrupted: %w", ctxErr)
		}
		if err != nil {
			if path == root {
				return err
			}
			return skipUnreadable(d)
		}
		if path == root {
			return nil
		}
		if !d.IsDir() {
			if d.Type().IsRegular() && !opts.isIgnored(path) {
				visitFile(path, d.Name(), &found)
			}
			return nil
		}
		return visitDir(root, path, d.Name(), opts, &found)
	})
	if err != nil {
		return Found{}, fmt.Errorf("scan %s: %w", root, err)
	}
	slices.Sort(found.NodeModules)
	slices.Sort(found.Misc)
	slices.Sort(found.Framework)
	return found, nil
}

// visitFile records the regular file at path if it is a candidate.
func visitFile(path, name string, found *Found) {
	switch {
	case isMiscFile(name):
		found.Misc = append(found.Misc, path)
	case isFrameworkFile(path, name):
		found.Framework = append(found.Framework, path)
	}
}

// visitDir records dir if it is a candidate and returns filepath.SkipDir for
// anything that must not be descended into.
func visitDir(root, path, name string, opts Options, found *Found) error {
	switch {
	case slices.Contains(opts.SkipDirs, path), opts.isIgnored(path):
		return filepath.SkipDir
	case isFrameworkDir(root, path, name):
		// Checked before the hidden-directory rule: .next, .nuxt, ... are hidden.
		if !opts.containsIgnored(path) {
			found.Framework = append(found.Framework, path)
		}
		return filepath.SkipDir
	case strings.HasPrefix(name, "."):
		return filepath.SkipDir
	case name == "node_modules":
		if isFile(filepath.Join(filepath.Dir(path), "package.json")) && !opts.containsIgnored(path) {
			found.NodeModules = append(found.NodeModules, path)
		}
		return filepath.SkipDir
	case isMiscDir(name):
		if !opts.containsIgnored(path) {
			found.Misc = append(found.Misc, path)
		}
		return filepath.SkipDir
	default:
		return nil
	}
}

// skipUnreadable keeps the walk going past entries that cannot be read
// (typically permission denied).
func skipUnreadable(d fs.DirEntry) error {
	if d != nil && d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// errNotUnderHome is returned by Tilde for paths outside home.
var errNotUnderHome = errors.New("path is not under home")

// Tilde rewrites path relative to home as "~/<rel>", using forward slashes on
// every OS.
func Tilde(home, path string) (string, error) {
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return "", fmt.Errorf("relativize %s: %w", path, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: %w", path, errNotUnderHome)
	}
	return "~/" + filepath.ToSlash(rel), nil
}
