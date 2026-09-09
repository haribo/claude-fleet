package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// #803. The watcher now says which of two things its status rests on: something
// stated — Claude Code's own session record, or the process table — or the shape
// of a quiet transcript, which is a deduction.
//
// The daemon needs the difference when a hook and the watcher disagree. A hook
// silenced by an outage leaves a status behind it, and only an observation should
// be allowed to overturn that; a guess at silence must not, because a permission
// prompt and a running tool look the same from outside (#235).
func TestTheWatcherSaysWhetherItSawOrInferred(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no compaction markers
	now := time.Now()

	// In the registry: Claude Code states this session's condition.
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "idle"}}
	if _, _, _, declared := resolveStatus(reg, nil, "s", &transcript.Info{}, time.Minute, now.Add(-time.Minute), now); !declared {
		t.Error("a status read from Claude Code's own record was reported as inferred; a stale hook would go on outranking it")
	}

	// Not in the registry: everything left is read off the transcript.
	if _, _, _, declared := resolveStatus(nil, nil, "s", &transcript.Info{}, time.Hour, now.Add(-time.Hour), now); declared {
		t.Error("a status deduced from a quiet transcript was reported as observed; it would clear a hook that is right")
	}
}

// A process confidently gone is observed too — it is read from the process table,
// not guessed at.
func TestAProcessFoundGoneIsObserved(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now()
	dead := map[string]sessionRecord{"s": {SessionID: "s", Status: "idle", PID: 1, ProcStart: 999999}}

	status, _, _, declared := resolveStatus(dead, nil, "s", &transcript.Info{}, time.Minute, now.Add(-time.Minute), now)
	if status != "ended" {
		t.Fatalf("status = %q, want ended", status)
	}
	if !declared {
		t.Error("the process table was treated as a guess")
	}
}
