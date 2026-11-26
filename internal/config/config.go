// Package config describes the cln configuration file and how to generate it.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"

	"github.com/adhemukhlis/cln/internal/jsonfile"
)

// FileName is the configuration file written by `cln init`.
const FileName = "config.json"

// ErrNotInitialized is returned by Load when no config file exists yet.
var ErrNotInitialized = errors.New("config.json not found; run `cln init` first")

// Config is the content of the configuration file.
type Config struct {
	Path   Path   `json:"path"`
	Device Device `json:"device"`
	Tools  []Tool `json:"tools"`
}

// Path holds filesystem locations relevant to cleaning.
type Path struct {
	Home string `json:"home"`
}

// Device describes the machine cln runs on, using Go's GOOS/GOARCH names
// (os: darwin, linux, windows; arch: amd64, arm64).
type Device struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// New builds a Config from explicit values. A nil tools slice is stored as
// empty so the file always contains "tools": [] rather than null.
func New(home, goos, goarch string, tools []Tool) Config {
	if tools == nil {
		tools = []Tool{}
	}
	return Config{
		Path:   Path{Home: home},
		Device: Device{OS: goos, Arch: goarch},
		Tools:  tools,
	}
}

// Detect builds a Config for the current user and machine.
func Detect() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("detect home directory: %w", err)
	}
	return New(home, runtime.GOOS, runtime.GOARCH, DetectTools(exec.LookPath)), nil
}

// Write saves cfg as indented JSON to path, overwriting any existing file.
func Write(path string, cfg Config) error {
	return jsonfile.Write(path, cfg)
}

// Load reads the config file at path. It returns ErrNotInitialized when the
// file does not exist, and an error when it is invalid or has no home path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the fixed FileName or a test file
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, ErrNotInitialized
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Path.Home == "" {
		return Config{}, fmt.Errorf("%s: path.home is empty; run `cln init` again", path)
	}
	return cfg, nil
}
