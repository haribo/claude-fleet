// Package watch continuously scans the local Claude Code transcripts and reports
// every recent session to the fleet server, deriving status from the transcript
// state. Client-side; it covers sessions the hooks miss (already-open ones).
package watch

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/localwatch"
	"github.com/haribo/claude-vigie/internal/presence"
	"github.com/haribo/claude-vigie/internal/version"
)

// Options configures the watch loop.
type Options struct {
	Interval      time.Duration
	MaxAge        time.Duration
	UsageInterval time.Duration

	// Now is the loop's wall clock, defaulting to clock.Now. It is here because
	// the cadences Run owns — the 5 s heartbeat and the 5 min GC — are decided by
	// subtracting two readings of it, and a test cannot assert "on its own rhythm,
	// not the scan's" by waiting five real seconds per case. docs/code.md asks for
	// an injected func() time.Time in business logic and this loop had gone
	// without one, so the rules that live only here had no guard at all (#602).
	Now func() time.Time
}

// Status thresholds derived from how recently a transcript changed.
const (
	activeWindow = 10 * time.Second // transcript written this recently = working
	toolWindow   = 5 * time.Minute  // a tool_use turn may run this long before writing
)

// gcInterval is how often the watcher garbage-collects dead session mappings.
const gcInterval = 5 * time.Minute

// systemUser returns the OS account the watcher runs as (which, on a typical
// single-user machine, is the account that launched the sessions): the USER env
// var if set, else the current user, else "".
func systemUser() string {
	if u := config.OSUser(); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// ProjectsDir returns the Claude Code transcripts root (~/.claude/projects).
func ProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// Run scans on an interval and reports until ctx is canceled.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	root, err := ProjectsDir()
	if err != nil {
		return err
	}

	go runUsageLoop(ctx, cfg, opts.UsageInterval)

	// Name a build mismatch up front rather than letting it surface as refused
	// reports minutes later (#384).
	reportDaemonDrift(cfg)

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	now := opts.Now
	if now == nil {
		now = clock.Now
	}

	sc := newScanner()
	lastGC := now()
	var drifted, beatFailing, markFailing bool
	var lastBeat time.Time
	for {
		// Liveness is claimed on its own, never as a side effect of session data: a
		// watcher with nothing to report is still running, and a machine with no
		// live session used to read as watcher-less (#386). The answer also carries
		// the drift verdict, so it is what ends a drifted state (#384).
		if now().Sub(lastBeat) >= heartbeatInterval {
			drifted, beatFailing = beat(cfg, drifted, beatFailing)
			lastBeat = now()
		}

		// A drifted watcher goes inert rather than exiting: the packaged unit uses
		// Restart=on-failure, so exiting would crash-loop every few seconds and cost
		// the machine all observability. It keeps beating, so it stays visible, and
		// resumes by itself once the builds realign.
		if !drifted {
			reports, err := sc.scan(root, cfg.Machine, opts.MaxAge, now())
			if err != nil {
				fmt.Fprintf(os.Stderr, "watch: %v\n", err)
			}
			// Claim the local mark only here, after a real scan. It tells the
			// reporting hooks that transcripts on this machine are already being
			// read incrementally, so they can skip their own full re-read (#420).
			// A drifted watcher beats but never reaches this line: it stops
			// scanning, so hooks must keep reading for themselves.
			markFailing = markLocal(markFailing)
			drifted = postReports(cfg, reports, drifted)
		}
		if now().Sub(lastGC) > gcInterval {
			collectDeadMappings(opts.MaxAge, now())
			lastGC = now()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// markLocal refreshes the on-disk mark the reporting hooks read. Like beat, it
// announces transitions only, so a persistent failure never fills the journal.
//
// A scan interval longer than localwatch.StaleAfter leaves the mark permanently
// stale; that is safe by construction — hooks simply keep reading transcripts
// themselves, which is exactly what they did before #420.
func markLocal(failing bool) bool {
	err := localwatch.Mark()
	switch {
	case err == nil:
		if failing {
			fmt.Fprintln(os.Stderr, "watch: local watcher mark is writable again")
		}
		return false
	case !failing:
		fmt.Fprintf(os.Stderr, "watch: cannot write the local watcher mark, hooks will read transcripts themselves: %v\n", err)
	}
	return true
}

// heartbeatInterval is how often the watcher claims liveness. It sits well inside
// the 15 s staleness threshold the TUI and the Machines tab use, and is decoupled
// from the scan interval so a fast scan does not multiply requests (#386).
const heartbeatInterval = 5 * time.Second

// beat claims liveness and folds the daemon's answer into the drift state. It
// announces transitions only — into drift, out of it, and in and out of a
// transport failure — so a persistent condition never fills the journal (#386).
func beat(cfg *config.Config, drifted, failing bool) (newDrifted, newFailing bool) {
	err := postJSON(cfg, "/api/watcher/heartbeat", api.HeartbeatRequest{
		Machine:        cfg.Machine,
		WatcherVersion: version.Version,
		WatcherCommit:  version.Commit,
	}, nil)
	switch {
	case err == nil:
		if drifted {
			fmt.Fprintln(os.Stderr, "watch: build now matches the daemon — resuming session reports")
		}
		if failing {
			fmt.Fprintln(os.Stderr, "watch: heartbeat is reaching the server again")
		}
		return false, false
	case isDrift(err):
		if !drifted {
			fmt.Fprintf(os.Stderr, "watch: %v; session reports stay refused until this machine is upgraded\n", err)
		}
		return true, false
	default:
		// A transport failure is not drift — including the 404 an older daemon
		// answers, which the startup version probe has already explained.
		if !failing {
			fmt.Fprintf(os.Stderr, "watch: heartbeat: %v\n", err)
		}
		return drifted, true
	}
}

// postReports sends each report and returns whether this watcher is drifted — the
// daemon refusing its build. It stops at the first refusal, so a drifted watcher
// makes one rejected request rather than one per session, and it announces the
// transition into drift exactly once (#384).
func postReports(cfg *config.Config, reports []api.ReportRequest, drifted bool) bool {
	for _, r := range reports {
		err := post(cfg, r)
		switch {
		case err == nil:
			if drifted {
				fmt.Fprintln(os.Stderr, "watch: build now matches the daemon — resuming session reports")
				drifted = false
			}
		case isDrift(err):
			if !drifted {
				fmt.Fprintf(os.Stderr, "watch: %v; session reports stay refused until this machine is upgraded\n", err)
			}
			return true
		default:
			fmt.Fprintf(os.Stderr, "watch: reporting %s: %v\n", r.SessionID, err)
		}
	}
	return drifted
}

// collectDeadMappings removes presence mappings for sessions whose process died
// without a SessionEnd and whose transcript is past the watcher's window.
//
// It takes now from its caller rather than reading the clock itself: Run decides
// *whether* to collect by subtracting two readings of its own clock, and
// presence.GC decides *what* is old enough by comparing against one more. Two
// clocks either side of one decision is the shape #601 fixed elsewhere.
func collectDeadMappings(maxAge time.Duration, now time.Time) {
	n, err := presence.GC(maxAge, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: presence gc: %v\n", err)
	} else if n > 0 {
		fmt.Fprintf(os.Stderr, "watch: cleaned %d dead session mapping(s)\n", n)
	}
}
