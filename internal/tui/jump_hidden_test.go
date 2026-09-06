package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haribo/claude-vigie/internal/api"
)

// #758. #736 fixed the jump for one way of hiding the caller and stated there was
// no other. There is: a session raises a call, its process dies, the watcher
// reports `ended` — and the call survives, because only a typed prompt or a clean
// SessionEnd clears one. Hiding ended sessions is on by default, so the row is off
// the board while the jump still picks it, and the panel opens on another session.
//
// The gap was in the invariant, not the mechanism: #736 argued from the attention
// set, and a raised call is not in it.

func callingButEnded() []api.SessionView {
	return []api.SessionView{
		{ID: "alpha", Name: "alpha", Machine: "laptop", Status: "working",
			LastSeenAt: "2026-09-06T10:00:00Z"},
		{ID: "bravo", Name: "bravo", Machine: "server", Status: "ended",
			CallAt: "2026-09-06T09:00:00Z", AttentionReason: "call",
			LastSeenAt: "2026-09-06T09:00:00Z"},
	}
}

func jumpWith(p prefs, sessions []api.SessionView) model {
	return model{
		width: 120, height: 40, prefs: p, sessions: sessions,
		sess: sessionsView{prevStatus: map[string]string{}, prevAttention: map[string]bool{}, prevCall: map[string]bool{}},
	}
}

func TestTheJumpRevealsACallerHiddenByAPreference(t *testing.T) {
	m := jumpWith(defaultPrefs(), callingButEnded())
	if !m.prefs.hideEnded {
		t.Fatal("setup: hiding ended sessions is meant to be the default")
	}

	out := pressN(m)
	if out.sess.selectedID != "bravo" {
		t.Fatalf("selected %q, want bravo — the call is what the jump is for", out.sess.selectedID)
	}
	if got := shownInDetail(out); got != "bravo" {
		t.Errorf("the panel shows %q, want bravo — the jump opened another session", got)
	}
}

// The preference is not rewritten to get there: it is the operator's, and it must
// still be in force for every other ended session.
func TestTheRevealDoesNotUnhideEverythingElse(t *testing.T) {
	sessions := append(callingButEnded(), api.SessionView{
		ID: "charlie", Name: "charlie", Machine: "server", Status: "ended",
		LastSeenAt: "2026-09-06T08:00:00Z",
	})
	out := pressN(jumpWith(defaultPrefs(), sessions))

	if !out.prefs.hideEnded {
		t.Error("the jump switched the operator's hide-ended preference off")
	}
	for _, s := range out.visibleSessions() {
		if s.ID == "charlie" {
			t.Error("an unrelated ended session came back on screen with the caller")
		}
	}
}

// And it lasts only as long as the panel it opened: closing the detail puts the
// board back the way the operator set it.
func TestTheRevealEndsWithTheDetailPanel(t *testing.T) {
	out := pressN(jumpWith(defaultPrefs(), callingButEnded()))
	out.sess = out.sess.handleNav(tea.KeyMsg{Type: tea.KeyEsc}, out.visibleSessions())

	for _, s := range out.visibleSessions() {
		if s.ID == "bravo" {
			t.Error("the revealed session stayed on the board after its panel was closed")
		}
	}
}
