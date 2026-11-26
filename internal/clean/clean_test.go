package clean

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/adhemukhlis/cln/internal/scan"
)

func TestNewPlan(t *testing.T) {
	home := filepath.FromSlash("/Users/me")
	res := scan.Result{
		NodeModules: []string{"~/git/a/node_modules", "~/git/keep/node_modules"},
		Misc:        []string{"~/.DS_Store", "~/private/.DS_Store"},
	}
	ignored := []string{
		filepath.Join(home, "git", "keep", "node_modules", "patched"), // inside a candidate
		filepath.Join(home, "private"),                                // parent of a candidate
	}

	plan, err := NewPlan(home, res, ignored)
	if err != nil {
		t.Fatalf("NewPlan() error = %v", err)
	}
	want := []Item{
		{Category: "node_modules", Display: "~/git/a/node_modules", Path: filepath.Join(home, "git", "a", "node_modules")},
		{Category: "misc", Display: "~/.DS_Store", Path: filepath.Join(home, ".DS_Store")},
	}
	if len(plan.Items) != len(want) {
		t.Fatalf("Items = %+v, want %+v", plan.Items, want)
	}
	for i := range want {
		if plan.Items[i] != want[i] {
			t.Errorf("Items[%d] = %+v, want %+v", i, plan.Items[i], want[i])
		}
	}
	if plan.Ignored != 2 {
		t.Errorf("Ignored = %d, want 2", plan.Ignored)
	}
	if plan.Count("node_modules") != 1 || plan.Count("misc") != 1 {
		t.Errorf("Count() = %d/%d, want 1/1", plan.Count("node_modules"), plan.Count("misc"))
	}
}

func TestNewPlanRejectsOutsideHome(t *testing.T) {
	home := filepath.FromSlash("/Users/me")
	for _, entry := range []string{"/etc", "~", "~/", "~/..", "~/../other", "~/git/../../x", "git/a"} {
		t.Run(entry, func(t *testing.T) {
			_, err := NewPlan(home, scan.Result{Misc: []string{entry}}, nil)
			if err == nil {
				t.Fatalf("NewPlan(%q) error = nil, want rejection", entry)
			}
		})
	}
}

func TestExecute(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "proj", "node_modules")
	if err := os.MkdirAll(filepath.Join(dir, "dep", "lib"), 0o750); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, ".DS_Store")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	items := []Item{
		{Category: "node_modules", Path: dir},
		{Category: "misc", Path: file},
		{Category: "misc", Path: filepath.Join(root, "gone", ".DS_Store")},
	}

	rep, err := Execute(t.Context(), items, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if rep.Deleted != 2 || rep.Missing != 1 || len(rep.Failed) != 0 {
		t.Errorf("report = %+v, want 2 deleted, 1 missing, 0 failed", rep)
	}
	for _, p := range []string{dir, file} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists (err = %v)", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "proj")); err != nil {
		t.Errorf("parent directory removed: %v", err)
	}
}

func TestExecuteContinuesAfterFailure(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	errDenied := errors.New("permission denied")
	remove := func(p string) error {
		if p == a {
			return errDenied
		}
		return os.RemoveAll(p)
	}

	rep, err := execute(t.Context(), []Item{{Path: a}, {Path: b}}, nil, remove)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Deleted != 1 || len(rep.Failed) != 1 || !errors.Is(rep.Failed[0].Err, errDenied) {
		t.Errorf("report = %+v, want 1 deleted and a's failure", rep)
	}
}

func TestExecuteCanceled(t *testing.T) {
	file := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Execute(ctx, []Item{{Path: file}}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("file deleted after cancel: %v", err)
	}
}

func TestExecuteReportsProgress(t *testing.T) {
	root := t.TempDir()
	items := []Item{{Display: "~/a", Path: filepath.Join(root, "a")}, {Display: "~/b", Path: filepath.Join(root, "b")}}
	var seen []int
	if _, err := Execute(t.Context(), items, func(done int, it Item) {
		if it != items[done] {
			t.Errorf("onItem(%d) item = %+v, want %+v", done, it, items[done])
		}
		seen = append(seen, done)
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != 0 || seen[1] != 1 {
		t.Errorf("onItem done values = %v, want [0 1]", seen)
	}
}
