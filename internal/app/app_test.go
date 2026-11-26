package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"
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
			err := Run(t.Context(), tt.args, &stdout, &stderr, info)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}
