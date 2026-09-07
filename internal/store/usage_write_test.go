package store

import (
	"context"
	"testing"
	"time"
)

// #774. One machine in the fleet fetches the usage figures, elected by a lease.
// The daemon read who held it and wrote the snapshot as two separate operations,
// so a machine whose lease had lapsed in between could still write — landing an
// older reading on top of a fresher one from the machine that had taken over.
//
// The stored snapshot is never fabricated: both machines read the same account,
// so whichever lands is real. What was not guaranteed is that it is the latest,
// and the gauges carry the snapshot's age — so an operator could be shown a
// figure older than one the daemon already had.

func TestASnapshotIsRefusedOnceTheLeaseHasMoved(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

	if ok, _, err := st.AcquireLease(ctx, "laptop", time.Minute, t0); err != nil || !ok {
		t.Fatalf("setup: laptop should hold the lease: %v %v", ok, err)
	}
	// laptop fetches, and stalls. The lease lapses and the server takes over.
	later := t0.Add(2 * time.Minute)
	if ok, _, err := st.AcquireLease(ctx, "server", time.Minute, later); err != nil || !ok {
		t.Fatalf("setup: server should take the lapsed lease: %v %v", ok, err)
	}
	if err := st.SetMeta(ctx, "usage", "fresh, from the server"); err != nil {
		t.Fatal(err)
	}

	// laptop finally posts what it fetched two minutes ago.
	holder, written, err := st.SetMetaIfLeaseHolder(ctx, "usage", "stale, from the laptop", "laptop", later)
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Error("a machine that no longer holds the lease overwrote the snapshot")
	}
	if holder != "server" {
		t.Errorf("current holder reported as %q, want server — the caller cannot say who to blame", holder)
	}

	v, _, err := st.GetMeta(ctx, "usage")
	if err != nil {
		t.Fatal(err)
	}
	if v != "fresh, from the server" {
		t.Errorf("stored snapshot = %q; an older reading replaced a newer one", v)
	}
}

// The holder's own write still lands, which is the whole point of holding it.
func TestTheHolderStillWrites(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

	if _, _, err := st.AcquireLease(ctx, "laptop", time.Minute, now); err != nil {
		t.Fatal(err)
	}
	holder, written, err := st.SetMetaIfLeaseHolder(ctx, "usage", "a reading", "laptop", now)
	if err != nil || !written || holder != "laptop" {
		t.Fatalf("holder=%q written=%v err=%v; the elected machine must be able to write", holder, written, err)
	}
	if v, _, _ := st.GetMeta(ctx, "usage"); v != "a reading" {
		t.Errorf("stored = %q, want the holder's reading", v)
	}
}

// An expired lease belongs to nobody, so nobody may write on it.
func TestNobodyWritesOnAnExpiredLease(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

	if _, _, err := st.AcquireLease(ctx, "laptop", time.Minute, now); err != nil {
		t.Fatal(err)
	}
	holder, written, err := st.SetMetaIfLeaseHolder(ctx, "usage", "late", "laptop", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Error("a machine wrote on a lease that had expired under it")
	}
	if holder != "" {
		t.Errorf("holder = %q, want empty — an expired lease belongs to nobody", holder)
	}
}
