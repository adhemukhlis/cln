package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// mkTree creates the given directories and files under root.
func mkTree(t *testing.T, root string, dirs, files []string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRun(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root,
		[]string{
			"git/app/node_modules/react/node_modules/scheduler", // nested dependency
			"git/app/node_modules/@scope/pkg/node_modules",      // nested dependency
			"git/mono/packages/web/node_modules",                // workspace package
			"git/mono/node_modules",
			"git/orphan/node_modules",             // no package.json next to it
			".nvm/versions/node/lib/node_modules", // hidden dir
			"Library/App/node_modules",            // skipped via options
		},
		[]string{
			"git/app/package.json",
			"git/app/node_modules/react/package.json",
			"git/mono/package.json",
			"git/mono/packages/web/package.json",
			".nvm/versions/node/lib/package.json",
			"Library/App/package.json",

			// Misc candidates.
			".DS_Store",
			"git/app/.DS_Store",
			"Downloads/photos/Thumbs.db",
			"Downloads/win/THUMBS.DB",
			"Downloads/photos/._IMG_0001.jpg",
			"Downloads/kde/.directory",
			"Downloads/archive/__MACOSX/._a.txt", // inside a misc dir: dir reported only

			// Inside candidates or skipped dirs: never reported on their own.
			"git/app/node_modules/.DS_Store",
			"git/orphan/node_modules/.DS_Store",
			".config/.DS_Store",
			"Library/.DS_Store",

			// Look-alikes that must stay.
			"Documents/desktop.ini",
			"Documents/._",
			"Documents/notes.DS_Store.txt",
		},
	)

	got, err := Run(t.Context(), root, Options{SkipDirs: []string{filepath.Join(root, "Library")}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	abs := func(paths ...string) []string {
		out := make([]string, len(paths))
		for i, p := range paths {
			out[i] = filepath.Join(root, filepath.FromSlash(p))
		}
		slices.Sort(out)
		return out
	}
	wantNodeModules := abs(
		"git/app/node_modules",
		"git/mono/node_modules",
		"git/mono/packages/web/node_modules",
	)
	wantMisc := abs(
		".DS_Store",
		"git/app/.DS_Store",
		"Downloads/photos/Thumbs.db",
		"Downloads/win/THUMBS.DB",
		"Downloads/photos/._IMG_0001.jpg",
		"Downloads/kde/.directory",
		"Downloads/archive/__MACOSX",
	)
	if !slices.Equal(got.NodeModules, wantNodeModules) {
		t.Errorf("NodeModules =\n  %v\nwant\n  %v", got.NodeModules, wantNodeModules)
	}
	if !slices.Equal(got.Misc, wantMisc) {
		t.Errorf("Misc =\n  %v\nwant\n  %v", got.Misc, wantMisc)
	}
}

func TestRunIgnore(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root,
		[]string{
			"git/keep/node_modules",     // ignored directly
			"git/inner/node_modules",    // contains an ignored path
			"git/drop/node_modules",     // reported
			"archive/__MACOSX",          // contains an ignored path
			"private/proj/node_modules", // under an ignored directory
		},
		[]string{
			"git/keep/package.json",
			"git/inner/package.json",
			"git/inner/node_modules/patched/index.js",
			"git/drop/package.json",
			"private/proj/package.json",
			"private/.DS_Store",
			"archive/__MACOSX/._keep",
			".DS_Store", // ignored file
			"git/.DS_Store",
		},
	)
	j := func(p string) string { return filepath.Join(root, filepath.FromSlash(p)) }

	got, err := Run(t.Context(), root, Options{Ignore: []string{
		j("git/keep/node_modules"),
		j("git/inner/node_modules/patched"),
		j("archive/__MACOSX/._keep"),
		j("private"),
		j(".DS_Store"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{j("git/drop/node_modules")}; !slices.Equal(got.NodeModules, want) {
		t.Errorf("NodeModules = %v, want %v", got.NodeModules, want)
	}
	if want := []string{j("git/.DS_Store")}; !slices.Equal(got.Misc, want) {
		t.Errorf("Misc = %v, want %v", got.Misc, want)
	}
}

func TestRunIgnoresSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
	root := t.TempDir()
	target := t.TempDir()
	mkTree(t, target, []string{"proj/node_modules"}, []string{"proj/package.json", "proj/.DS_Store"})
	if err := os.Symlink(filepath.Join(target, "proj"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(target, "proj", ".DS_Store"), filepath.Join(root, ".DS_Store")); err != nil {
		t.Fatal(err)
	}

	got, err := Run(t.Context(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.NodeModules) != 0 || len(got.Misc) != 0 {
		t.Errorf("followed symlink: %+v", got)
	}
}

func TestRunCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Run(ctx, t.TempDir(), Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestRunMissingRoot(t *testing.T) {
	_, err := Run(t.Context(), filepath.Join(t.TempDir(), "absent"), Options{})
	if err == nil {
		t.Fatal("want error for missing root")
	}
}

func TestTilde(t *testing.T) {
	home := filepath.FromSlash("/Users/me")
	got, err := Tilde(home, filepath.Join(home, "git", "app", "node_modules"))
	if err != nil || got != "~/git/app/node_modules" {
		t.Errorf("Tilde() = %q, %v", got, err)
	}
	if _, err := Tilde(home, filepath.FromSlash("/opt/x")); err == nil {
		t.Error("want error for path outside home")
	}
}

func TestDefaultSkipDirs(t *testing.T) {
	home := filepath.FromSlash("/h")
	modCache := filepath.Join(home, "go", "pkg", "mod")
	tests := map[string][]string{
		"darwin":  {modCache, filepath.Join(home, "Library")},
		"windows": {modCache, filepath.Join(home, "AppData")},
		"linux":   {modCache},
	}
	for goos, want := range tests {
		if got := DefaultSkipDirs(home, goos); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", goos, got, want)
		}
	}
}

func TestNewResult(t *testing.T) {
	home := filepath.FromSlash("/Users/me")
	got, err := NewResult(home, Found{
		NodeModules: []string{filepath.Join(home, "git", "a", "node_modules")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.NodeModules, []string{"~/git/a/node_modules"}) {
		t.Errorf("NodeModules = %v", got.NodeModules)
	}
	if got.Misc == nil || len(got.Misc) != 0 {
		t.Errorf("Misc = %#v, want empty non-nil slice", got.Misc)
	}
}

func TestExpand(t *testing.T) {
	home := filepath.FromSlash("/Users/me")
	got, err := Expand(home, "~/git/a/node_modules")
	if err != nil || got != filepath.Join(home, "git", "a", "node_modules") {
		t.Errorf("Expand() = %q, %v", got, err)
	}
	for _, bad := range []string{"~", "~/", "~/.", "~/..", "~/../x", "/etc", "git/a"} {
		if _, err := Expand(home, bad); err == nil {
			t.Errorf("Expand(%q) error = nil, want rejection", bad)
		}
	}
}
