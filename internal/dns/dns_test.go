package dns

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// fakeSystem builds a System with the given executables on PATH and running
// processes.
func fakeSystem(goos string, onPath, running []string) System {
	return System{
		GOOS: goos,
		LookPath: func(file string) (string, error) {
			if slices.Contains(onPath, file) {
				return "/usr/bin/" + file, nil
			}
			return "", errors.New("not found")
		},
		Running: func(name string) bool { return slices.Contains(running, name) },
	}
}

func planStrings(t *testing.T, sys System) []string {
	t.Helper()
	steps, err := Plan(sys)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.String()
	}
	return out
}

func TestPlan(t *testing.T) {
	tests := []struct {
		name string
		sys  System
		want []string
	}{
		{
			name: "darwin uses fixed system paths",
			sys:  fakeSystem("darwin", nil, nil),
			want: []string{"/usr/bin/dscacheutil -flushcache", "/usr/bin/killall -HUP mDNSResponder"},
		},
		{
			name: "windows",
			sys:  fakeSystem("windows", []string{"ipconfig"}, nil),
			want: []string{"/usr/bin/ipconfig /flushdns"},
		},
		{
			name: "linux with systemd-resolved",
			sys:  fakeSystem("linux", []string{"resolvectl", "systemd-resolve"}, nil),
			want: []string{"/usr/bin/resolvectl flush-caches"},
		},
		{
			name: "linux with old systemd",
			sys:  fakeSystem("linux", []string{"systemd-resolve"}, nil),
			want: []string{"/usr/bin/systemd-resolve --flush-caches"},
		},
		{
			name: "linux with every cache",
			sys:  fakeSystem("linux", []string{"resolvectl", "nscd", "pkill"}, []string{"dnsmasq"}),
			want: []string{"/usr/bin/resolvectl flush-caches", "/usr/bin/nscd -i hosts", "/usr/bin/pkill -HUP -x dnsmasq"},
		},
		{
			name: "linux dnsmasq installed but not running",
			sys:  fakeSystem("linux", []string{"pkill", "dnsmasq"}, nil),
			want: []string{},
		},
		{
			name: "linux without local cache",
			sys:  fakeSystem("linux", nil, nil),
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planStrings(t, tt.sys); !slices.Equal(got, tt.want) {
				t.Errorf("Plan() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlanErrors(t *testing.T) {
	if _, err := Plan(fakeSystem("windows", nil, nil)); err == nil {
		t.Error("windows without ipconfig: want error")
	}
	if _, err := Plan(fakeSystem("plan9", nil, nil)); err == nil {
		t.Error("unsupported OS: want error")
	}
}

func TestNeedsRoot(t *testing.T) {
	tests := []struct {
		goos string
		euid int
		want bool
	}{
		{"darwin", 501, true},
		{"darwin", 0, false},
		{"linux", 1000, true},
		{"linux", 0, false},
		{"windows", -1, false},
	}
	for _, tt := range tests {
		if got := NeedsRoot(tt.goos, tt.euid); got != tt.want {
			t.Errorf("NeedsRoot(%s, %d) = %v, want %v", tt.goos, tt.euid, got, tt.want)
		}
	}
}

func TestRunContinuesAndIncludesOutput(t *testing.T) {
	steps := []Step{{Path: "/bin/a"}, {Path: "/bin/b", Args: []string{"-x"}}}
	var ran []string
	run := func(_ context.Context, path string, _ ...string) ([]byte, error) {
		ran = append(ran, path)
		if path == "/bin/a" {
			return []byte("permission denied\n"), errors.New("exit status 1")
		}
		return nil, nil
	}

	results := Run(t.Context(), steps, run)
	if !slices.Equal(ran, []string{"/bin/a", "/bin/b"}) {
		t.Fatalf("ran = %v, want both steps", ran)
	}
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "permission denied") {
		t.Errorf("results[0].Err = %v, want command output included", results[0].Err)
	}
	if results[1].Err != nil {
		t.Errorf("results[1].Err = %v, want nil", results[1].Err)
	}
}
