package server

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/store"
)

// #792. A session can reach `ended` two ways. #739 stamped a time on one of them
// — a report that establishes the end — and the 0.13.0 notes announced the case
// where nobody was there to close the session. The other way carries none: the
// daemon reads a session as over when its reports stop on a machine whose watcher
// is alive, and nothing is written on that path. The row went grey with a dash
// where the time belongs.
//
// `Last seen` does not answer in its place, which is what I assumed for three
// rounds without checking. It carries the report's own timestamp — for a watch
// report, the transcript's last *activity* — so a session that sat quiet for
// hours before it was killed shows `last seen 3h ago`: when it last worked, not
// when it stopped. Measured on a live fleet: a session being reported every two
// seconds showed a last-seen of nineteen minutes.
//
// What is recorded is when vigie stopped hearing about the session. That is the
// same honesty as the stamped path — the watcher notices a disappearance on its
// next pass, not at the instant of death — and it is the closest observable.

func TestAReadTimeEndCarriesTheTimeVigieStoppedSeeingIt(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	lastHeard := now.Add(-time.Hour)

	// Quiet for three hours, then killed: the last activity is far older than the
	// last report, which is the whole reason `Last seen` cannot answer.
	sess := store.Session{
		Status:     "idle",
		ReportedAt: lastHeard.Format(time.RFC3339),
		LastSeenAt: now.Add(-4 * time.Hour).Format(time.RFC3339),
	}

	v := toView(sess, nil, now, true)
	if v.Status != "ended" {
		t.Fatalf("status = %q, want ended", v.Status)
	}
	if v.EndedAt == "" {
		t.Fatal("the row reads ended with no time; this is the case where nobody was there to close the session")
	}
	if v.EndedAt != lastHeard.Format(time.RFC3339) {
		t.Errorf("ended_at = %q, want the last time vigie heard about the session (%q)",
			v.EndedAt, lastHeard.Format(time.RFC3339))
	}
	if v.EndedAt == v.LastSeenAt {
		t.Error("the end time repeats `last seen`; the two answer different questions and this path is where they diverge")
	}
}

// A recorded end wins: it is an observation, where this is a deduction from
// silence, and the two must not be allowed to disagree.
func TestARecordedEndTimeIsNotOverwritten(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	stamped := now.Add(-3 * time.Hour).Format(time.RFC3339)

	v := toView(store.Session{
		Status: "ended", EndedAt: stamped, ReportedAt: now.Add(-time.Hour).Format(time.RFC3339),
	}, nil, now, true)

	if v.EndedAt != stamped {
		t.Errorf("ended_at = %q, want the recorded %q — an observation outranks an inference", v.EndedAt, stamped)
	}
}

// A session that is not over has no end time, whatever else is true of it. The
// clear #722 added must survive this.
func TestALiveSessionStillHasNoEndTime(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	v := toView(store.Session{Status: "working", ReportedAt: now.Add(-2 * time.Second).Format(time.RFC3339)}, nil, now, true)
	if v.Status != "working" || v.EndedAt != "" {
		t.Errorf("status %q, ended_at %q — a running session was given an end time", v.Status, v.EndedAt)
	}
}

// On an unwatched machine "no news" means unobserved, not dead (#285): the
// session reads `stale`, and nothing is over, so no time is invented.
func TestAnUnwatchedMachineInventsNoEndTime(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	v := toView(store.Session{Status: "idle", ReportedAt: now.Add(-time.Hour).Format(time.RFC3339)}, nil, now, false)
	if v.Status != "stale" {
		t.Fatalf("status = %q, want stale", v.Status)
	}
	if v.EndedAt != "" {
		t.Errorf("ended_at = %q on a session vigie only lost sight of", v.EndedAt)
	}
}
