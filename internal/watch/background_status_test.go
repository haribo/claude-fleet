package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// #748. The whole chain, from what the registry and the transcript say to what
// the row shows. Claude Code's registry sits at `shell` for a session that
// launched a background command — the same signature as an operator who dropped
// to a `!` prompt — and the transcript's launch is already answered. Read alone,
// each says "at rest"; together they say a command is running.
func TestASessionWaitingOnABackgroundCommandReadsWorking(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no compaction markers
	now := time.Now()
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "shell"}}
	running := &transcript.Info{BackgroundActive: true}

	status, detail, _ := resolveStatus(reg, nil, "s", running, 5*time.Minute, now.Add(-5*time.Minute), now)

	if status != "working" {
		t.Errorf("status = %q, want working — the board offers a session that will resume by itself as free", status)
	}
	if detail == "shell" {
		t.Error("DETAIL says `shell`, which is the other thing that registry state means: the operator at a prompt")
	}
}

// The genuine #280 case is untouched: an operator at a shell prompt, with nothing
// running, is at rest. The registry cannot tell the two apart — the transcript is
// what separates them.
func TestAnOperatorAtAShellPromptIsStillIdle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now()
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "shell"}}

	status, detail, _ := resolveStatus(reg, nil, "s", &transcript.Info{}, 5*time.Minute, now.Add(-5*time.Minute), now)

	if status != "idle" || detail != "shell" {
		t.Errorf("(%q, %q), want (idle, shell)", status, detail)
	}
}

// No liveness cap: a command that has run for two days is a session working for
// two days (docs/design/session-status.md § 2, ADR-0012).
func TestALongBackgroundCommandStillReadsWorking(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now()
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "shell"}}
	running := &transcript.Info{BackgroundActive: true}

	twoDays := 48 * time.Hour
	status, _, _ := resolveStatus(reg, nil, "s", running, twoDays, now.Add(-twoDays), now)
	if status != "working" {
		t.Errorf("status = %q after 48 h, want working — a timer must not decide a command has stopped", status)
	}
}
