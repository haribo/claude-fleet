package server

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

// #765. The end time can now appear without the status moving, and the SSE delta
// gate was told it never could.
//
// #739 made the stamp fire whenever a report establishes the end and no time is
// recorded yet. Two ways to reach a session that is `ended` with no end time — a
// report carrying no timestamp, and a row that ended before #739 shipped — and in
// both, the next report fills the time in while the status stays put. The
// signature was identical, so nothing was published and an open dashboard read
// `Ended: —` until someone reloaded it by hand.
//
// The exclusion said "always accompanied by a Status change, which is covered".
// It was true when it was written. #739 is what made it false, and it read as
// verified because it lived in a test file.

func TestAnEndTimeCanAppearWithoutTheStatusMoving(t *testing.T) {
	// A report with no timestamp: `ended`, and nothing to record.
	noTime := applyReport(store.Session{}, true, api.ReportRequest{
		SessionID: "s", Event: "watch", Status: "ended", Timestamp: "",
	})
	if noTime.Status != "ended" || noTime.EndedAt != "" {
		t.Fatalf("status %q, ended_at %q — want ended with no time, the state the premise denied",
			noTime.Status, noTime.EndedAt)
	}

	// A row that ended before #739: same state, reached the other way.
	old := store.Session{ID: "s", Status: "ended"}
	filled := applyReport(old, false, api.ReportRequest{
		SessionID: "s", Event: "watch", Status: "ended", Timestamp: "2026-09-06T10:00:00Z",
	})
	if filled.EndedAt != "2026-09-06T10:00:00Z" {
		t.Fatalf("ended_at = %q, want the report's instant", filled.EndedAt)
	}
	if filled.Status != old.Status {
		t.Fatalf("the status moved (%q → %q); this test is about the case where it does not",
			old.Status, filled.Status)
	}
	if visibleSignature(old) == visibleSignature(filled) {
		t.Error("the end time appeared and the signature did not change — no event is published, " +
			"and an open dashboard shows `Ended: —` until it is reloaded by hand")
	}
}
