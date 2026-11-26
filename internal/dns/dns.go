// Package dns flushes the operating system's DNS cache.
package dns

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNeedRoot is returned when flushing requires root and cln is not root.
var ErrNeedRoot = errors.New("flushing the DNS cache needs root; run: sudo cln flush-dns")

// Step is one command to run.
type Step struct {
	Path string // Absolute path of the executable.
	Args []string
}

// String renders the step as a shell-like command line.
func (s Step) String() string {
	return strings.Join(append([]string{s.Path}, s.Args...), " ")
}

// System describes what Plan may inspect on the host. Fields are functions so
// tests can fake them.
type System struct {
	GOOS string
	// LookPath resolves an executable on PATH (exec.LookPath).
	LookPath func(file string) (string, error)
	// Running reports whether a process with this exact name is running.
	Running func(name string) bool
}

// NeedsRoot reports whether flushing on goos requires root, given the
// effective user id (-1 on Windows).
func NeedsRoot(goos string, euid int) bool {
	return goos != "windows" && euid != 0
}

// Plan returns the commands that flush the DNS cache on sys. An empty plan
// with a nil error means the host has no local DNS cache (common on Linux).
func Plan(sys System) ([]Step, error) {
	switch sys.GOOS {
	case "darwin":
		return []Step{
			{Path: "/usr/bin/dscacheutil", Args: []string{"-flushcache"}},
			{Path: "/usr/bin/killall", Args: []string{"-HUP", "mDNSResponder"}},
		}, nil
	case "windows":
		path, err := sys.LookPath("ipconfig")
		if err != nil {
			return nil, fmt.Errorf("find ipconfig: %w", err)
		}
		return []Step{{Path: path, Args: []string{"/flushdns"}}}, nil
	case "linux":
		return linuxPlan(sys), nil
	default:
		return nil, fmt.Errorf("flushing DNS is not supported on %s", sys.GOOS)
	}
}

// linuxPlan targets every local DNS cache that is present: systemd-resolved,
// nscd and dnsmasq.
func linuxPlan(sys System) []Step {
	var steps []Step
	if path, err := sys.LookPath("resolvectl"); err == nil {
		steps = append(steps, Step{Path: path, Args: []string{"flush-caches"}})
	} else if path, err := sys.LookPath("systemd-resolve"); err == nil {
		steps = append(steps, Step{Path: path, Args: []string{"--flush-caches"}})
	}
	if path, err := sys.LookPath("nscd"); err == nil {
		steps = append(steps, Step{Path: path, Args: []string{"-i", "hosts"}})
	}
	if sys.Running("dnsmasq") {
		if path, err := sys.LookPath("pkill"); err == nil {
			steps = append(steps, Step{Path: path, Args: []string{"-HUP", "-x", "dnsmasq"}})
		}
	}
	return steps
}

// Result is the outcome of one step.
type Result struct {
	Step Step
	Err  error // Includes the command's output when it failed.
}

// Runner runs a command and returns its combined output.
type Runner func(ctx context.Context, path string, args ...string) ([]byte, error)

// Run executes every step in order, continuing after failures.
func Run(ctx context.Context, steps []Step, run Runner) []Result {
	results := make([]Result, 0, len(steps))
	for _, s := range steps {
		out, err := run(ctx, s.Path, s.Args...)
		if err != nil {
			if msg := strings.TrimSpace(string(out)); msg != "" {
				err = fmt.Errorf("%w: %s", err, msg)
			}
		}
		results = append(results, Result{Step: s, Err: err})
	}
	return results
}

// Exec is the real Runner.
func Exec(ctx context.Context, path string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput() //nolint:gosec // path and args come from Plan's fixed list
	if err != nil {
		return out, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

// HostSystem returns a System backed by the real host.
func HostSystem(ctx context.Context, goos string) System {
	return System{
		GOOS:     goos,
		LookPath: exec.LookPath,
		Running: func(name string) bool {
			// pgrep exits 0 when a matching process exists.
			return exec.CommandContext(ctx, "pgrep", "-x", name).Run() == nil //nolint:gosec // name is a fixed process name from Plan
		},
	}
}
