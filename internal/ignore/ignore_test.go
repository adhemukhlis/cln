package ignore

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	if err := Create(path); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"ignore\": []\n}\n"; string(data) != want {
		t.Errorf("content = %q, want %q", data, want)
	}
}

func TestCreateKeepsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	existing := []byte(`{"ignore":["~/keep"]}`)
	if err := os.WriteFile(path, existing, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Create(path); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Create() error = %v, want fs.ErrExist", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, existing) {
		t.Errorf("existing file changed to %q", data)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	home := filepath.FromSlash("/Users/me")
	abs := filepath.Join(t.TempDir(), "elsewhere")

	t.Run("missing file ignores nothing", func(t *testing.T) {
		got, err := Load(filepath.Join(dir, "absent.json"), home)
		if err != nil || len(got) != 0 {
			t.Fatalf("Load() = %v, %v; want empty, nil", got, err)
		}
	})

	t.Run("resolves entries", func(t *testing.T) {
		path := filepath.Join(dir, "ok.json")
		content := `{"ignore": ["~/git/keep/node_modules", "~/.DS_Store", "~/git/x/../y/", "` +
			filepath.ToSlash(abs) + `"]}`
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path, home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		want := []string{
			filepath.Join(home, "git", "keep", "node_modules"),
			filepath.Join(home, ".DS_Store"),
			filepath.Join(home, "git", "y"),
			abs,
		}
		if !slices.Equal(got, want) {
			t.Errorf("Load() =\n  %v\nwant\n  %v", got, want)
		}
	})

	for name, content := range map[string]string{
		"invalid json":   `{`,
		"relative entry": `{"ignore": ["git/keep"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path, home); err == nil {
				t.Fatal("Load() error = nil, want an error")
			}
		})
	}
}

func TestCovers(t *testing.T) {
	j := func(p string) string { return filepath.Join(filepath.FromSlash("/h"), filepath.FromSlash(p)) }
	ignored := []string{j("git/keep"), j("x/node_modules/patched")}
	tests := map[string]bool{
		"git/keep":       true,  // exact
		"git/keep/a":     true,  // inside ignored dir
		"git":            true,  // contains ignored dir
		"x/node_modules": true,  // contains ignored path
		"git/keeper":     false, // prefix of name only
		"other":          false,
	}
	for p, want := range tests {
		if got := Covers(ignored, j(p)); got != want {
			t.Errorf("Covers(%q) = %v, want %v", p, got, want)
		}
	}
}
