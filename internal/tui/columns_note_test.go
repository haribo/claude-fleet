package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #788. Narrow the terminal and a warning took a row above the session list and
// kept it. It cost rows at the moment rows are scarcest — measured on the real
// renderer at 96 columns, the message wrapped onto two lines and the board fell
// from ten sessions to seven, with a scroll indicator to say so — and it repeated
// something already on screen: the operator narrowed the window, and the missing
// columns are visible by their absence from the header.
//
// `sessions-chrome.md` § 2 decides this: a permanent row must not already be on
// screen, and must be something the operator can act on or is misled without.
// This failed both. `hidden N` earns its row on the second test precisely because
// the filters are silent; nothing about a narrow terminal is.
//
// The fact still has to be answerable for the case that is not self-inflicted —
// a terminal that *starts* narrow, a tmux pane, an ssh from a phone — so it moves
// behind `i`.

func narrowModel() model {
	m := model{
		width: 96, height: 16,
		prefs:    defaultPrefs(),
		sessions: []api.SessionView{{ID: "s", Name: "s", Machine: "m", Status: "working", LastSeenAt: "2026-09-08T10:00:00Z"}},
		sess:     sessionsView{prevStatus: map[string]string{}, prevAttention: map[string]bool{}, prevCall: map[string]bool{}},
	}
	return m
}

func TestTheBoardCarriesNoOverflowWarning(t *testing.T) {
	body := narrowModel().viewSessions()
	if strings.Contains(body, "columns hidden") {
		t.Errorf("the session list still carries the overflow warning:\n%s", body)
	}
}

// The fact is not lost: it is behind `i`, where "why is the board showing me less
// than it holds?" is already answered.
func TestTheStateModalNamesTheHiddenColumns(t *testing.T) {
	m := narrowModel()
	out := renderState(m.stateRows(), m.columnsNote(), m.width)
	if !strings.Contains(out, "columns hidden") {
		t.Errorf("the modal does not say the board is narrower than its layout:\n%s", out)
	}
	// It names them, which the header cannot: an absent column has no label.
	for _, want := range []string{"BRANCH", "DETAIL", "Settings"} {
		if !strings.Contains(out, want) {
			t.Errorf("the note does not mention %q — naming the columns and where to change them is what it is for:\n%s", want, out)
		}
	}
}

// And it is absent when everything fits: a modal that always carries a line
// trains the eye to skip the place the exception will appear (§ 2).
func TestNoNoteWhenTheColumnsFit(t *testing.T) {
	m := narrowModel()
	m.width = 400
	if note := m.columnsNote(); note != "" {
		t.Errorf("note = %q on a terminal wide enough for every column", note)
	}
	if out := renderState(m.stateRows(), m.columnsNote(), m.width); strings.Contains(out, "columns hidden") {
		t.Error("the modal carries the note with nothing hidden")
	}
}

// It is about the session board, so it appears where the board is.
func TestTheNoteIsScopedToTheSessionsTab(t *testing.T) {
	m := narrowModel()
	m.tab = tabStats
	if note := m.columnsNote(); note != "" {
		t.Errorf("note = %q while the operator is on Stats; the columns it names are on another tab", note)
	}
}
