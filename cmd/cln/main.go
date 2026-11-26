// Command cln is a cross-platform CLI device cleaner.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/adhemukhlis/cln/internal/app"
)

// Set at build time via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	info := app.BuildInfo{Version: version, Commit: commit, Date: date}
	err := app.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, info)

	switch {
	case err == nil:
		return 0
	case errors.Is(err, app.ErrUsage):
		return 2
	default:
		fmt.Fprintln(os.Stderr, "cln:", err)
		return 1
	}
}
