package store

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// #779. Three writes must never count twice: the token rollup, the status
// interval, and the usage lease. Each reads, decides, and writes in one
// transaction, and the transactions were deferred — the write lock was taken at
// the first write, not at the first read.
//
// The property held anyway, by accident of the engine: a deferred transaction
// that read before another committed is *refused* when it tries to write
// (`SQLITE_BUSY_SNAPSHOT`). Measured over 200 rounds before the change: no
// inflation, and errors in a few percent of rounds. Under-counting rather than
// double-counting is the right direction — `stats_daily` is never recomputed —
// but nothing in the code asked for it, and a retry loop or a driver change would
// have lowered the bar in silence.
//
// The transactions now take the write lock up front (`_txlock=immediate`), so a
// second writer waits for the first instead of doing work it will have to throw
// away. Same outcome, by construction rather than by refusal.

const contendedRounds = 60

// The property the daily figures depend on: growth reported by several writers at
// once is counted exactly once.
func TestConcurrentRollupsCountGrowthOnce(t *testing.T) {
	for r := 0; r < contendedRounds; r++ {
		st := openTestStore(t)
		ctx := context.Background()
		var wg sync.WaitGroup
		var returned int64
		start := make(chan struct{})
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				d, err := st.RollUpTokens(ctx, "s1", 1000, "2026-09-07", "m")
				if err != nil {
					t.Errorf("round %d: a contended rollup failed: %v", r, err)
					return
				}
				atomic.AddInt64(&returned, d)
			}()
		}
		close(start)
		wg.Wait()

		var stored int64
		if err := st.db.QueryRow(
			`SELECT output_tokens FROM stats_daily WHERE day = '2026-09-07' AND model = 'm'`).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != 1000 {
			t.Fatalf("round %d: stats_daily holds %d, want 1000 — the day is wrong for good, it is never recomputed", r, stored)
		}
		if returned != 1000 {
			t.Fatalf("round %d: the callers were told %d was counted, and %d was", r, returned, stored)
		}
		_ = st.Close()
	}
}

// The lease elects one fetcher, and losing the race is an ordinary answer — not
// an error. It used to arrive as one: the losing transaction was refused, so the
// caller could not tell "someone else holds it" from "the database is broken".
func TestOneMachineWinsTheLeaseAndTheRestAreTold(t *testing.T) {
	for r := 0; r < contendedRounds; r++ {
		st := openTestStore(t)
		ctx := context.Background()
		now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

		var wg sync.WaitGroup
		var won, denied int64
		start := make(chan struct{})
		for _, holder := range []string{"a", "b", "c", "d"} {
			wg.Add(1)
			go func(h string) {
				defer wg.Done()
				<-start
				ok, _, err := st.AcquireLease(ctx, h, time.Minute, now)
				switch {
				case err != nil:
					t.Errorf("round %d: %s got an error where a denial was the answer: %v", r, h, err)
				case ok:
					atomic.AddInt64(&won, 1)
				default:
					atomic.AddInt64(&denied, 1)
				}
			}(holder)
		}
		close(start)
		wg.Wait()

		if won != 1 {
			t.Fatalf("round %d: %d machines hold the lease at once; the fleet fetches usage more than once", r, won)
		}
		if denied != 3 {
			t.Fatalf("round %d: %d clean denials, want 3", r, denied)
		}
		_ = st.Close()
	}
}

// The lock mode is part of the guarantee above, so it is stated where it can fail
// rather than left to be inferred from the DSN.
func TestTransactionsTakeTheWriteLockUpFront(t *testing.T) {
	if !strings.Contains(dsn("/tmp/x.db"), "_txlock=immediate") {
		t.Error("transactions are deferred again; the three writes above hold only while SQLite refuses a stale-snapshot write")
	}
}
