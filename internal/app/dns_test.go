package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adhemukhlis/cln/internal/dns"
)

func darwinSystem() dns.System {
	return dns.System{
		GOOS:     "darwin",
		LookPath: func(string) (string, error) { return "", errors.New("unused") },
		Running:  func(string) bool { return false },
	}
}

func TestFlushDNSNeedsRoot(t *testing.T) {
	ran := false
	run := func(context.Context, string, ...string) ([]byte, error) { ran = true; return nil, nil }

	var stdout bytes.Buffer
	err := flushDNS(t.Context(), &stdout, darwinSystem(), 501, run)
	if !errors.Is(err, dns.ErrNeedRoot) {
		t.Fatalf("error = %v, want ErrNeedRoot", err)
	}
	if ran {
		t.Error("ran a command without root")
	}
}

func TestFlushDNS(t *testing.T) {
	var cmds []string
	run := func(_ context.Context, path string, args ...string) ([]byte, error) {
		cmds = append(cmds, strings.Join(append([]string{path}, args...), " "))
		return nil, nil
	}

	var stdout bytes.Buffer
	if err := flushDNS(t.Context(), &stdout, darwinSystem(), 0, run); err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(cmds) != 2 {
		t.Errorf("ran %v, want 2 commands", cmds)
	}
	if !strings.Contains(stdout.String(), "DNS cache flushed") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestFlushDNSReportsFailure(t *testing.T) {
	run := func(_ context.Context, path string, _ ...string) ([]byte, error) {
		if strings.HasSuffix(path, "killall") {
			return []byte("No matching processes"), errors.New("exit status 1")
		}
		return nil, nil
	}

	var stdout bytes.Buffer
	err := flushDNS(t.Context(), &stdout, darwinSystem(), 0, run)
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("error = %v, want 1 of 2 failed", err)
	}
	if !strings.Contains(stdout.String(), "failed: /usr/bin/killall -HUP mDNSResponder") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestFlushDNSNoLocalCache(t *testing.T) {
	sys := dns.System{
		GOOS:     "linux",
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Running:  func(string) bool { return false },
	}
	var stdout bytes.Buffer
	if err := flushDNS(t.Context(), &stdout, sys, 0, nil); err != nil {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(stdout.String(), "no local DNS cache") {
		t.Errorf("stdout = %q", stdout.String())
	}
}
