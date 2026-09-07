package daemon

import (
	"errors"
	"testing"
	"time"
)

// #777. A read that did not happen is not an answer.
//
// `GetMeta` reports a failed read the same way it reports an absent key —
// `ok=false` — and both callers dropped the error. `decideRetention` reads
// `ok=false` as "nothing stored yet: the default governs", and a positive default
// prunes. So a busy database or an I/O error deleted sessions on a setting nobody
// had managed to read, and the next start wrote the default over the operator's
// choice — after which the setting really is the default, and nothing is left to
// notice.
//
// This is #656 arriving through another door: that fix rests on absent and empty
// being distinguishable, and they stop being so the moment the read fails.

func TestAFailedReadPrunesNothing(t *testing.T) {
	got := decideRetention("", false, errors.New("database is locked"), 24*time.Hour)
	if got.prune {
		t.Error("a failed read pruned; deleting on a setting nobody could read is the one outcome that cannot be undone")
	}
	if got.warning == "" {
		t.Error("it pruned nothing and said nothing — the operator has no way to learn their retention is unreadable")
	}
}

// The three answers stay three. An absent key still means the default governs,
// and an empty value still means the operator asked to keep everything (#656).
func TestTheThreeAnswersStayApart(t *testing.T) {
	if got := decideRetention("", false, nil, 24*time.Hour); !got.prune || got.window != 24*time.Hour {
		t.Errorf("absent: %+v, want the default governing", got)
	}
	if got := decideRetention("", true, nil, 24*time.Hour); got.prune {
		t.Errorf("empty: %+v, want keep-everything preserved", got)
	}
}

// And the seed does not fire on a read it could not make: seeding exists so the
// settings API has something to show on first run, never to overwrite. The
// decision is separated from the I/O for the same reason `decideRetention` is —
// a branch that ends in the operator's choice being overwritten has to be
// testable without arranging a failing disk.
func TestSeedingSkipsAReadItCouldNotMake(t *testing.T) {
	if shouldSeed(false, errors.New("database is locked")) {
		t.Error("a failed read seeded the default over whatever the operator had chosen")
	}
	if !shouldSeed(false, nil) {
		t.Error("an absent key must still be seeded, or the settings API has nothing to show")
	}
	if shouldSeed(true, nil) {
		t.Error("a stored value was overwritten; empty means keep everything (#656)")
	}
}
