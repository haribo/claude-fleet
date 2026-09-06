package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/status"
)

// #742. Every reason the daemon can send has a sentence here. A reason with none
// would notify "needs you", which is the fallback for a client that has fallen
// behind the daemon — not something an operator should ever be shown in a build
// that knows the vocabulary.
func TestEveryReasonHasASentence(t *testing.T) {
	for _, reason := range status.Reasons {
		got := notifyBody(api.SessionView{AttentionReason: reason})
		if got == "" || got == "needs you" {
			t.Errorf("reason %q notifies %q — the terminal has nothing to say about a reason the daemon sends", reason, got)
		}
	}
}

// The status is what the session *is*; the notification says what is being asked.
// A session stopped on a 529 was announced as `is error`.
func TestTheBodySaysWhatIsAskedNotWhatTheStatusIs(t *testing.T) {
	if got := notifyBody(api.SessionView{Status: "error", AttentionReason: "error"}); got == "error" {
		t.Errorf("body = %q — that is the machine's word for the state, not an instruction", got)
	}
	// A call speaks for itself when it carries a message, and still announces
	// itself when it does not.
	if got := notifyBody(api.SessionView{AttentionReason: "call", CallMessage: "the migration needs a decision"}); got != "the migration needs a decision" {
		t.Errorf("body = %q, want the call's own message", got)
	}
	if got := notifyBody(api.SessionView{AttentionReason: "call"}); got == "" {
		t.Error("a call with no message says nothing")
	}
}

// The reason is the daemon's verdict. A client that re-derived it from the status
// is what #538 was, and what ADR-0011 moved out of the clients.
func TestTheBodyDoesNotReadTheRawStatus(t *testing.T) {
	notMarked := api.SessionView{Status: "waiting", AttentionReason: ""}
	if got := notifyBody(notMarked); got != "needs you" {
		t.Errorf("body = %q for a session the daemon did not mark — the terminal decided for itself", got)
	}
}
