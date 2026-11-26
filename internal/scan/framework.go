package scan

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Framework candidates are build outputs and caches that a framework,
// compiler or build tool regenerates on the next build. Most directory names
// are only candidates when a project marker sits next to them (dist next to
// package.json or go.mod, target next to Cargo.toml, ...), so a folder that
// merely shares the name is left alone. Names that only a tool ever creates
// (__pycache__, .pytest_cache, ...) need no marker.
//
// Generic names (dist, build, coverage, target) are also used for real source
// folders (e.g. electron-builder keeps icons in build/, some Go repositories
// commit dist/), so they additionally must be ignored by git: listed in a
// .gitignore next to them or in an ancestor .gitignore up to the repository
// root. Committed folders are never touched.

// Project markers.
const (
	npmMarker    = "package.json"
	goMarker     = "go.mod"
	cargoMarker  = "Cargo.toml"
	mavenMarker  = "pom.xml"
	pubMarker    = "pubspec.yaml"
	gradleMarker = "build.gradle"
	gradleKts    = "build.gradle.kts"
	settingsKts  = "settings.gradle.kts"
	settings     = "settings.gradle"
)

// dirRule describes when a directory with a given name is a candidate.
type dirRule struct {
	markers    []string // At least one must exist next to it; none means no marker needed.
	gitignored bool     // The directory must also be ignored by git.
}

// frameworkDirRule returns the rule for a directory name, and false for any
// other name. It is a switch, not a map, because it runs for every directory
// walked.
func frameworkDirRule(name string) (dirRule, bool) {
	switch name {
	case ".next", // Next.js
		".nuxt",         // Nuxt
		".svelte-kit",   // SvelteKit
		".turbo",        // Turborepo cache
		".parcel-cache", // Parcel
		".angular",      // Angular CLI cache
		".docusaurus":   // Docusaurus
		return dirRule{markers: []string{npmMarker}}, true
	case ".gradle": // Per-project Gradle cache.
		return dirRule{markers: []string{gradleMarker, gradleKts, settings, settingsKts}}, true
	case ".dart_tool": // Dart/Flutter tool cache.
		return dirRule{markers: []string{pubMarker}}, true
	case "__pycache__", // Python bytecode.
		".pytest_cache", // pytest
		".mypy_cache",   // mypy
		".ruff_cache":   // Ruff
		return dirRule{}, true
	case "coverage": // Test coverage reports (Jest, Vitest, c8, ...).
		return dirRule{markers: []string{npmMarker}, gitignored: true}, true
	case "dist": // Bundler output (Vite, tsc, webpack, ...) or GoReleaser output.
		return dirRule{markers: []string{npmMarker, goMarker}, gitignored: true}, true
	case "build": // CRA/Docusaurus, Gradle and Flutter output.
		return dirRule{markers: []string{npmMarker, gradleMarker, gradleKts, pubMarker}, gitignored: true}, true
	case "target": // Cargo and Maven output.
		return dirRule{markers: []string{cargoMarker, mavenMarker}, gitignored: true}, true
	default:
		return dirRule{}, false
	}
}

// isFrameworkDir reports whether the directory at path is a framework
// candidate. The scan root bounds the search for ancestor .gitignore files.
func isFrameworkDir(root, path, name string) bool {
	rule, ok := frameworkDirRule(name)
	if !ok {
		return false
	}
	parent := filepath.Dir(path)
	if len(rule.markers) > 0 && !hasAnyFile(parent, rule.markers) {
		return false
	}
	return !rule.gitignored || gitignored(root, parent, name)
}

// isFrameworkFile reports whether the regular file at path is a framework
// candidate.
func isFrameworkFile(path, name string) bool {
	if strings.HasSuffix(name, ".tsbuildinfo") && len(name) > len(".tsbuildinfo") {
		return true // TypeScript incremental build state, e.g. tsconfig.tsbuildinfo.
	}
	switch name {
	case ".eslintcache", ".stylelintcache":
		return hasAnyFile(filepath.Dir(path), []string{npmMarker})
	}
	return false
}

func hasAnyFile(dir string, names []string) bool {
	for _, n := range names {
		if isFile(filepath.Join(dir, n)) {
			return true
		}
	}
	return false
}

// gitignored reports whether a directory called name inside parent is ignored
// by a .gitignore in parent or in an ancestor, up to the repository root (the
// first directory containing .git) or root, whichever comes first. Only plain
// name patterns are understood; anything fancier is treated as not ignored.
func gitignored(root, parent, name string) bool {
	for dir := parent; ; dir = filepath.Dir(dir) {
		if gitignoreMatches(filepath.Join(dir, ".gitignore"), name, dir == parent) {
			return true
		}
		if exists(filepath.Join(dir, ".git")) || dir == root || filepath.Dir(dir) == dir {
			return false
		}
	}
}

// gitignoreMatches reports whether the .gitignore at path ignores a directory
// called name. Anchored patterns ("/dist") only count when the .gitignore sits
// next to the directory (sibling).
func gitignoreMatches(path, name string, sibling bool) bool {
	f, err := os.Open(path) //nolint:gosec // path is a .gitignore found while scanning
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // read-only file

	ignored := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		negated := strings.HasPrefix(line, "!")
		pattern := strings.TrimSuffix(strings.TrimPrefix(line, "!"), "/")
		pattern = strings.TrimPrefix(pattern, "**/")
		anchored := strings.HasPrefix(pattern, "/")
		if strings.TrimPrefix(pattern, "/") != name || (anchored && !sibling) {
			continue
		}
		ignored = !negated // Later lines override earlier ones, as in git.
	}
	return ignored
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
