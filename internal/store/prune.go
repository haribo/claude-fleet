package store

import (
	"context"
	"fmt"
	"time"
)

// staleWhere selects sessions considered stale by their last report, falling
// back to last_seen_at for rows written before last_report_at existed.
const staleWhere = `(CASE WHEN last_report_at != '' THEN last_report_at ELSE last_seen_at END) < ?`

// PruneSessions deletes sessions — and their events and token samples — whose
// last report is older than olderThan, bounding the database. It returns the
// number of sessions removed.
//
// The last report is the only thing consulted, deliberately. It cannot tell a
// machine that will not come back from one that simply cannot reach the daemon:
// from here the two are the same silence, so an outage longer than the window
// deletes the sessions of a machine still running them (#806). They return on the
// next scan; their event log, samples and start time do not. Token totals do —
// the rollup counts against a mark this never touches (#432).
//
// Sparing an unreachable machine was considered and does not work. The daemon
// draws that distinction for the *board* — `stale` rather than `ended` when no
// watcher is heard from a machine (#285) — on a sixty-second window, which makes
// a machine gone for a year "unreachable" too. Sparing on it would spare every
// dead machine, and nothing would ever be pruned. Documented instead, in
// docs/deployment.md.
func (s *Store) PruneSessions(ctx context.Context, olderThan time.Duration, now time.Time) (int, error) {
	cutoff := now.Add(-olderThan).UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin prune: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	sub := `SELECT id FROM sessions WHERE ` + staleWhere
	if _, err := tx.ExecContext(ctx, `DELETE FROM token_samples WHERE session_id IN (`+sub+`)`, cutoff); err != nil {
		return 0, fmt.Errorf("pruning samples: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE session_id IN (`+sub+`)`, cutoff); err != nil {
		return 0, fmt.Errorf("pruning events: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE `+staleWhere, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prune: %w", err)
	}
	return int(n), nil
}
