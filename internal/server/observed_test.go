package server

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

// #803. A session finished its turn while the daemon was unreachable, so the
// `Stop` hook never posted — hooks do not queue, and one must not pay the
// deadline on every tool call for a daemon already found unreachable (#578). The
// link came back, the watcher reported `idle`, and the daemon refused it: a hook
// outranks the watcher.
//
// The rule is right and its premise had gone. A hook outranks the watcher because
// it speaks from inside the session — but this one observed nothing, it was
// silenced, and the status it left behind is an hour old.
//
// So the rule is stated the way it was always meant: **what was observed beats
// what was inferred**. A watcher `idle` read from Claude Code's own session
// record is a statement by Claude Code, and it outranks a stale hook. A watcher
// `idle` inferred from a quiet transcript does not — that is where a permission
// prompt and a running tool look the same (#235), which is what the guard exists
// for.

func TestADeclaredIdleClearsAStaleHookWorking(t *testing.T) {
	// A prompt: the hook owns `working`.
	sess := hookReport(store.Session{}, true, "UserPromptSubmit", "t1")
	if sess.Status != "working" || sess.StatusSource != "hook" {
		t.Fatalf("setup: %q from %q", sess.Status, sess.StatusSource)
	}

	// The outage swallows the Stop. The watcher then reports what Claude Code's
	// own record says: this session is at rest.
	got := applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", Status: "idle", StatusDeclared: true, Timestamp: "t2",
	})
	if got.Status != "idle" {
		t.Errorf("status = %q, want idle — the board shows a session busy that has been waiting since the outage", got.Status)
	}
}

// The guard it must not break: an `idle` the watcher only inferred from silence
// still loses to a hook, because a permission prompt looks exactly like that.
func TestAnInferredIdleStillLosesToAHook(t *testing.T) {
	for _, held := range []string{"working", "waiting", "thinking"} {
		sess := store.Session{ID: "s", Status: held, StatusSource: "hook"}
		got := applyReport(sess, false, api.ReportRequest{
			SessionID: "s", Event: "watch", Status: "idle", Timestamp: "t2",
		})
		if got.Status != held {
			t.Errorf("a guess at silence overrode a hook-set %q → %q", held, got.Status)
		}
	}
}

// And a declared status the hook never contradicted is unaffected either way.
func TestADeclaredStatusIsOtherwiseOrdinary(t *testing.T) {
	sess := store.Session{ID: "s", Status: "idle", StatusSource: "watch"}
	got := applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", Status: "working", StatusDeclared: true, Timestamp: "t2",
	})
	if got.Status != "working" || got.StatusSource != "watch" {
		t.Errorf("(%q, %q), want (working, watch)", got.Status, got.StatusSource)
	}
}
