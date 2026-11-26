// Package app implements the cln command-line interface.
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

// ErrUsage signals invalid command-line usage; the message has already been printed.
var ErrUsage = errors.New("usage error")

// BuildInfo describes the running binary, injected from main via ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

const usage = `cln - device cleaner

Usage:
  cln [flags] <command>

Commands:
  version    Print version information

Flags:
`

// Run parses args and executes the requested command.
func Run(_ context.Context, args []string, stdout, stderr io.Writer, info BuildInfo) error {
	fs := flag.NewFlagSet("cln", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {} // Usage is printed explicitly so write errors are not lost.
	showVersion := fs.Bool("version", false, "print version and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return printUsage(stderr, fs)
		}
		return errors.Join(ErrUsage, printUsage(stderr, fs))
	}

	if *showVersion {
		return printVersion(stdout, info)
	}

	switch cmd := fs.Arg(0); cmd {
	case "":
		return errors.Join(ErrUsage, printUsage(stderr, fs))
	case "version":
		return printVersion(stdout, info)
	default:
		if _, err := fmt.Fprintf(stderr, "unknown command %q\n\n", cmd); err != nil {
			return errors.Join(ErrUsage, fmt.Errorf("write usage: %w", err))
		}
		return errors.Join(ErrUsage, printUsage(stderr, fs))
	}
}

func printUsage(w io.Writer, fs *flag.FlagSet) error {
	if _, err := io.WriteString(w, usage); err != nil {
		return fmt.Errorf("write usage: %w", err)
	}
	fs.SetOutput(w)
	fs.PrintDefaults()
	return nil
}

func printVersion(w io.Writer, info BuildInfo) error {
	_, err := fmt.Fprintf(w, "cln %s (commit %s, built %s)\n", info.Version, info.Commit, info.Date)
	if err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}
