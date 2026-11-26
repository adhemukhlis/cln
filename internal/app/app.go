// Package app implements the cln command-line interface.
package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/adhemukhlis/cln/internal/clean"
	"github.com/adhemukhlis/cln/internal/config"
	"github.com/adhemukhlis/cln/internal/ignore"
	"github.com/adhemukhlis/cln/internal/progress"
	"github.com/adhemukhlis/cln/internal/scan"
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
  clean      Permanently delete the items in result.json (asks first; -yes skips)
  init       Write config.json for this device to the current directory
  ignore     Create an empty ignore.json in the current directory
  scan       Find cleanup candidates under the home path (minus ignore.json); writes result.json
  version    Print version information

Flags:
`

// Run parses args and executes the requested command.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, info BuildInfo) error {
	flags := flag.NewFlagSet("cln", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {} // Usage is printed explicitly so write errors are not lost.
	showVersion := flags.Bool("version", false, "print version and exit")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return printUsage(stderr, flags)
		}
		return errors.Join(ErrUsage, printUsage(stderr, flags))
	}

	if *showVersion {
		return printVersion(stdout, info)
	}

	switch cmd := flags.Arg(0); cmd {
	case "":
		return errors.Join(ErrUsage, printUsage(stderr, flags))
	case "init":
		return runInit(stdout)
	case "clean":
		return runClean(ctx, flags.Args()[1:], stdin, stdout, stderr)
	case "ignore":
		return runIgnore(stdout)
	case "scan":
		return runScan(ctx, stdout, stderr)
	case "version":
		return printVersion(stdout, info)
	default:
		if _, err := fmt.Fprintf(stderr, "unknown command %q\n\n", cmd); err != nil {
			return errors.Join(ErrUsage, fmt.Errorf("write usage: %w", err))
		}
		return errors.Join(ErrUsage, printUsage(stderr, flags))
	}
}

func printUsage(w io.Writer, flags *flag.FlagSet) error {
	if _, err := io.WriteString(w, usage); err != nil {
		return fmt.Errorf("write usage: %w", err)
	}
	flags.SetOutput(w)
	flags.PrintDefaults()
	return nil
}

func printVersion(w io.Writer, info BuildInfo) error {
	_, err := fmt.Fprintf(w, "cln %s (commit %s, built %s)\n", info.Version, info.Commit, info.Date)
	if err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}

// runInit detects the current device and writes config.json to the working directory.
func runInit(stdout io.Writer) error {
	cfg, err := config.Detect()
	if err != nil {
		return err
	}
	if err := config.Write(config.FileName, cfg); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "wrote %s\n", config.FileName); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// runScan finds cleanup candidates under the configured home path and writes
// result.json to the working directory. It requires config.json from `cln init`
// and skips the paths listed in ignore.json, if present.
func runScan(ctx context.Context, stdout, stderr io.Writer) error {
	cfg, err := config.Load(config.FileName)
	if err != nil {
		return err
	}
	home := cfg.Path.Home
	ignored, err := ignore.Load(ignore.FileName, home)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "scanning %s (%d ignored) ...\n", home, len(ignored)); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	line := progress.New(stderr)
	found, err := scan.Run(ctx, home, scan.Options{
		SkipDirs: scan.DefaultSkipDirs(home, cfg.Device.OS),
		Ignore:   ignored,
		Progress: func(p scan.Progress) {
			line.Tick(func(spinner string) string {
				return fmt.Sprintf("%s %s dirs · %d node_modules · %d misc · %d framework  %s",
					spinner, progress.Count(p.Dirs), p.NodeModules, p.Misc, p.Framework, shortPath(home, p.Current))
			})
		},
	})
	line.Clear()
	if err != nil {
		return err
	}
	res, err := scan.NewResult(home, found)
	if err != nil {
		return err
	}
	if err := scan.WriteResult(scan.ResultFileName, res); err != nil {
		return err
	}

	_, err = fmt.Fprintf(stdout, "found %d node_modules, %d misc, %d framework; wrote %s\n",
		len(res.NodeModules), len(res.Misc), len(res.Framework), scan.ResultFileName)
	if err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// runIgnore creates an empty ignore.json in the working directory, leaving an
// existing one untouched.
func runIgnore(stdout io.Writer) error {
	err := ignore.Create(ignore.FileName)
	msg := "created " + ignore.FileName
	if errors.Is(err, fs.ErrExist) {
		loc, absErr := filepath.Abs(ignore.FileName)
		if absErr != nil {
			loc = ignore.FileName
		}
		msg = "file already exist: " + loc
	} else if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, msg); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// runClean permanently deletes the candidates in result.json after asking for
// confirmation (skipped with -yes). It re-applies ignore.json and refuses any
// entry outside the configured home.
func runClean(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("cln clean", flag.ContinueOnError)
	flags.SetOutput(stderr)
	yes := flags.Bool("yes", false, "delete without asking for confirmation")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return ErrUsage
	}

	plan, err := loadPlan()
	if err != nil {
		return err
	}
	p := printer{w: stdout}
	p.printf("node_modules: %d\nmisc: %d\nframework: %d\n",
		plan.Count("node_modules"), plan.Count("misc"), plan.Count("framework"))
	if plan.Ignored > 0 {
		p.printf("skipping %d entries covered by %s\n", plan.Ignored, ignore.FileName)
	}
	if len(plan.Items) == 0 {
		p.printf("nothing to delete\n")
		return p.err
	}
	if !*yes {
		p.printf("Delete %d items permanently? [y/N] ", len(plan.Items))
		if p.err != nil {
			return p.err
		}
		if !confirmed(stdin) {
			p.printf("aborted, nothing deleted\n")
			return p.err
		}
	}

	line := progress.New(stderr)
	total := len(plan.Items)
	rep, execErr := clean.Execute(ctx, plan.Items, func(done int, it clean.Item) {
		line.Show(fmt.Sprintf("%s %d/%d  %s", line.Bar(done, total, 20), done, total, it.Display))
	})
	line.Clear()
	for _, f := range rep.Failed {
		if _, err := fmt.Fprintf(stderr, "failed: %s: %v\n", f.Item.Display, f.Err); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}
	p.printf("deleted %d, already gone %d, failed %d\n", rep.Deleted, rep.Missing, len(rep.Failed))
	if execErr != nil {
		return execErr
	}
	if p.err != nil {
		return p.err
	}
	if len(rep.Failed) > 0 {
		return fmt.Errorf("%d items could not be deleted", len(rep.Failed))
	}
	return nil
}

// loadPlan reads config.json, result.json and ignore.json and validates them
// into a deletion plan.
func loadPlan() (clean.Plan, error) {
	cfg, err := config.Load(config.FileName)
	if err != nil {
		return clean.Plan{}, err
	}
	res, err := scan.LoadResult(scan.ResultFileName)
	if err != nil {
		return clean.Plan{}, err
	}
	ignored, err := ignore.Load(ignore.FileName, cfg.Path.Home)
	if err != nil {
		return clean.Plan{}, err
	}
	return clean.NewPlan(cfg.Path.Home, res, ignored)
}

// confirmed reads one line from r and reports whether it is "y" or "yes".
// EOF or a read error counts as no.
func confirmed(r io.Reader) bool {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// printer writes formatted output and keeps the first write error.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	if _, err := fmt.Fprintf(p.w, format, args...); err != nil {
		p.err = fmt.Errorf("write output: %w", err)
	}
}

// shortPath shows path as "~/..." when it is inside home.
func shortPath(home, path string) string {
	if short, err := scan.Tilde(home, path); err == nil {
		return short
	}
	return path
}
