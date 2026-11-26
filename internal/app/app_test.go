package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/adhemukhlis/cln/internal/config"
	"github.com/adhemukhlis/cln/internal/ignore"
	"github.com/adhemukhlis/cln/internal/scan"
)

func TestRun(t *testing.T) {
	info := BuildInfo{Version: "v1.2.3", Commit: "abc123", Date: "2026-01-01"}

	tests := []struct {
		name       string
		args       []string
		wantErr    error
		wantStdout string
	}{
		{name: "version command", args: []string{"version"}, wantStdout: "cln v1.2.3"},
		{name: "version flag", args: []string{"-version"}, wantStdout: "cln v1.2.3"},
		{name: "help", args: []string{"-h"}},
		{name: "no command", args: nil, wantErr: ErrUsage},
		{name: "unknown command", args: []string{"nope"}, wantErr: ErrUsage},
		{name: "unknown flag", args: []string{"-nope"}, wantErr: ErrUsage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(t.Context(), tt.args, nil, &stdout, &stderr, info)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}

func TestRunInit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows.

	var stdout, stderr bytes.Buffer
	if err := Run(t.Context(), []string{"init"}, nil, &stdout, &stderr, BuildInfo{}); err != nil {
		t.Fatalf("Run(init) error = %v, stderr = %s", err, stderr.String())
	}

	data, err := os.ReadFile(config.FileName)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	var got config.Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	if got.Path.Home != home {
		t.Errorf("path.home = %q, want %q", got.Path.Home, home)
	}
	if got.Device.OS == "" || got.Device.Arch == "" {
		t.Errorf("device not filled: %+v", got.Device)
	}
	if got.Tools == nil {
		t.Errorf(`"tools" missing or null in %s`, data)
	}
}

func TestRunScanRequiresInit(t *testing.T) {
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	err := Run(t.Context(), []string{"scan"}, nil, &stdout, &stderr, BuildInfo{})
	if !errors.Is(err, config.ErrNotInitialized) {
		t.Fatalf("Run(scan) error = %v, want ErrNotInitialized", err)
	}
	if _, statErr := os.Stat(scan.ResultFileName); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("result.json written without init (stat err = %v)", statErr)
	}
}

func TestRunScan(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	for _, f := range []string{"git/a/package.json", "git/b/package.json"} {
		p := filepath.Join(home, f)
		if err := os.MkdirAll(filepath.Join(filepath.Dir(p), "node_modules", "dep"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".DS_Store"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Write(config.FileName, config.New(home, "linux", "amd64", nil)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "git", "c", "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "git", "c", "package.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignore.FileName, []byte(`{"ignore": ["~/git/c"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run(t.Context(), []string{"scan"}, nil, &stdout, &stderr, BuildInfo{}); err != nil {
		t.Fatalf("Run(scan) error = %v, stderr = %s", err, stderr.String())
	}

	data, err := os.ReadFile(scan.ResultFileName)
	if err != nil {
		t.Fatalf("result not written: %v", err)
	}
	var got scan.Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	want := []string{"~/git/a/node_modules", "~/git/b/node_modules"}
	if !slices.Equal(got.NodeModules, want) {
		t.Errorf("node_modules = %v, want %v", got.NodeModules, want)
	}
	if !slices.Equal(got.Misc, []string{"~/.DS_Store"}) {
		t.Errorf("misc = %v, want [~/.DS_Store]", got.Misc)
	}
}

func TestRunIgnore(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	if err := Run(t.Context(), []string{"ignore"}, nil, &stdout, &stderr, BuildInfo{}); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	if got := stdout.String(); got != "created ignore.json\n" {
		t.Errorf("first run stdout = %q", got)
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{"ignore"}, nil, &stdout, &stderr, BuildInfo{}); err != nil {
		t.Fatalf("second run error = %v", err)
	}
	loc, err := filepath.Abs(ignore.FileName)
	if err != nil {
		t.Fatal(err)
	}
	if want := "file already exist: " + loc + "\n"; stdout.String() != want {
		t.Errorf("second run stdout = %q, want %q", stdout.String(), want)
	}
}

// setupClean writes config.json, result.json and the listed files under a
// fresh home, in a fresh working directory, and returns home.
func setupClean(t *testing.T, result scan.Result, files ...string) string {
	t.Helper()
	t.Chdir(t.TempDir())
	home := t.TempDir()
	for _, f := range files {
		p := filepath.Join(home, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.Write(config.FileName, config.New(home, "linux", "amd64", nil)); err != nil {
		t.Fatal(err)
	}
	if err := scan.WriteResult(scan.ResultFileName, result); err != nil {
		t.Fatal(err)
	}
	return home
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	return err == nil
}

func TestRunCleanRequiresScan(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := config.Write(config.FileName, config.New(t.TempDir(), "linux", "amd64", nil)); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run(t.Context(), []string{"clean", "-yes"}, nil, &stdout, &stderr, BuildInfo{})
	if !errors.Is(err, scan.ErrNoResult) {
		t.Fatalf("error = %v, want ErrNoResult", err)
	}
}

func TestRunCleanConfirmation(t *testing.T) {
	res := scan.Result{NodeModules: []string{"~/a/node_modules"}, Misc: []string{"~/.DS_Store"}}
	files := []string{"a/package.json", "a/node_modules/dep/index.js", ".DS_Store"}

	tests := []struct {
		name        string
		args        []string
		stdin       string
		wantDeleted bool
	}{
		{name: "answer no", args: []string{"clean"}, stdin: "n\n"},
		{name: "empty answer", args: []string{"clean"}, stdin: "\n"},
		{name: "no input", args: []string{"clean"}, stdin: ""},
		{name: "answer yes", args: []string{"clean"}, stdin: "YES\n", wantDeleted: true},
		{name: "-yes flag", args: []string{"clean", "-yes"}, wantDeleted: true},
		{name: "--yes flag", args: []string{"clean", "--yes"}, wantDeleted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := setupClean(t, res, files...)
			var stdout, stderr bytes.Buffer
			err := Run(t.Context(), tt.args, strings.NewReader(tt.stdin), &stdout, &stderr, BuildInfo{})
			if err != nil {
				t.Fatalf("error = %v, stderr = %s", err, stderr.String())
			}
			for _, p := range []string{"a/node_modules", ".DS_Store"} {
				if gone := !exists(t, filepath.Join(home, filepath.FromSlash(p))); gone != tt.wantDeleted {
					t.Errorf("%s deleted = %v, want %v\nstdout: %s", p, gone, tt.wantDeleted, stdout.String())
				}
			}
			if !exists(t, filepath.Join(home, "a", "package.json")) {
				t.Error("project file next to node_modules was deleted")
			}
		})
	}
}

func TestRunCleanReappliesIgnore(t *testing.T) {
	home := setupClean(t, scan.Result{Misc: []string{"~/keep/.DS_Store", "~/.DS_Store"}},
		"keep/.DS_Store", ".DS_Store")
	if err := os.WriteFile(ignore.FileName, []byte(`{"ignore": ["~/keep"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run(t.Context(), []string{"clean", "-yes"}, nil, &stdout, &stderr, BuildInfo{}); err != nil {
		t.Fatalf("error = %v", err)
	}
	if !exists(t, filepath.Join(home, "keep", ".DS_Store")) {
		t.Error("ignored file was deleted")
	}
	if exists(t, filepath.Join(home, ".DS_Store")) {
		t.Error("candidate was not deleted")
	}
	if !strings.Contains(stdout.String(), "skipping 1 entries") {
		t.Errorf("stdout = %q, want ignore notice", stdout.String())
	}
}

func TestRunCleanRejectsOutsideHome(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	setupClean(t, scan.Result{Misc: []string{"~/../" + filepath.Base(filepath.Dir(outside)) + "/victim"}})

	var stdout, stderr bytes.Buffer
	if err := Run(t.Context(), []string{"clean", "-yes"}, nil, &stdout, &stderr, BuildInfo{}); err == nil {
		t.Fatal("error = nil, want rejection")
	}
	if !exists(t, outside) {
		t.Error("file outside home was deleted")
	}
}
