// The usage-lease loop: one machine in the fleet fetches the usage snapshot,
// elected by a lease the daemon hands out (docs/design/token-rollup.md).
package watch

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/usage"
)

// runUsageLoop periodically tries to hold the usage lease and, when it does,
// fetches subscription usage and reports it. The token never leaves the machine.
func runUsageLoop(ctx context.Context, cfg *config.Config, interval time.Duration) {
	if interval <= 0 {
		return
	}
	fetcher := &usage.Fetcher{}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		usageCycle(ctx, cfg, fetcher)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func usageCycle(ctx context.Context, cfg *config.Config, fetcher *usage.Fetcher) {
	var lease api.LeaseResponse
	if err := postJSON(cfg, "/api/usage/lease", api.LeaseRequest{Holder: cfg.Machine}, &lease); err != nil {
		fmt.Fprintf(os.Stderr, "watch: usage lease: %v\n", err)
		return
	}
	if !lease.Acquired {
		return // another machine holds the lease
	}
	// From here the lease is ours, and every way out that is not a successful post
	// has to hand it back. A holder that keeps the lease without delivering empties
	// the gauges for the whole fleet, and an empty gauge reads exactly like one
	// nobody has filled yet (docs/design/usage.md § 2, #646).
	rep, ok, err := fetcher.Fetch(ctx, clock.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: usage fetch: %v\n", err)
		releaseUsageLease(cfg)
		return
	}
	if !ok {
		releaseUsageLease(cfg) // backing off: someone else may be able to fetch now
		return
	}
	rep.Holder = cfg.Machine // the lease this machine just acquired (#515)
	if err := postJSON(cfg, "/api/usage", rep, nil); err != nil {
		fmt.Fprintf(os.Stderr, "watch: post usage: %v\n", err)
		releaseUsageLease(cfg)
	}
}

// releaseUsageLease hands the lease back, best-effort: if the call fails the lease
// simply lapses on its own after the server's TTL, which is the same outcome one
// cycle later.
func releaseUsageLease(cfg *config.Config) {
	req := api.LeaseRequest{Holder: cfg.Machine, Release: true}
	if err := postJSON(cfg, "/api/usage/lease", req, nil); err != nil {
		fmt.Fprintf(os.Stderr, "watch: releasing the usage lease: %v\n", err)
	}
}
