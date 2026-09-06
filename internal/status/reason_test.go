package status_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/haribo/claude-vigie/internal/status"
)

// #742. The notification body said `is waiting` for a session that had stopped on
// a 529 — the TUI passed the raw status through as a predicate, and the browser
// passed it through as a noun. What a client must render is why the session is
// calling, and that verdict is the daemon's: the dashboard had already dropped
// `error` from an attention list it kept for itself (#538).
func TestTheReasonVocabularyMatchesTheSharedFixture(t *testing.T) {
	b, err := os.ReadFile("../../test/fixtures/attention-reasons.json")
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	var doc struct {
		Reasons []struct {
			Reason string `json:"reason"`
		} `json:"reasons"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	want := make([]string, len(doc.Reasons))
	for i, r := range doc.Reasons {
		want[i] = r.Reason
	}
	if len(status.Reasons) != len(want) {
		t.Fatalf("Reasons = %v, fixture = %v", status.Reasons, want)
	}
	for i := range want {
		if status.Reasons[i] != want[i] {
			t.Errorf("Reasons[%d] = %q, fixture says %q", i, status.Reasons[i], want[i])
		}
	}
}

func TestTheReasonSaysWhyRatherThanRepeatingTheStatus(t *testing.T) {
	cases := []struct {
		status  string
		hasCall bool
		want    string
	}{
		{"waiting", false, "waiting"},
		{"error", false, "error"},
		// A raised call outranks the status: the session said so itself, where the
		// other two are deductions (ADR-0010).
		{"waiting", true, "call"},
		{"working", true, "call"},
		{"idle", true, "call"},
		// Nothing is being asked.
		{"working", false, ""},
		{"idle", false, ""},
		{"ended", false, ""},
		{"stale", false, ""},
	}
	for _, c := range cases {
		if got := status.Reason(c.status, c.hasCall); got != c.want {
			t.Errorf("Reason(%q, call=%v) = %q, want %q", c.status, c.hasCall, got, c.want)
		}
	}
}

// Every status that needs the operator has a reason, and that reason is one the
// clients know. `Reason` returns the status itself for an attention status, which
// is right — the reason *is* the status when no call was raised — and holds only
// while the two vocabularies agree. A status added to the attention set tomorrow
// and not here would notify with a code no client can render.
func TestEveryAttentionStatusHasAReasonTheClientsKnow(t *testing.T) {
	known := map[string]bool{}
	for _, r := range status.Reasons {
		known[r] = true
	}
	for _, s := range status.Attention {
		got := status.Reason(s, false)
		if got == "" {
			t.Errorf("status %q needs the operator and has no reason to give them", s)
			continue
		}
		if !known[got] {
			t.Errorf("status %q yields reason %q, which is not in the shared vocabulary %v", s, got, status.Reasons)
		}
	}
	// And every status vigie can hold produces a known reason or none — never a
	// code invented on the way past.
	for _, s := range status.All {
		if got := status.Reason(s, false); got != "" && !known[got] {
			t.Errorf("status %q yields unknown reason %q", s, got)
		}
	}
}
