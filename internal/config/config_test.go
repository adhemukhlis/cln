package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNew(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			tools := []Tool{{Name: "node", Path: "/bin/node"}}
			got := New("/home/u", goos, "arm64", tools)
			if got.Path.Home != "/home/u" || got.Device != (Device{OS: goos, Arch: "arm64"}) {
				t.Errorf("New() = %+v", got)
			}
			if !slices.Equal(got.Tools, tools) {
				t.Errorf("Tools = %v, want %v", got.Tools, tools)
			}
		})
	}
}

func TestNewNilToolsEncodesAsEmptyArray(t *testing.T) {
	data, err := json.Marshal(New("/h", "linux", "amd64", nil))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["tools"]); got != "[]" {
		t.Errorf(`"tools" = %s, want []`, got)
	}
}

func TestDetectTools(t *testing.T) {
	node := Tool{Name: "node", Path: "/opt/bin/node"}
	bun := Tool{Name: "bun", Path: "/opt/bin/bun"}
	tests := []struct {
		name      string
		installed []string
		want      []Tool
	}{
		{name: "none", installed: nil, want: []Tool{}},
		{name: "node only", installed: []string{"node"}, want: []Tool{node}},
		{name: "bun only", installed: []string{"bun"}, want: []Tool{bun}},
		{name: "both, fixed order", installed: []string{"bun", "node"}, want: []Tool{node, bun}},
		{name: "ignores unknown", installed: []string{"deno", "node"}, want: []Tool{node}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookPath := func(file string) (string, error) {
				if slices.Contains(tt.installed, file) {
					return "/opt/bin/" + file, nil
				}
				return "", errors.New("not found")
			}
			if got := DetectTools(lookPath); !slices.Equal(got, tt.want) {
				t.Errorf("DetectTools() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteOverwritesAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	want := New("/Users/me", "darwin", "amd64", []Tool{
		{Name: "node", Path: "/Users/me/.nvm/versions/node/v24.20.0/bin/node"},
		{Name: "bun", Path: "/Users/me/.bun/bin/bun"},
	})
	if err := Write(path, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, data)
	}
	if got.Path != want.Path || got.Device != want.Device || !slices.Equal(got.Tools, want.Tools) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file", func(t *testing.T) {
		_, err := Load(filepath.Join(dir, "absent.json"))
		if !errors.Is(err, ErrNotInitialized) {
			t.Fatalf("Load() error = %v, want ErrNotInitialized", err)
		}
	})

	t.Run("round trip", func(t *testing.T) {
		path := filepath.Join(dir, FileName)
		want := New("/Users/me", "darwin", "amd64", nil)
		if err := Write(path, want); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.Path != want.Path || got.Device != want.Device {
			t.Errorf("Load() = %+v, want %+v", got, want)
		}
	})

	for name, content := range map[string]string{
		"invalid json": "{",
		"empty home":   `{"path":{"home":""}}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || errors.Is(err, ErrNotInitialized) {
				t.Fatalf("Load() error = %v, want a parse/validation error", err)
			}
		})
	}
}
