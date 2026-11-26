package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/adhemukhlis/cln/internal/dns"
)

// runDNS implements `cln flush-dns`: it flushes the host's DNS cache. It does
// not need `cln init` and never touches the working directory, so running it
// with sudo leaves no root-owned files behind.
func runDNS(ctx context.Context, stdout io.Writer) error {
	return flushDNS(ctx, stdout, dns.HostSystem(ctx, runtime.GOOS), os.Geteuid(), dns.Exec)
}

// flushDNS is runDNS with the host dependencies injected.
func flushDNS(ctx context.Context, stdout io.Writer, sys dns.System, euid int, run dns.Runner) error {
	if dns.NeedsRoot(sys.GOOS, euid) {
		return dns.ErrNeedRoot
	}
	steps, err := dns.Plan(sys)
	if err != nil {
		return err
	}
	p := printer{w: stdout}
	if len(steps) == 0 {
		p.printf("no local DNS cache found; nothing to flush\n")
		return p.err
	}

	failed := 0
	for _, r := range dns.Run(ctx, steps, run) {
		if r.Err != nil {
			failed++
			p.printf("failed: %s: %v\n", r.Step, r.Err)
			continue
		}
		p.printf("done:   %s\n", r.Step)
	}
	if p.err != nil {
		return p.err
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d DNS flush steps failed", failed, len(steps))
	}
	p.printf("DNS cache flushed\n")
	return p.err
}
