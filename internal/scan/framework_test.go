package scan

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunFramework(t *testing.T) {
	root := t.TempDir()
	j := func(p string) string { return filepath.Join(root, filepath.FromSlash(p)) }
	files := map[string]string{
		// Next.js app: .next next to package.json, plus tsbuildinfo and eslint cache.
		"next/package.json":         "{}",
		"next/.next/cache/x":        "",
		"next/tsconfig.tsbuildinfo": "",
		"next/.eslintcache":         "",
		"next/.next/.DS_Store":      "", // inside a candidate: not reported
		"plain/.next/x":             "", // no package.json: skipped (hidden dir)
		"plain/.eslintcache":        "", // no package.json: kept
		"plain/types.tsbuildinfo":   "", // unique extension: reported without marker
		"plain/.tsbuildinfo":        "", // extension only, no name: kept

		// Generic names need a marker AND a .gitignore entry.
		"vite/package.json":        "{}",
		"vite/.gitignore":          "node_modules\n/dist\n",
		"vite/dist/index.js":       "",
		"electron/package.json":    "{}",
		"electron/.gitignore":      "dist\n",
		"electron/build/icon.icns": "", // committed resources: not gitignored, kept
		"electron/dist/app.js":     "",
		"goapp/go.mod":             "module x",
		"goapp/.gitignore":         "dist/\n",
		"goapp/dist/goapp":         "",
		"gosrc/go.mod":             "module y",
		"gosrc/dist/source.go":     "", // committed dist in a Go repo: kept
		"nomarker/.gitignore":      "dist\n",
		"nomarker/dist/x":          "",
		"negated/package.json":     "{}",
		"negated/.gitignore":       "dist\n!dist\n",
		"negated/dist/x":           "",

		// Monorepo: ignored by the repo-root .gitignore.
		"mono/.git/HEAD":             "",
		"mono/.gitignore":            "**/build\ncoverage\n",
		"mono/apps/web/package.json": "{}",
		"mono/apps/web/build/x":      "",
		"mono/apps/web/coverage/x":   "",

		// "/build" only applies next to the .gitignore that declares it.
		"anchored/.gitignore":       "/build\n",
		"anchored/sub/package.json": "{}",
		"anchored/sub/build/x":      "",

		// .gitignore files above the repository root are not consulted.
		"outside-repo/.gitignore":     "build\n",
		"outside-repo/r/.git/HEAD":    "",
		"outside-repo/r/package.json": "{}",
		"outside-repo/r/build/x":      "",

		// Other ecosystems.
		"rust/Cargo.toml":          "",
		"rust/.gitignore":          "/target\n",
		"rust/target/debug/x":      "",
		"rustnoignore/Cargo.toml":  "",
		"rustnoignore/target/x":    "", // target not gitignored: kept
		"java/pom.xml":             "",
		"java/.gitignore":          "target/\n",
		"java/target/x.jar":        "",
		"android/build.gradle.kts": "",
		"android/.gitignore":       "build\n",
		"android/.gradle/x":        "", // tool-specific name: marker only
		"android/build/x":          "",
		"flutter/pubspec.yaml":     "",
		"flutter/.dart_tool/x":     "",
		"py/app/__pycache__/a.pyc": "", // no marker needed
		"py/.pytest_cache/x":       "",
		"py/.mypy_cache/x":         "",
		"py/.ruff_cache/x":         "",
	}
	for p, c := range files {
		writeFile(t, j(p), c)
	}

	got, err := Run(t.Context(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		j("android/.gradle"),
		j("android/build"),
		j("electron/dist"),
		j("flutter/.dart_tool"),
		j("goapp/dist"),
		j("java/target"),
		j("mono/apps/web/build"),
		j("mono/apps/web/coverage"),
		j("next/.eslintcache"),
		j("next/.next"),
		j("next/tsconfig.tsbuildinfo"),
		j("plain/types.tsbuildinfo"),
		j("py/.mypy_cache"),
		j("py/.pytest_cache"),
		j("py/.ruff_cache"),
		j("py/app/__pycache__"),
		j("rust/target"),
		j("vite/dist"),
	}
	slices.Sort(want)
	if !slices.Equal(got.Framework, want) {
		t.Errorf("Framework =\n  %v\nwant\n  %v", got.Framework, want)
	}
	if slices.Contains(got.Misc, j("next/.next/.DS_Store")) {
		t.Error("file inside a framework candidate reported as misc")
	}
}

func TestRunFrameworkIgnore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app", "package.json"), "{}")
	writeFile(t, filepath.Join(root, "app", ".next", "keep.txt"), "")

	got, err := Run(t.Context(), root, Options{Ignore: []string{filepath.Join(root, "app", ".next", "keep.txt")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Framework) != 0 {
		t.Errorf("Framework = %v, want none (contains an ignored path)", got.Framework)
	}
}
